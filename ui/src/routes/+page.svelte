<script lang="ts">
	import { onMount } from 'svelte';
	import { clusters, refreshClusters, clusterState } from '$lib/clusters.svelte';
	import { can } from '$lib/session.svelte';
	import { rate, compact, ago } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';

	onMount(() => {
		const t = setInterval(() => !document.hidden && refreshClusters().catch(() => {}), 5000);
		return () => clearInterval(t);
	});

	const totals = $derived(
		clusters.list.reduce(
			(a, c) => ({
				nodes: a.nodes + c.status.nodes,
				running: a.running + c.status.nodesRunning,
				conns: a.conns + c.status.connections,
				queues: a.queues + c.status.queues,
				messages: a.messages + c.status.messages,
				publish: a.publish + c.status.publishRate
			}),
			{ nodes: 0, running: 0, conns: 0, queues: 0, messages: 0, publish: 0 }
		)
	);
	const unhealthy = $derived(clusters.list.filter((c) => clusterState(c).tone === 'bad').length);
</script>

<svelte:head><title>Clusters · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<h1>Clusters</h1>
			<div class="sub">
				{#if clusters.list.length}
					{clusters.list.length} cluster{clusters.list.length === 1 ? '' : 's'} · {totals.running}/{totals.nodes} nodes running · {compact(totals.conns)} connections ·
					{compact(totals.messages)} messages queued · {rate(totals.publish)} published
					{#if unhealthy}<span class="badge bad" style="margin-left:6px">{unhealthy} unhealthy</span>{/if}
				{:else}Every RabbitMQ cluster rabmin manages{/if}
			</div>
		</div>
		<span class="spacer"></span>
		{#if can('admin')}<a href="/clusters/new" class="btn primary"><Icon name="plus" />Add cluster</a>{/if}
	</div>

	{#if clusters.list.length === 0}
		<div class="card">
			<Empty icon="topology" title="No clusters yet">
				{#if can('admin')}
					Register a RabbitMQ cluster (or a single node) by its management API URLs.
					<div style="margin-top:14px"><a href="/clusters/new" class="btn primary"><Icon name="plus" />Add your first cluster</a></div>
				{:else}
					Ask an admin to register a RabbitMQ cluster.
				{/if}
			</Empty>
		</div>
	{:else}
		<div class="grid">
			{#each clusters.list as c (c.id)}
				{@const st = clusterState(c)}
				{@const s = c.status}
				<a href="/clusters/{c.id}" class="card sys">
					<div class="top">
						<div class="min">
							<h2 class="ellipsis">{c.name}</h2>
							<div class="faint ellipsis small">
								{#if s.clusterName}<span class="mono">{s.clusterName}</span> · {/if}{c.managementUrls.length} management URL{c.managementUrls.length === 1 ? '' : 's'}
							</div>
						</div>
						<span class="badge {st.tone}"><span class="dot {st.tone}"></span>{st.text}</span>
					</div>
					{#if c.description}<p class="muted desc">{c.description}</p>{/if}
					<div class="metrics">
						<div><span class="faint">Nodes</span><b class="num">{s.nodesRunning}/{s.nodes}</b></div>
						<div><span class="faint">Connections</span><b class="num">{compact(s.connections)}</b></div>
						<div><span class="faint">Queues</span><b class="num">{compact(s.queues)}</b></div>
						<div><span class="faint">Messages</span><b class="num">{compact(s.messages)}</b></div>
						<div><span class="faint">Publish</span><b class="num">{rate(s.publishRate)}</b></div>
						<div><span class="faint">Deliver</span><b class="num">{rate(s.deliverRate)}</b></div>
					</div>
					{#if s.error}<div class="alert bad small">{s.error}</div>{/if}
					<div class="foot faint small">
						{#each s.versions as v}<span class="badge">RabbitMQ {v}</span>{/each}
						{#if s.alarms}<span class="badge bad">{s.alarms} alarm{s.alarms === 1 ? '' : 's'}</span>{/if}
						{#if s.attention}<span class="badge warn">{s.attention} to look at</span>{/if}
						{#if s.ratesSource === 'prometheus'}<span class="badge info" title="Rates computed from Prometheus counters">Prometheus</span>{/if}
						<span class="spacer"></span>
						{#if s.lastPoll}<span>polled {ago(s.lastPoll)}</span>{/if}
					</div>
				</a>
			{/each}
		</div>
	{/if}
</div>

<style>
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
		gap: 14px;
	}
	.sys {
		padding: 16px;
		color: var(--text);
		text-decoration: none;
		display: flex;
		flex-direction: column;
		gap: 12px;
		transition: border-color 0.15s;
	}
	.sys:hover {
		border-color: var(--accent);
		text-decoration: none;
	}
	.top {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		justify-content: space-between;
	}
	.min {
		min-width: 0;
	}
	.small {
		font-size: 12px;
	}
	.desc {
		margin: 0;
		font-size: 13px;
	}
	.metrics {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 10px 8px;
	}
	.metrics div {
		display: flex;
		flex-direction: column;
		font-size: 12px;
	}
	.metrics b {
		font-size: 17px;
		font-weight: 600;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 6px;
		flex-wrap: wrap;
		margin-top: auto;
	}
</style>
