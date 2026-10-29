<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, compact, num, rate, duration } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import Bindings from '$lib/components/Bindings.svelte';
	import PublishForm from '$lib/components/PublishForm.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const vhost = $derived(page.params.vhost!);
	const name = $derived(page.params.name!);
	let d = $state<Schemas['QueueDetail'] | null>(null);
	let error = $state<string | null>(null);
	let showRaw = $state(false);

	async function load() {
		try {
			d = await unwrap(api.GET('/v1/clusters/{cluster}/queues/{vhost}/{name}', { params: { path: { cluster: live.id, vhost, name } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	$effect(() => {
		name;
		load();
		const t = setInterval(() => !document.hidden && load(), 5000);
		return () => clearInterval(t);
	});

	const args = $derived(Object.entries((d?.queue.arguments ?? {}) as Record<string, unknown>).filter(([k]) => k !== 'x-queue-type'));
	const effective = $derived(Object.entries((d?.effectivePolicy ?? {}) as Record<string, unknown>));

	async function purge() {
		if (!d) return;
		if (
			!(await confirm({
				title: 'Purge queue',
				message: `Remove all ${num(d.queue.ready)} ready messages from ${name}? Unacknowledged messages stay. This cannot be undone.`,
				confirmText: 'Purge',
				danger: true,
				typeToConfirm: name
			}))
		)
			return;
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/queues/{vhost}/{name}/purge', { params: { path: { cluster: live.id, vhost, name } } }));
			toast(`Purged ${name}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	let ifEmpty = $state(true);
	let ifUnused = $state(true);
	async function remove() {
		if (!d) return;
		const guard = [ifEmpty && 'it is empty', ifUnused && 'it has no consumers'].filter(Boolean).join(' and ');
		if (
			!(await confirm({
				title: `Delete ${d.queue.type === 'stream' ? 'stream' : 'queue'}`,
				message: `Delete ${name} in ${vhost}${guard ? ` (only if ${guard})` : ` with its ${num(d.queue.messages)} messages`}? This cannot be undone.`,
				confirmText: 'Delete',
				danger: true,
				typeToConfirm: name
			}))
		)
			return;
		try {
			await unwrap(
				api.DELETE('/v1/clusters/{cluster}/queues/{vhost}/{name}', { params: { path: { cluster: live.id, vhost, name }, query: { ifEmpty, ifUnused } } })
			);
			toast(`Deleted ${name}`);
			goto(`/clusters/${live.id}/queues`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	const sac = $derived(args.some(([k, v]) => k === 'x-single-active-consumer' && v === true));
</script>

<svelte:head><title>{name} · Queues · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<div class="faint" style="font-size:12.5px"><a href="/clusters/{live.id}/queues">Queues</a> / <span class="mono">{vhost}</span> /</div>
			<h1>{name}</h1>
			{#if d}
				<div class="sub">
					<span class="badge {d.queue.type === 'quorum' ? 'accent' : d.queue.type === 'stream' ? 'info' : ''}">{d.queue.type}</span>
					<span class="badge {d.queue.state === 'running' || d.queue.state === 'idle' ? 'ok' : 'warn'}">{d.queue.state}</span>
					{d.queue.durable ? 'durable' : 'transient'}{d.queue.autoDelete ? ', auto-delete' : ''}{d.queue.exclusive ? ', exclusive' : ''}
					{#if sac}· single active consumer{/if}
				</div>
			{/if}
		</div>
		<span class="spacer"></span>
		{#if can('operator') && d}
			{#if d.queue.type !== 'stream'}<button class="btn" onclick={purge}><Icon name="trash" />Purge</button>{/if}
			<button class="btn danger" onclick={remove}><Icon name="trash" />Delete</button>
		{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !d}
		{#if !error}<Loading />{/if}
	{:else}
		{@const x = d.queue}
		<div class="kpis">
			<div class="card stat"><div class="label">Ready</div><div class="v num">{compact(x.ready)}</div><div class="faint small">{bytes(d.bytesReady)}</div></div>
			<div class="card stat"><div class="label">Unacked</div><div class="v num">{compact(x.unacked)}</div><div class="faint small">{bytes(d.bytesUnacked)}</div></div>
			<div class="card stat">
				<div class="label">Total</div>
				<div class="v num">{compact(x.messages)}</div>
				<div class="faint small">{compact(d.messagesPersistent)} persistent · {bytes(x.messageBytes)}</div>
			</div>
			<div class="card stat"><div class="label">Incoming</div><div class="v num">{rate(x.publishRate)}</div></div>
			<div class="card stat"><div class="label">Delivered</div><div class="v num">{rate(x.deliverRate)}</div><div class="faint small">ack {rate(x.ackRate)}</div></div>
			<div class="card stat">
				<div class="label">Consumers</div>
				<div class="v num">{x.consumers}</div>
				<div class="faint small">capacity {(x.consumerCapacity * 100).toFixed(0)}%</div>
			</div>
		</div>

		<div class="grid-2" style="margin-top:14px">
			<div class="stack">
				<div class="card">
					<div class="card-head"><h2>Features</h2></div>
					<div class="card-body stack">
						<dl class="kv-list">
							{#if x.members.length}
								<dt>Leader</dt><dd>{x.leader}</dd>
								<dt>Members</dt>
								<dd>
									{#each x.members as m}<span class="badge {x.online.includes(m) ? 'ok' : 'bad'}">{m.replace(/^rabbit@/, '')}{x.online.includes(m) ? '' : ' offline'}</span> {/each}
								</dd>
							{:else}
								<dt>Node</dt><dd>{x.node}</dd>
							{/if}
							{#if x.deliveryLimit != null}<dt>Delivery limit</dt><dd>{x.deliveryLimit}</dd>{/if}
							<dt>Policy</dt><dd>{x.policy ?? '—'}</dd>
							<dt>Operator policy</dt><dd>{x.operatorPolicy ?? '—'}</dd>
							<dt>Memory</dt><dd>{bytes(x.memory)}</dd>
						</dl>
						{#if args.length}
							<div>
								<div class="label" style="margin-bottom:4px">Arguments</div>
								<table class="t">
									<tbody>
										{#each args as [k, v]}<tr><td class="mono">{k}</td><td class="mono">{JSON.stringify(v)}</td></tr>{/each}
									</tbody>
								</table>
							</div>
						{/if}
						{#if effective.length}
							<div>
								<div class="label" style="margin-bottom:4px">Effective policy</div>
								<table class="t">
									<tbody>
										{#each effective as [k, v]}<tr><td class="mono">{k}</td><td class="mono">{JSON.stringify(v)}</td></tr>{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				</div>
				<div class="card">
					<div class="card-head"><h2>Consumers ({d.consumers.length})</h2></div>
					{#if d.consumers.length === 0}
						<div class="card-body faint">No consumers.</div>
					{:else}
						<table class="t">
							<thead><tr><th>Client</th><th>Tag</th><th>Ack</th><th class="r">Prefetch</th><th>Status</th></tr></thead>
							<tbody>
								{#each d.consumers as k}
									<tr>
										<td>{k.clientName || '(unnamed)'}<div class="faint small">{k.user} · {k.node}</div></td>
										<td class="mono small">{k.tag}</td>
										<td>{k.ackRequired ? 'manual' : 'auto'}</td>
										<td class="r num">{k.prefetch || '∞'}</td>
										<td>
											<span class="badge {k.status === 'waiting' ? 'warn' : 'ok'}">{k.status === 'single_active' ? 'single active' : k.status}</span>
											{#if k.timeoutMs}<div class="faint small">timeout {duration(k.timeoutMs)}</div>{/if}
										</td>
									</tr>
								{/each}
							</tbody>
						</table>
					{/if}
				</div>
				<div class="card">
					<div class="card-head"><h2>Bindings</h2></div>
					<div class="card-body">
						<Bindings cluster={live.id} {vhost} {name} side="destination" kind="queue" bindings={d.bindings} onchange={load} />
						<p class="faint small">Every queue is also bound to the default exchange with its own name.</p>
					</div>
				</div>
			</div>
			<div class="stack">
				{#if can('operator')}
					<div class="card">
						<div class="card-head"><h2>Publish to this queue</h2></div>
						<div class="card-body"><PublishForm cluster={live.id} {vhost} exchange="" queue={name} /></div>
					</div>
					<div class="card">
						<div class="card-head"><h2>Delete safely</h2></div>
						<div class="card-body row">
							<label class="check"><input type="checkbox" bind:checked={ifEmpty} />Only if empty</label>
							<label class="check"><input type="checkbox" bind:checked={ifUnused} />Only if unused</label>
						</div>
					</div>
				{/if}
				<div class="alert info">
					Reading messages from a classic or quorum queue is not non-destructive in RabbitMQ: a get with requeue changes order and redelivery counts. A safe
					browser for streams and an explicit peek come in the next release.
				</div>
				<div class="card">
					<div class="card-head">
						<h2>Management object</h2>
						<span class="spacer"></span>
						<button class="btn sm" onclick={() => (showRaw = !showRaw)}>{showRaw ? 'Hide' : 'Show'}</button>
					</div>
					{#if showRaw}<div class="card-body"><JsonView value={d.raw} /></div>{/if}
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.stat {
		padding: 12px 14px;
	}
	.label {
		font-size: 12px;
		color: var(--text-2);
	}
	.v {
		font-size: 22px;
		font-weight: 600;
	}
	.small {
		font-size: 12px;
	}
</style>
