import { api, unwrap, type Schemas } from './api/client';

type Snapshot = Schemas['Snapshot'];
type RatePoint = Schemas['RatePoint'];

/** Live view of one cluster: the latest snapshot plus history, kept current over SSE. */
export class LiveCluster {
	id: string;
	snapshot = $state<Snapshot | null>(null);
	history = $state<RatePoint[]>([]);
	connected = $state(false);
	error = $state<string | null>(null);
	#es: EventSource | null = null;
	#retry: ReturnType<typeof setTimeout> | null = null;
	#closed = false;

	constructor(id: string) {
		this.id = id;
	}

	async start() {
		this.#closed = false;
		try {
			this.history = await unwrap(api.GET('/v1/clusters/{cluster}/history', { params: { path: { cluster: this.id } } }));
		} catch {}
		this.#open();
	}

	#open() {
		if (this.#closed) return;
		const es = new EventSource(`/api/v1/clusters/${encodeURIComponent(this.id)}/events`);
		this.#es = es;
		es.addEventListener('snapshot', (e) => {
			const snap = JSON.parse((e as MessageEvent).data) as Snapshot;
			this.snapshot = snap;
			this.connected = true;
			this.error = snap.error || null;
			if (snap.connected) {
				const last = this.history[this.history.length - 1];
				if (!last || last.t !== snap.time) {
					const t = snap.totals;
					const r = snap.rates;
					this.history.push({
						t: snap.time,
						publish: r.publish,
						deliver: r.deliverGet,
						ack: r.ack,
						confirm: r.confirm,
						redeliver: r.redeliver,
						unroutable: r.dropUnroutable + r.returnUnroutable,
						ready: t.messagesReady,
						unacked: t.messagesUnacked,
						connections: t.connections,
						channels: t.channels,
						queues: t.queues,
						consumers: t.consumers,
						connCreated: r.connCreated,
						connClosed: r.connClosed,
						chanCreated: r.chanCreated,
						chanClosed: r.chanClosed,
						queueCreated: r.queueCreated,
						queueDeleted: r.queueDeleted
					});
					if (this.history.length > 720) this.history.splice(0, this.history.length - 720);
				}
			}
		});
		es.onerror = () => {
			this.connected = false;
			es.close();
			if (!this.#closed) this.#retry = setTimeout(() => this.#open(), 3000);
		};
	}

	async refresh() {
		this.snapshot = await unwrap(
			api.GET('/v1/clusters/{cluster}/snapshot', { params: { path: { cluster: this.id }, query: { refresh: true } } })
		);
	}

	close() {
		this.#closed = true;
		this.#es?.close();
		if (this.#retry) clearTimeout(this.#retry);
	}
}
