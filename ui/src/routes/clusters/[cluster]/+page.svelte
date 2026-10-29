<script lang="ts">
	import { getLive } from '$lib/context';
	import { findCluster, refreshClusters } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { bytes, compact, num, rate, ago, duration } from '$lib/format';
	import Stat from '$lib/components/Stat.svelte';
	import Chart from '$lib/components/Chart.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Modal from '$lib/components/Modal.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const snap = $derived(live.snapshot);
	const h = $derived(live.history);
	const times = $derived(h.map((p) => new Date(p.t).getTime()));
	let refreshing = $state(false);
	let renameOpen = $state(false);
	let newName = $state('');

	async function refresh() {
		refreshing = true;
		try {
			await live.refresh();
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			refreshing = false;
		}
	}

	async function rename(e: SubmitEvent) {
		e.preventDefault();
		try {
			await unwrap(api.PUT('/v1/clusters/{cluster}/cluster-name', { params: { path: { cluster: live.id } }, body: { name: newName } }));
			await live.refresh();
			await refreshClusters();
			toast('Cluster renamed');
			renameOpen = false;
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	const rateRows = $derived(
		snap
			? [
					['Publish', snap.rates.publish, 'Messages received from publishers'],
					['Publisher confirm', snap.rates.confirm, 'Publishes confirmed to publishers'],
					['Deliver (manual ack)', snap.rates.deliver, 'Deliveries to consumers that acknowledge'],
					['Deliver (auto ack)', snap.rates.deliverNoAck, 'Deliveries to consumers in no-ack mode'],
					['Consumer ack', snap.rates.ack, ''],
					['Redelivered', snap.rates.redeliver, 'Deliveries of messages delivered before'],
					['Get (manual ack)', snap.rates.get, 'basic.get with acknowledgement'],
					['Get (auto ack)', snap.rates.getNoAck, ''],
					['Get (empty)', snap.rates.getEmpty, 'basic.get on an empty queue'],
					['Unroutable (return)', snap.rates.returnUnroutable, 'Mandatory messages returned to publishers'],
					['Unroutable (drop)', snap.rates.dropUnroutable, 'Messages routed nowhere and dropped'],
					['Disk read', snap.rates.diskReads, 'Messages read from disk by queues'],
					['Disk write', snap.rates.diskWrites, 'Messages written to disk by queues']
				]
			: []
	);
</script>

<svelte:head><title>{cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<h1>{cluster?.name ?? live.id}</h1>
			<div class="sub">
				{#if snap?.info.clusterName}
					RabbitMQ cluster <b class="mono">{snap.info.clusterName}</b>
					{#if can('admin')}<button
							class="btn ghost sm icon"
							title="Rename the RabbitMQ cluster"
							aria-label="Rename the RabbitMQ cluster"
							onclick={() => ((newName = snap?.info.clusterName ?? ''), (renameOpen = true))}><Icon name="edit" size={13} /></button
						>{/if}
					·
				{/if}
				{#if snap?.info.rabbitmqVersion}{snap.info.productName} {snap.info.versions.join(', ') || snap.info.rabbitmqVersion} · Erlang {snap.info.erlangVersion} ·{/if}
				{#if snap}updated {ago(snap.time)}{/if}
				{#if live.connected}<span class="badge ok" style="margin-left:6px"><span class="dot ok"></span>live</span>{/if}
			</div>
		</div>
		<span class="spacer"></span>
		<button class="btn" onclick={refresh} disabled={refreshing}><Icon name="refresh" />Refresh</button>
	</div>

	{#if !snap}
		<Loading text="Polling the cluster…" />
	{:else}
		{#if snap.error}
			<div class="alert bad" style="margin-bottom:14px"><b>Cannot reach the management API:</b> {snap.error}</div>
		{/if}
		{#if snap.info.statsDisabled && snap.ratesSource !== 'prometheus'}
			<div class="alert warn" style="margin-bottom:14px">The management statistics database is disabled on this cluster. Add Prometheus URLs in settings to see message rates.</div>
		{/if}

		<div class="kpis">
			<Stat
				label="Nodes running"
				value="{snap.totals.nodesRunning} / {snap.totals.nodes}"
				tone={snap.totals.nodesRunning < snap.totals.nodes || snap.alarms.length ? 'bad' : snap.nodes.some((n) => !n.healthy) ? 'warn' : 'ok'}
				sub={snap.alarms.length ? `${snap.alarms.length} resource alarm${snap.alarms.length === 1 ? '' : 's'}` : `${snap.nodes.filter((n) => n.healthy).length} healthy`}
			/>
			<Stat label="Connections" value={num(snap.totals.connections)} spark={h.map((p) => p.connections)} sub="{num(snap.totals.channels)} channels" />
			<Stat label="Queues" value={num(snap.totals.queues)} spark={h.map((p) => p.queues)} sub="{num(snap.totals.exchanges)} exchanges" />
			<Stat label="Consumers" value={num(snap.totals.consumers)} spark={h.map((p) => p.consumers)} />
			<Stat label="Queued messages" value={compact(snap.totals.messages)} spark={h.map((p) => p.ready + p.unacked)} sub="{compact(snap.totals.messagesReady)} ready · {compact(snap.totals.messagesUnacked)} unacked" />
			<Stat label="Publish" value={rate(snap.rates.publish)} spark={h.map((p) => p.publish)} sub="deliver {rate(snap.rates.deliverGet)}" />
		</div>

		<div class="grid-2" style="margin-top:14px">
			<div class="card">
				<div class="card-head">
					<h2>Message rates</h2>
					<span class="spacer"></span>
					<span class="badge" title="Where the rates come from">{snap.ratesSource === 'management' ? 'management stats' : snap.ratesSource === 'prometheus' ? 'Prometheus' : 'no rates'}</span>
				</div>
				<div class="card-body">
					<Chart
						{times}
						format={(n) => compact(n)}
						series={[
							{ name: 'publish /s', values: h.map((p) => p.publish), color: 'var(--series-1)' },
							{ name: 'deliver /s', values: h.map((p) => p.deliver), color: 'var(--series-2)' },
							{ name: 'ack /s', values: h.map((p) => p.ack), color: 'var(--series-3)' },
							{ name: 'confirm /s', values: h.map((p) => p.confirm), color: 'var(--series-4)' },
							{ name: 'unroutable /s', values: h.map((p) => p.unroutable), color: 'var(--bad)' }
						]}
					/>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>Queued messages</h2></div>
				<div class="card-body">
					<Chart
						{times}
						format={(n) => compact(n)}
						series={[
							{ name: 'ready', values: h.map((p) => p.ready), color: 'var(--series-1)' },
							{ name: 'unacked', values: h.map((p) => p.unacked), color: 'var(--series-2)' }
						]}
					/>
				</div>
			</div>
		</div>

		<div class="split" style="margin-top:14px">
			<div class="stack">
				<div class="card">
					<div class="card-head">
						<h2>Nodes</h2>
						<span class="spacer"></span>
						<a class="btn sm" href="/clusters/{live.id}/nodes">Details<Icon name="chevronRight" size={14} /></a>
					</div>
					<div class="table-wrap">
						<table class="t">
							<thead><tr><th>Node</th><th>Memory</th><th>Disk free</th><th>File descriptors</th><th>Uptime</th></tr></thead>
							<tbody>
								{#each snap.nodes as n (n.name)}
									<tr>
										<td>
											<div class="row" style="gap:6px;flex-wrap:nowrap">
												<span class="dot {!n.running ? 'bad' : n.healthy ? 'ok' : 'warn'}"></span>
												<a class="nowrap" href="/clusters/{live.id}/nodes/{encodeURIComponent(n.name)}" title={n.name}><b>{n.name.replace(/^rabbit@/, '')}</b></a>
												{#if n.beingDrained}<span class="badge warn">maintenance</span>{/if}
											</div>
										</td>
										{#if n.running}
											<td style="min-width:130px">
												<div class="faint small nowrap">{bytes(n.memUsed)} / {bytes(n.memLimit)}</div>
												<Meter used={n.memUsed} total={n.memLimit} />
											</td>
											<td class="nowrap">
												{#if n.diskAlarm}<span class="badge bad">alarm</span>{/if}
												{bytes(n.diskFree)} <span class="faint small">limit {bytes(n.diskFreeLimit)}</span>
											</td>
											<td style="min-width:110px">
												<div class="faint small">{num(n.fdUsed)} / {num(n.fdTotal)}</div>
												<Meter used={n.fdUsed} total={n.fdTotal} />
											</td>
											<td class="nowrap">{duration(n.uptimeMs)}</td>
										{:else}
											<td colspan="4"><span class="badge bad">not running</span></td>
										{/if}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>

				<div class="card">
					<div class="card-head">
						<h2>Deepest queues</h2>
						<span class="faint small">top {Math.min(10, snap.queues.length)} by messages, plus queues with problems</span>
					</div>
					<div class="table-wrap">
						<table class="t">
							<thead
								><tr><th>Queue</th><th>Type</th><th class="r">Ready</th><th class="r">Unacked</th><th class="r">Consumers</th><th class="r">In/s</th><th class="r">Out/s</th><th>Members</th></tr></thead
							>
							<tbody>
								{#each snap.queues as q (q.vhost + '/' + q.name)}
									<tr>
										<td><span class="faint">{q.vhost} /</span> <b>{q.name}</b>{#if q.state && q.state !== 'running'} <span class="badge warn">{q.state}</span>{/if}</td>
										<td><span class="badge {q.type === 'quorum' ? 'accent' : q.type === 'stream' ? 'info' : ''}">{q.type}</span></td>
										<td class="r num">{compact(q.ready)}</td>
										<td class="r num">{compact(q.unacked)}</td>
										<td class="r num">{#if q.consumers === 0 && q.ready > 0 && q.type !== 'stream'}<span class="badge warn">0</span>{:else}{q.consumers}{/if}</td>
										<td class="r num">{rate(q.publishRate)}</td>
										<td class="r num">{rate(q.deliverRate)}</td>
										<td class="nowrap">
											{#if q.members.length}
												<span class="badge {q.online.length < q.members.length ? 'warn' : ''}" title={q.members.join('\n')}>{q.online.length}/{q.members.length} online</span>
											{:else}<span class="faint">{q.node?.replace(/^rabbit@/, '') ?? ''}</span>{/if}
										</td>
									</tr>
								{:else}
									<tr><td colspan="8" class="faint">No queues.</td></tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			</div>

			<div class="stack">
				<div class="card">
					<div class="card-head">
						<h2>Attention</h2>
						<span class="spacer"></span>
						{#if snap.attention.length === 0}<span class="badge ok">All clear</span>{:else}<span class="badge warn">{snap.attention.length}</span>{/if}
					</div>
					{#if snap.attention.length === 0}
						<div class="card-body faint">Every node is running and healthy, with no alarms, and no queue needs a look.</div>
					{:else}
						<div class="card-body stack" style="gap:8px">
							{#each snap.attention as a}
								<div class="attn {a.severity}">
									<span class="dot {a.severity}"></span>
									<div class="min">
										<div class="ellipsis"><b>{a.subject}</b></div>
										<div class="muted small">{a.message}</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</div>

				<div class="card">
					<div class="card-head"><h2>All message rates</h2></div>
					<table class="t">
						<tbody>
							{#each rateRows as [label, v, hint]}
								<tr title={String(hint)}><td>{label}</td><td class="r num">{rate(Number(v))}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>

				<div class="card">
					<div class="card-head"><h2>Churn</h2><span class="spacer"></span><span class="faint small">per second</span></div>
					<div class="card-body">
						<Chart
							{times}
							height={150}
							format={(n) => compact(n)}
							series={[
								{ name: 'conn opened', values: h.map((p) => p.connCreated), color: 'var(--series-1)' },
								{ name: 'conn closed', values: h.map((p) => p.connClosed), color: 'var(--series-2)' },
								{ name: 'chan opened', values: h.map((p) => p.chanCreated), color: 'var(--series-3)' },
								{ name: 'chan closed', values: h.map((p) => p.chanClosed), color: 'var(--series-4)' },
								{ name: 'queue created', values: h.map((p) => p.queueCreated), color: 'var(--warn)' },
								{ name: 'queue deleted', values: h.map((p) => p.queueDeleted), color: 'var(--bad)' }
							]}
						/>
						<div class="churn faint small">
							<span>connections +{rate(snap.rates.connCreated)} −{rate(snap.rates.connClosed)}</span>
							<span>channels +{rate(snap.rates.chanCreated)} −{rate(snap.rates.chanClosed)}</span>
							<span>queues +{rate(snap.rates.queueCreated)} −{rate(snap.rates.queueDeleted)}</span>
						</div>
					</div>
				</div>
			</div>
		</div>

		<div class="grid-2" style="margin-top:14px">
			<div class="card">
				<div class="card-head"><h2>Listening ports</h2></div>
				<div class="table-wrap">
					<table class="t">
						<thead><tr><th>Node</th><th>Protocol</th><th>Bound to</th><th class="r">Port</th><th>TLS</th></tr></thead>
						<tbody>
							{#each snap.listeners as l}
								<tr><td>{l.node}</td><td>{l.protocol}</td><td class="mono">{l.ip}</td><td class="r num">{l.port}</td><td>{l.tls ? 'yes' : ''}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
			<div class="card">
				<div class="card-head"><h2>Web contexts</h2></div>
				<div class="table-wrap">
					<table class="t">
						<thead><tr><th>Node</th><th>Context</th><th>Path</th><th class="r">Port</th><th>TLS</th></tr></thead>
						<tbody>
							{#each snap.contexts as c}
								<tr><td>{c.node}</td><td>{c.description}</td><td class="mono">{c.path}</td><td class="r num">{c.port}</td><td>{c.tls ? 'yes' : ''}</td></tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	{/if}
</div>

<Modal bind:open={renameOpen} title="Rename the RabbitMQ cluster">
	<form id="renameform" class="stack" onsubmit={rename}>
		<p class="muted" style="margin:0">Sets the <code>cluster_name</code> RabbitMQ reports to clients and in metrics. rabmin's own label for this cluster is changed in settings.</p>
		<label class="field"><span>Cluster name</span><input bind:value={newName} required /></label>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (renameOpen = false)}>Cancel</button>
		<button class="btn primary" form="renameform">Rename</button>
	{/snippet}
</Modal>

<style>
	.split {
		display: grid;
		grid-template-columns: minmax(0, 1.6fr) minmax(300px, 1fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1100px) {
		.split {
			grid-template-columns: 1fr;
		}
	}
	.small {
		font-size: 12px;
	}
	.min {
		min-width: 0;
	}
	.attn {
		display: flex;
		gap: 9px;
		align-items: flex-start;
		padding: 8px 10px;
		border-radius: var(--radius-sm);
		background: var(--bg-sunken);
	}
	.attn .dot {
		margin-top: 6px;
	}
	.churn {
		display: flex;
		gap: 14px;
		flex-wrap: wrap;
		margin-top: 6px;
	}
</style>
