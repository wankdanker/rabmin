/** Catalogs of RabbitMQ queue arguments and policy keys, for form helpers. */

export type QueueType = 'classic' | 'quorum' | 'stream';
export type ValueKind = 'int' | 'string' | 'bool' | 'choice' | 'duration';

export type ArgSpec = {
	key: string;
	label: string;
	kind: ValueKind;
	types: QueueType[];
	hint?: string;
	choices?: string[];
	placeholder?: string;
};

/** x-arguments the management UI offers when declaring a queue. */
export const queueArgs: ArgSpec[] = [
	{ key: 'x-message-ttl', label: 'Message TTL (ms)', kind: 'int', types: ['classic', 'quorum'], hint: 'Messages older than this are discarded or dead-lettered.' },
	{ key: 'x-expires', label: 'Auto-expire (ms)', kind: 'int', types: ['classic', 'quorum'], hint: 'Delete the queue after it has been unused this long.' },
	{ key: 'x-max-length', label: 'Max length (messages)', kind: 'int', types: ['classic', 'quorum'] },
	{ key: 'x-max-length-bytes', label: 'Max length (bytes)', kind: 'int', types: ['classic', 'quorum', 'stream'], hint: 'For streams: the retention size.' },
	{ key: 'x-overflow', label: 'Overflow behaviour', kind: 'choice', types: ['classic', 'quorum'], choices: ['drop-head', 'reject-publish', 'reject-publish-dlx'] },
	{ key: 'x-dead-letter-exchange', label: 'Dead letter exchange', kind: 'string', types: ['classic', 'quorum'] },
	{ key: 'x-dead-letter-routing-key', label: 'Dead letter routing key', kind: 'string', types: ['classic', 'quorum'] },
	{ key: 'x-dead-letter-strategy', label: 'Dead letter strategy', kind: 'choice', types: ['quorum'], choices: ['at-most-once', 'at-least-once'], hint: 'at-least-once needs overflow reject-publish.' },
	{ key: 'x-single-active-consumer', label: 'Single active consumer', kind: 'bool', types: ['classic', 'quorum', 'stream'] },
	{ key: 'x-max-priority', label: 'Max priority', kind: 'int', types: ['classic'], hint: 'Classic queues only; 1–255, ideally ≤ 10.' },
	{ key: 'x-delivery-limit', label: 'Delivery limit', kind: 'int', types: ['quorum'], hint: 'Redeliveries before a message is dead-lettered or dropped. RabbitMQ 4.x defaults to 20; -1 disables.' },
	{ key: 'x-quorum-initial-group-size', label: 'Initial replicas', kind: 'int', types: ['quorum'] },
	{ key: 'x-quorum-target-group-size', label: 'Target replicas', kind: 'int', types: ['quorum'] },
	{ key: 'x-initial-cluster-size', label: 'Initial replicas', kind: 'int', types: ['stream'] },
	{ key: 'x-queue-leader-locator', label: 'Leader locator', kind: 'choice', types: ['quorum', 'stream'], choices: ['client-local', 'balanced'] },
	{ key: 'x-max-age', label: 'Max age', kind: 'string', types: ['stream'], placeholder: '7D', hint: 'Units Y, M, D, h, m, s.' },
	{ key: 'x-stream-max-segment-size-bytes', label: 'Max segment size (bytes)', kind: 'int', types: ['stream'] },
	{ key: 'x-stream-filter-size-bytes', label: 'Filter size (bytes)', kind: 'int', types: ['stream'], hint: '16–255; for stream filtering.' }
];

export type PolicyKey = {
	key: string;
	kind: ValueKind;
	applies: ('classic' | 'quorum' | 'stream' | 'exchange')[];
	operator?: boolean;
	choices?: string[];
	hint?: string;
};

/** Policy definition keys. operator marks the keys operator policies accept. */
export const policyKeys: PolicyKey[] = [
	{ key: 'message-ttl', kind: 'int', applies: ['classic', 'quorum'], operator: true },
	{ key: 'expires', kind: 'int', applies: ['classic', 'quorum'], operator: true },
	{ key: 'max-length', kind: 'int', applies: ['classic', 'quorum'], operator: true },
	{ key: 'max-length-bytes', kind: 'int', applies: ['classic', 'quorum', 'stream'], operator: true },
	{ key: 'overflow', kind: 'choice', applies: ['classic', 'quorum'], choices: ['drop-head', 'reject-publish', 'reject-publish-dlx'], operator: true },
	{ key: 'delivery-limit', kind: 'int', applies: ['quorum'], operator: true },
	{ key: 'target-group-size', kind: 'int', applies: ['quorum'], operator: true },
	{ key: 'dead-letter-exchange', kind: 'string', applies: ['classic', 'quorum'] },
	{ key: 'dead-letter-routing-key', kind: 'string', applies: ['classic', 'quorum'] },
	{ key: 'dead-letter-strategy', kind: 'choice', applies: ['quorum'], choices: ['at-most-once', 'at-least-once'] },
	{ key: 'queue-leader-locator', kind: 'choice', applies: ['quorum', 'stream'], choices: ['client-local', 'balanced'] },
	{ key: 'max-age', kind: 'string', applies: ['stream'] },
	{ key: 'stream-max-segment-size-bytes', kind: 'int', applies: ['stream'] },
	{ key: 'alternate-exchange', kind: 'string', applies: ['exchange'] },
	{ key: 'federation-upstream', kind: 'string', applies: ['exchange', 'classic', 'quorum'] },
	{ key: 'federation-upstream-set', kind: 'string', applies: ['exchange', 'classic', 'quorum'] }
];

/** Which object kinds an apply-to value covers. */
export function appliesTo(applyTo: string): PolicyKey['applies'] {
	switch (applyTo) {
		case 'exchanges':
			return ['exchange'];
		case 'queues':
			return ['classic', 'quorum', 'stream'];
		case 'classic_queues':
			return ['classic'];
		case 'quorum_queues':
			return ['quorum'];
		case 'streams':
			return ['stream'];
		default:
			return ['classic', 'quorum', 'stream', 'exchange'];
	}
}

/** Converts a form string to the JSON value a key expects. */
export function coerce(kind: ValueKind, v: string): unknown {
	if (kind === 'int') {
		const n = Number(v);
		return Number.isFinite(n) ? n : v;
	}
	if (kind === 'bool') return v === 'true';
	return v;
}

/** Guesses the type of a free-form argument value: numbers, booleans, else strings. */
export function guessValue(v: string): unknown {
	const t = v.trim();
	if (t === 'true' || t === 'false') return t === 'true';
	if (/^-?\d+(\.\d+)?$/.test(t)) return Number(t);
	return v;
}

export const userTags = ['administrator', 'monitoring', 'policymaker', 'management', 'impersonator'];

/** Encodes a path segment for app routes (vhost "/" becomes %2F). */
export const seg = (s: string) => encodeURIComponent(s);

/** The default exchange's display name. */
export const exName = (n: string) => (n === '' ? '(AMQP default)' : n);
