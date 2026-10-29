<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { num, rate } from '$lib/format';
	import { seg, exName } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Pager from '$lib/components/Pager.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';
	import KeyValueEditor from '$lib/components/KeyValueEditor.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let vhost = $state(page.url.searchParams.get('vhost') ?? '');
	let q = $state('');
	let type = $state('');
	let internal = $state(false);
	let sort = $state('name');
	let desc = $state(false);
	let pageNo = $state(1);
	const pageSize = 100;
	let data = $state<Schemas['ExchangesPage'] | null>(null);
	let error = $state<string | null>(null);

	async function load() {
		try {
			data = await unwrap(
				api.GET('/v1/clusters/{cluster}/exchanges', {
					params: { path: { cluster: live.id }, query: { vhost: vhost || undefined, q: q || undefined, type: type || undefined, internal, sort, desc, page: pageNo, pageSize } }
				})
			);
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(() => {
		load();
		const t = setInterval(() => !document.hidden && load(), 10000);
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
			desc = k === 'rateIn' || k === 'rateOut';
		}
		apply();
	}

	let open = $state(false);
	let f = $state({ vhost: '', name: '', type: 'topic', durable: true, autoDelete: false, internal: false, alternate: '' });
	let extra = $state<Record<string, unknown>>({});
	async function create(e: SubmitEvent) {
		e.preventDefault();
		const args: Record<string, unknown> = { ...extra };
		if (f.alternate) args['alternate-exchange'] = f.alternate;
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/exchanges/{vhost}/{name}', {
					params: { path: { cluster: live.id, vhost: f.vhost, name: f.name } },
					body: { type: f.type, durable: f.durable, autoDelete: f.autoDelete, internal: f.internal, arguments: args }
				})
			);
			toast(`Declared ${f.name}`);
			open = false;
			goto(`/clusters/${live.id}/exchanges/${seg(f.vhost)}/${seg(f.name)}`);
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
	function openCreate() {
		f = { vhost: vhost || '/', name: '', type: 'topic', durable: true, autoDelete: false, internal: false, alternate: '' };
		extra = {};
		open = true;
	}
	const cols: [string, string, boolean][] = [
		['name', 'Name', false],
		['vhost', 'Virtual host', false],
		['type', 'Type', false],
		['rateIn', 'In', true],
		['rateOut', 'Out', true]
	];
</script>

<svelte:head><title>Exchanges · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Exchanges</h1>
			<div class="sub">{#if data}{num(data.filtered)} exchanges · rates averaged over 30 s{/if}</div>
		</div>
		<span class="spacer"></span>
		{#if can('operator')}<button class="btn primary" onclick={openCreate}><Icon name="plus" />Add exchange</button>{/if}
	</div>

	<form class="card filters" onsubmit={apply}>
		<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={vhost} all onchange={() => apply()} /></label>
		<label class="field"><span>Name</span><input type="search" bind:value={q} placeholder="contains…" /></label>
		<label class="field"
			><span>Type</span><select bind:value={type} onchange={() => apply()}>
				<option value="">All</option>
				{#each ['direct', 'fanout', 'topic', 'headers', 'x-local-random', 'x-modulus-hash', 'x-consistent-hash'] as t}<option value={t}>{t}</option>{/each}
			</select></label
		>
		<div class="field"><span>&nbsp;</span><label class="check"><input type="checkbox" bind:checked={internal} onchange={() => apply()} />Show amq.* and internal</label></div>
		<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
	</form>
	{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

	<div class="card table-wrap" style="margin-top:14px">
		{#if !data}
			<Loading />
		{:else if data.items.length === 0}
			<Empty icon="swap" title="No exchanges match" />
		{:else}
			<table class="t">
				<thead>
					<tr>
						{#each cols as [k, label, right]}
							<th class="sortable" class:r={right} onclick={() => sortBy(k)}>{label}{#if sort === k}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th>
						{/each}
						<th>Features</th><th>Policy</th>
					</tr>
				</thead>
				<tbody>
					{#each data.items as x (x.vhost + '/' + x.name)}
						<tr class="clickable" onclick={() => goto(`/clusters/${live.id}/exchanges/${seg(x.vhost)}/${seg(x.name || 'amq.default')}`)}>
							<td><b>{exName(x.name)}</b></td>
							<td class="mono">{x.vhost}</td>
							<td><span class="badge">{x.type}</span></td>
							<td class="r num">{rate(x.rateIn)}</td>
							<td class="r num">{rate(x.rateOut)}</td>
							<td>
								{#if x.durable}<span class="badge" title="durable">D</span>{/if}
								{#if x.autoDelete}<span class="badge" title="auto-delete">AD</span>{/if}
								{#if x.internal}<span class="badge" title="internal">I</span>{/if}
								{#each Object.keys(x.arguments ?? {}) as a}<span class="badge info" title={JSON.stringify((x.arguments as Record<string, unknown>)[a])}>{a.replace(/^x-/, '')}</span>{/each}
							</td>
							<td>{x.policy ?? ''}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	{#if data}<Pager page={pageNo} {pageSize} total={data.filtered} onpage={(p) => ((pageNo = p), load())} />{/if}
</div>

<Modal bind:open title="Add exchange">
	<form id="exform" class="stack" onsubmit={create}>
		<div class="grid-2">
			<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={f.vhost} /></label>
			<label class="field"><span>Name</span><input bind:value={f.name} required pattern="^(?!amq\.).*" title="amq.* names are reserved" /></label>
		</div>
		<div class="grid-2">
			<label class="field"
				><span>Type</span><select bind:value={f.type}>
					{#each ['direct', 'fanout', 'topic', 'headers', 'x-local-random', 'x-modulus-hash', 'x-consistent-hash'] as t}<option value={t}>{t}</option>{/each}
				</select></label
			>
			<label class="field"><span>Alternate exchange</span><input bind:value={f.alternate} placeholder="for unroutable messages" /></label>
		</div>
		<div class="row">
			<label class="check"><input type="checkbox" bind:checked={f.durable} />Durable</label>
			<label class="check"><input type="checkbox" bind:checked={f.autoDelete} />Auto-delete</label>
			<label class="check"><input type="checkbox" bind:checked={f.internal} />Internal</label>
		</div>
		<div class="field"><span>Other arguments</span><KeyValueEditor bind:value={extra} /></div>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="exform">Declare</button>
	{/snippet}
</Modal>

<style>
	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
</style>
