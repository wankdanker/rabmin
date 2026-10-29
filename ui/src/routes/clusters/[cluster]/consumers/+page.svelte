<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { num, duration } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Pager from '$lib/components/Pager.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));

	let q = $state(page.url.searchParams.get('q') ?? '');
	let vhost = $state('');
	let queue = $state(page.url.searchParams.get('queue') ?? '');
	let sort = $state('queue');
	let desc = $state(false);
	let pageNo = $state(1);
	const pageSize = 100;
	let auto = $state(true);
	let data = $state<Schemas['ConsumersPage'] | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(false);

	async function load() {
		loading = true;
		try {
			data = await unwrap(
				api.GET('/v1/clusters/{cluster}/consumers', {
					params: { path: { cluster: live.id }, query: { q: q || undefined, vhost: vhost || undefined, queue: queue || undefined, sort, desc, page: pageNo, pageSize } }
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
			desc = k === 'prefetch';
		}
		apply();
	}
	const cols: [string, string, boolean][] = [
		['queue', 'Queue', false],
		['clientName', 'Client', false],
		['tag', 'Consumer tag', false],
		['ackRequired', 'Ack mode', false],
		['prefetch', 'Prefetch', true],
		['status', 'Status', false]
	];
</script>

<svelte:head><title>Consumers · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Consumers</h1>
			<div class="sub">{#if data}{num(data.filtered)} of {num(data.total)} consumers{/if}</div>
		</div>
		<span class="spacer"></span>
		<label class="check"><input type="checkbox" bind:checked={auto} />Auto refresh</label>
		<button class="btn" onclick={() => load()} disabled={loading}><Icon name="refresh" />Refresh</button>
	</div>

	<form class="card filters" onsubmit={apply}>
		<label class="field"><span>Search</span><input type="search" bind:value={q} placeholder="queue, client, tag" /></label>
		<label class="field"><span>Virtual host</span><input bind:value={vhost} /></label>
		<label class="field"><span>Queue</span><input bind:value={queue} placeholder="exact name" /></label>
		<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
	</form>

	{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

	<div class="card table-wrap" style="margin-top:14px">
		{#if !data}
			<Loading />
		{:else if data.items.length === 0}
			<Empty icon="inbox" title="No consumers match" />
		{:else}
			<table class="t">
				<thead>
					<tr>
						{#each cols as [k, label, right]}
							<th class="sortable" class:r={right} onclick={() => sortBy(k)}>{label}{#if sort === k}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th>
						{/each}
						<th>Channel</th>
						<th>Arguments</th>
					</tr>
				</thead>
				<tbody>
					{#each data.items as k (k.channel + k.tag)}
						<tr>
							<td><span class="faint">{k.vhost} /</span> <b>{k.queue}</b>{#if k.exclusive} <span class="badge">exclusive</span>{/if}</td>
							<td>{k.clientName || '(unnamed)'}<div class="faint small">{k.user} · {k.node}</div></td>
							<td class="mono small">{k.tag}</td>
							<td>{k.ackRequired ? 'manual' : 'auto'}</td>
							<td class="r num">{k.prefetch || 'unlimited'}</td>
							<td>
								<span class="badge {k.status === 'waiting' ? 'warn' : k.active ? 'ok' : ''}" title={k.status === 'waiting' ? 'Waiting: another consumer is the single active consumer' : ''}
									>{k.status === 'single_active' ? 'single active' : k.status}</span
								>
								{#if k.timeoutMs}<div class="faint small">timeout {duration(k.timeoutMs)}</div>{/if}
							</td>
							<td><a class="mono small" href="/clusters/{live.id}/channels?q={encodeURIComponent(k.channel)}">{k.channel}</a></td>
							<td class="mono small">{JSON.stringify(k.arguments) === '{}' ? '' : JSON.stringify(k.arguments)}</td>
						</tr>
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
		grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.small {
		font-size: 11.5px;
	}
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
</style>
