<script lang="ts">
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { errMsg, type Schemas } from '$lib/api/client';
	import { toast } from '$lib/toast.svelte';
	import { ago } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import JsonView from '$lib/components/JsonView.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const snap = $derived(live.snapshot);
	let refreshing = $state(false);
	let shown = $state<Schemas['HealthResult'] | null>(null);
	let open = $state(false);

	const describe: Record<string, string> = {
		'local-alarms': 'No resource alarm on this node',
		alarms: 'No resource alarm anywhere in the cluster',
		'virtual-hosts': 'Every virtual host is running on this node',
		'is-in-service': 'Booted, running and not in maintenance mode',
		'ready-to-serve-clients': 'In service, listeners up and below the connection limit',
		'below-node-connection-limit': 'Fewer connections than the configured maximum',
		'node-is-quorum-critical': 'Shutting this node down would not make any quorum queue or stream lose its majority',
		'certificate-expiration': 'No listener certificate expires within a month',
		'protocol-listeners': "Every protocol the node advertises has an active listener"
	};

	const cell = (node: string, check: string) => snap?.health.find((h) => h.node === node && h.check === check);
	const nodes = $derived(snap?.nodes ?? []);
	const failing = $derived((snap?.health ?? []).filter((h) => h.status !== 'ok').length);

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
</script>

<svelte:head><title>Health · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Health</h1>
			<div class="sub">
				RabbitMQ's health checks, run on every node through its own management URL.
				{#if snap}{failing === 0 ? 'All passing' : `${failing} failing`} · checked {ago(snap.time)}{/if}
			</div>
		</div>
		<span class="spacer"></span>
		<button class="btn" onclick={refresh} disabled={refreshing}><Icon name="refresh" />Run now</button>
	</div>

	{#if !snap}
		<Loading />
	{:else}
		{#if snap.error}<div class="alert bad" style="margin-bottom:14px">{snap.error}</div>{/if}
		<div class="card table-wrap">
			<table class="t matrix">
				<thead>
					<tr>
						<th>Check</th>
						{#each nodes as n}<th class="c"><span class="dot {!n.running ? 'bad' : n.healthy ? 'ok' : 'warn'}"></span> {n.name.replace(/^rabbit@/, '')}</th>{/each}
					</tr>
				</thead>
				<tbody>
					{#each snap.healthChecks as check}
						<tr>
							<td>
								<b class="mono">{check}</b>
								<div class="faint small">{describe[check] ?? ''}</div>
							</td>
							{#each nodes as n}
								{@const r = cell(n.name, check)}
								<td class="c">
									{#if !r}
										<span class="faint" title={n.running ? 'No configured management URL reaches this node' : 'Node is down'}>{n.running ? 'no endpoint' : '—'}</span>
									{:else if r.status === 'ok'}
										<span class="badge ok"><Icon name="check" size={12} />ok</span>
									{:else}
										<button class="badge {r.status === 'failed' ? 'bad' : 'warn'} as-btn" onclick={() => ((shown = r), (open = true))} title={r.reason}>
											{r.status}
										</button>
									{/if}
								</td>
							{/each}
						</tr>
					{:else}
						<tr><td class="faint">No health results yet.</td></tr>
					{/each}
				</tbody>
			</table>
		</div>
		{#if snap.endpoints.some((e) => !e.ok) || nodes.some((n) => n.running && !n.endpoint)}
			<div class="alert warn" style="margin-top:14px">
				Some nodes cannot be checked: {nodes.filter((n) => n.running && !n.endpoint).map((n) => n.name).join(', ') || 'an endpoint is unreachable'}. Add one management URL
				per node in the cluster settings.
			</div>
		{/if}
		<div class="card" style="margin-top:14px">
			<div class="card-head"><h2>Management endpoints</h2></div>
			<table class="t">
				<thead><tr><th>URL</th><th>Node</th><th>Status</th><th class="r">Latency</th></tr></thead>
				<tbody>
					{#each snap.endpoints as e}
						<tr>
							<td class="mono">{e.url}</td><td>{e.node ?? '—'}</td>
							<td>{#if e.ok}<span class="badge ok">reachable</span>{:else}<span class="badge bad">unreachable</span> <span class="faint small">{e.error}</span>{/if}</td>
							<td class="r num">{e.latencyMs} ms</td>
						</tr>
					{/each}
					{#each snap.prometheus as p}
						<tr>
							<td class="mono">{p.url}</td><td>{p.node ?? '—'}</td>
							<td>{#if p.ok}<span class="badge ok">Prometheus</span>{:else}<span class="badge bad">Prometheus failing</span> <span class="faint small">{p.error}</span>{/if}</td>
							<td></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open title="{shown?.check} on {shown?.node}" width={720}>
	{#if shown}
		<div class="stack">
			<div class="alert {shown.status === 'failed' ? 'bad' : 'warn'}">{shown.reason || shown.status}</div>
			{#if shown.detail}<JsonView value={shown.detail} maxHeight={380} />{/if}
		</div>
	{/if}
</Modal>

<style>
	.matrix th.c,
	.matrix td.c {
		text-align: center;
		white-space: nowrap;
	}
	.small {
		font-size: 12px;
	}
	.as-btn {
		cursor: pointer;
		border: none;
		font: inherit;
	}
</style>
