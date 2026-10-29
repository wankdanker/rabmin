<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { toast } from '$lib/toast.svelte';
	import { num, rate } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Pager from '$lib/components/Pager.svelte';

	type Chan = Schemas['Channel'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));

	let q = $state(page.url.searchParams.get('q') ?? '');
	let vhost = $state('');
	let user = $state('');
	let node = $state('');
	let sort = $state('deliverRate');
	let desc = $state(true);
	let pageNo = $state(1);
	const pageSize = 50;
	let auto = $state(true);
	let data = $state<Schemas['ChannelsPage'] | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(false);
	let expanded = $state<string | null>(null);
	let detail = $state<Schemas['ChannelDetail'] | null>(null);
	const nodes = $derived(live.snapshot?.nodes.map((n) => n.name) ?? []);

	async function load() {
		loading = true;
		try {
			data = await unwrap(
				api.GET('/v1/clusters/{cluster}/channels', {
					params: { path: { cluster: live.id }, query: { q: q || undefined, vhost: vhost || undefined, user: user || undefined, node: node || undefined, sort, desc, page: pageNo, pageSize } }
				})
			);
			error = null;
		} catch (e) {
			error = errMsg(e);
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		load();
		const t = setInterval(() => auto && !document.hidden && load(), 5000);
		return () => clearInterval(t);
	});
	function apply(e?: Event) {
		e?.preventDefault();
		pageNo = 1;
		load();
	}
	function sortBy(k: string) {
		if (sort === k) desc = !desc;
		else {
			sort = k;
			desc = !['clientName', 'user', 'vhost', 'node'].includes(k);
		}
		apply();
	}
	async function toggle(c: Chan) {
		if (expanded === c.name) {
			expanded = null;
			return;
		}
		expanded = c.name;
		detail = null;
		try {
			detail = await unwrap(api.GET('/v1/clusters/{cluster}/channels/{name}', { params: { path: { cluster: live.id, name: c.name } } }));
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	// Unacked at the prefetch limit means consumers are saturated: they get no more messages until they ack.
	const saturated = (c: Chan) => c.prefetch > 0 && c.consumers > 0 && c.unacked >= c.prefetch * c.consumers;

	const cols: [string, string, boolean][] = [
		['clientName', 'Channel', false],
		['user', 'User / vhost', false],
		['node', 'Node', false],
		['consumers', 'Consumers', true],
		['prefetch', 'Prefetch', true],
		['unacked', 'Unacked', true],
		['unconfirmed', 'Unconfirmed', true],
		['publishRate', 'Publish', true],
		['deliverRate', 'Deliver', true],
		['ackRate', 'Ack', true]
	];
</script>

<svelte:head><title>Channels · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Channels</h1>
			<div class="sub">{#if data}{num(data.filtered)} of {num(data.total)} channels · rates averaged over 30 s{/if}</div>
		</div>
		<span class="spacer"></span>
		<label class="check"><input type="checkbox" bind:checked={auto} />Auto refresh</label>
		<button class="btn" onclick={() => load()} disabled={loading}><Icon name="refresh" />Refresh</button>
	</div>

	<form class="card filters" onsubmit={apply}>
		<label class="field"><span>Search</span><input type="search" bind:value={q} placeholder="client or channel name" /></label>
		<label class="field"><span>Virtual host</span><input bind:value={vhost} /></label>
		<label class="field"><span>User</span><input bind:value={user} /></label>
		<label class="field"
			><span>Node</span><select bind:value={node} onchange={() => apply()}>
				<option value="">All nodes</option>
				{#each nodes as n}<option value={n}>{n}</option>{/each}
			</select></label
		>
		<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
	</form>

	{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

	<div class="card table-wrap" style="margin-top:14px">
		{#if !data}
			<Loading />
		{:else if data.items.length === 0}
			<Empty icon="layers" title="No channels match" />
		{:else}
			<table class="t">
				<thead>
					<tr>
						{#each cols as [k, label, right]}
							<th class="sortable" class:r={right} onclick={() => sortBy(k)}>{label}{#if sort === k}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th>
						{/each}
						<th>Mode</th>
					</tr>
				</thead>
				<tbody>
					{#each data.items as c (c.name)}
						<tr class="clickable" onclick={() => toggle(c)}>
							<td class="chan"><b>{c.clientName || '(unnamed)'}</b> <span class="faint">#{c.number}</span><div class="faint small mono ellipsis" title={c.connection}>{c.connection}</div></td>
							<td>{c.user}<div class="faint small">{c.vhost}</div></td>
							<td class="nowrap">{c.node}</td>
							<td class="r num">{c.consumers}</td>
							<td class="r num">{c.prefetch || '—'}{#if c.globalPrefetch}<div class="faint small">{c.globalPrefetch} global</div>{/if}</td>
							<td class="r num">{#if saturated(c)}<span class="badge warn" title="Unacked has reached prefetch × consumers: consumers get nothing more until they ack">{num(c.unacked)}</span>{:else}{num(c.unacked)}{/if}</td>
							<td class="r num">{num(c.unconfirmed)}</td>
							<td class="r num">{rate(c.publishRate)}</td>
							<td class="r num">{rate(c.deliverRate)}</td>
							<td class="r num">{rate(c.ackRate)}</td>
							<td class="nowrap">
								{#if c.confirm}<span class="badge info">confirm</span>{/if}
								{#if c.transactional}<span class="badge accent">tx</span>{/if}
								{#if c.unroutableRate > 0}<span class="badge warn" title="Publishing unroutable messages">unroutable {rate(c.unroutableRate)}</span>{/if}
								{#if c.state !== 'running'}<span class="badge">{c.state}</span>{/if}
							</td>
						</tr>
						{#if expanded === c.name}
							<tr class="detail">
								<td colspan="11">
									{#if !detail}<Loading />{:else}
										<div class="grid-2">
											<div>
												<div class="label" style="margin-bottom:4px">Consumers ({detail.consumers.length})</div>
												{#if detail.consumers.length}
													<table class="t inner">
														<thead><tr><th>Queue</th><th>Tag</th><th>Ack</th><th class="r">Prefetch</th><th>Status</th></tr></thead>
														<tbody>
															{#each detail.consumers as k}
																<tr>
																	<td><span class="faint">{k.vhost} /</span> {k.queue}</td>
																	<td class="mono small">{k.tag}</td>
																	<td>{k.ackRequired ? 'manual' : 'auto'}</td>
																	<td class="r num">{k.prefetch}</td>
																	<td><span class="badge {k.active ? 'ok' : 'warn'}">{k.status}</span></td>
																</tr>
															{/each}
														</tbody>
													</table>
												{:else}<span class="faint">No consumers on this channel.</span>{/if}
											</div>
											<div class="stack">
												<div>
													<div class="label" style="margin-bottom:4px">Publishing to</div>
													{#each detail.publishes as p}<div>{p.vhost} / <b>{p.name || '(default)'}</b> <span class="faint">{rate(p.rate)}</span></div>{:else}<span
															class="faint">Per-exchange rates need <code>rates_mode = detailed</code> in the management plugin.</span
														>{/each}
												</div>
												<div>
													<div class="label" style="margin-bottom:4px">Delivering from</div>
													{#each detail.deliveries as p}<div>{p.vhost} / <b>{p.name}</b> <span class="faint">{rate(p.rate)}</span></div>{:else}<span class="faint">—</span>{/each}
												</div>
											</div>
										</div>
									{/if}
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	{#if data}<Pager page={pageNo} {pageSize} total={data.filtered} onpage={(p) => ((pageNo = p), load())} />{/if}
</div>

<style>
	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.detail td {
		background: var(--bg-sunken) !important;
	}
	.small {
		font-size: 11.5px;
	}
	.chan {
		max-width: 300px;
	}
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
	table.inner {
		background: var(--bg-elev);
		border: 1px solid var(--border);
	}
</style>
