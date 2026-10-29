<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { bytes, compact, num, rate } from '$lib/format';
	import { queueArgs, coerce, seg, type QueueType } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Pager from '$lib/components/Pager.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';
	import KeyValueEditor from '$lib/components/KeyValueEditor.svelte';

	type Queue = Schemas['Queue'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const sp = page.url.searchParams;
	let vhost = $state(sp.get('vhost') ?? '');
	let q = $state(sp.get('q') ?? '');
	let regex = $state('');
	let type = $state(sp.get('type') ?? '');
	let qstate = $state('');
	let policy = $state('');
	let sort = $state('messages');
	let desc = $state(true);
	let pageNo = $state(1);
	let pageSize = $state(100);
	let auto = $state(true);
	let data = $state<Schemas['QueuesPage'] | null>(null);
	let error = $state<string | null>(null);

	// Column chooser, remembered per browser.
	const allCols = [
		['type', 'Type'],
		['state', 'State'],
		['policy', 'Policy'],
		['ready', 'Ready'],
		['unacked', 'Unacked'],
		['messages', 'Total'],
		['messageBytes', 'Bytes'],
		['consumers', 'Consumers'],
		['publishRate', 'In/s'],
		['deliverRate', 'Out/s'],
		['ackRate', 'Ack/s'],
		['members', 'Members'],
		['memory', 'Memory']
	] as const;
	type Col = (typeof allCols)[number][0];
	const defaultCols: Col[] = ['type', 'state', 'policy', 'ready', 'unacked', 'consumers', 'publishRate', 'deliverRate', 'members'];
	let cols = $state<Col[]>(readCols());
	let chooser = $state(false);
	function readCols(): Col[] {
		try {
			const v = JSON.parse(localStorage.getItem('rabmin.queueCols') ?? 'null');
			if (Array.isArray(v)) return v;
		} catch {}
		return [...defaultCols];
	}
	function toggleCol(c: Col) {
		cols = cols.includes(c) ? cols.filter((x) => x !== c) : allCols.map(([k]) => k).filter((k) => k === c || cols.includes(k));
		try {
			localStorage.setItem('rabmin.queueCols', JSON.stringify(cols));
		} catch {}
	}

	async function load() {
		try {
			data = await unwrap(
				api.GET('/v1/clusters/{cluster}/queues', {
					params: {
						path: { cluster: live.id },
						query: {
							vhost: vhost || undefined,
							q: q || undefined,
							regex: regex || undefined,
							type: type || undefined,
							state: qstate || undefined,
							policy: policy || undefined,
							sort,
							desc,
							page: pageNo,
							pageSize
						}
					}
				})
			);
			error = null;
		} catch (e) {
			error = errMsg(e);
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
			desc = !['name', 'vhost', 'type', 'state', 'policy'].includes(k);
		}
		apply();
	}

	// Create.
	let open = $state(false);
	let f = $state({ vhost: '', name: '', type: 'quorum' as QueueType, durable: true, autoDelete: false });
	let argVals = $state<Record<string, string>>({});
	let extra = $state<Record<string, unknown>>({});
	let creating = $state(false);
	const typeArgs = $derived(queueArgs.filter((a) => a.types.includes(f.type)));
	function openCreate() {
		f = { vhost: vhost || '/', name: '', type: 'quorum', durable: true, autoDelete: false };
		argVals = {};
		extra = {};
		open = true;
	}
	async function create(e: SubmitEvent) {
		e.preventDefault();
		const args: Record<string, unknown> = { ...extra };
		for (const a of typeArgs) {
			const v = argVals[a.key];
			if (v !== undefined && v !== '') args[a.key] = coerce(a.kind, v);
		}
		creating = true;
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/queues/{vhost}/{name}', {
					params: { path: { cluster: live.id, vhost: f.vhost, name: f.name } },
					body: { type: f.type, durable: f.type === 'classic' ? f.durable : true, autoDelete: f.autoDelete, arguments: args }
				})
			);
			toast(`Declared ${f.name}`);
			open = false;
			goto(`/clusters/${live.id}/queues/${seg(f.vhost)}/${seg(f.name)}`);
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			creating = false;
		}
	}

	const stateTone = (s: string) => (s === 'running' || s === 'idle' ? '' : s === 'minority' || s === 'down' || s === 'crashed' ? 'bad' : 'warn');
</script>

<svelte:head><title>Queues · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Queues &amp; streams</h1>
			<div class="sub">{#if data}{num(data.filtered)} of {num(data.total)} · rates averaged over 30 s{/if}</div>
		</div>
		<span class="spacer"></span>
		<label class="check"><input type="checkbox" bind:checked={auto} />Auto refresh</label>
		<button class="btn" onclick={() => (chooser = !chooser)}><Icon name="layers" />Columns</button>
		{#if can('operator')}<button class="btn primary" onclick={openCreate}><Icon name="plus" />Add queue</button>{/if}
	</div>

	<form class="card filters" onsubmit={apply}>
		<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={vhost} all onchange={() => apply()} /></label>
		<label class="field"><span>Name</span><input type="search" bind:value={q} placeholder="contains…" /></label>
		<label class="field"><span>Regex</span><input bind:value={regex} placeholder="^orders\." class="mono" /></label>
		<label class="field"
			><span>Type</span><select bind:value={type} onchange={() => apply()}>
				<option value="">All</option><option value="classic">classic</option><option value="quorum">quorum</option><option value="stream">stream</option>
			</select></label
		>
		<label class="field"
			><span>State</span><select bind:value={qstate} onchange={() => apply()}>
				<option value="">Any</option>
				{#each ['running', 'idle', 'minority', 'down', 'crashed', 'stopped'] as s}<option value={s}>{s}</option>{/each}
			</select></label
		>
		<label class="field"><span>Policy</span><input bind:value={policy} placeholder="name, or - for none" /></label>
		<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
	</form>

	{#if chooser}
		<div class="card chooser">
			{#each allCols as [k, label]}<label class="check"><input type="checkbox" checked={cols.includes(k)} onchange={() => toggleCol(k)} />{label}</label>{/each}
		</div>
	{/if}

	{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

	<div class="card table-wrap" style="margin-top:14px">
		{#if !data}
			<Loading />
		{:else if data.items.length === 0}
			<Empty icon="queue" title="No queues match" />
		{:else}
			<table class="t">
				<thead>
					<tr>
						<th class="sortable" onclick={() => sortBy('name')}>Name{#if sort === 'name'}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th>
						{#each allCols.filter(([k]) => cols.includes(k)) as [k, label]}
							<th class="sortable" class:r={!['type', 'state', 'policy', 'members'].includes(k)} onclick={() => sortBy(k)}
								>{label}{#if sort === k}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th
							>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each data.items as x (x.vhost + '/' + x.name)}
						<tr class="clickable" onclick={() => goto(`/clusters/${live.id}/queues/${seg(x.vhost)}/${seg(x.name)}`)}>
							<td>
								{#if !vhost}<span class="faint">{x.vhost} /</span>{/if}
								<b>{x.name}</b>
								{#if x.exclusive}<span class="badge" title="exclusive">excl</span>{/if}
								{#if x.autoDelete}<span class="badge" title="auto-delete">AD</span>{/if}
								{#if !x.durable}<span class="badge warn" title="transient">transient</span>{/if}
							</td>
							{#each allCols.filter(([k]) => cols.includes(k)) as [k]}
								{#if k === 'type'}<td><span class="badge {x.type === 'quorum' ? 'accent' : x.type === 'stream' ? 'info' : ''}">{x.type}</span></td>
								{:else if k === 'state'}<td><span class="badge {stateTone(x.state)}">{x.state}</span></td>
								{:else if k === 'policy'}<td class="small">{x.policy ?? ''}{#if x.operatorPolicy}<div class="faint">op: {x.operatorPolicy}</div>{/if}</td>
								{:else if k === 'ready'}<td class="r num">{compact(x.ready)}</td>
								{:else if k === 'unacked'}<td class="r num">{compact(x.unacked)}</td>
								{:else if k === 'messages'}<td class="r num">{compact(x.messages)}</td>
								{:else if k === 'messageBytes'}<td class="r num">{bytes(x.messageBytes)}</td>
								{:else if k === 'consumers'}<td class="r num"
										>{#if x.consumers === 0 && x.ready > 0 && x.type !== 'stream'}<span class="badge warn" title="Messages waiting and nobody consuming">0</span>{:else}{x.consumers}{/if}</td
									>
								{:else if k === 'publishRate'}<td class="r num">{rate(x.publishRate)}</td>
								{:else if k === 'deliverRate'}<td class="r num">{rate(x.deliverRate)}</td>
								{:else if k === 'ackRate'}<td class="r num">{rate(x.ackRate)}</td>
								{:else if k === 'memory'}<td class="r num">{bytes(x.memory)}</td>
								{:else if k === 'members'}<td class="nowrap small">
										{#if x.members.length}<span class="badge {x.online.length < x.members.length ? 'warn' : ''}" title={x.members.join('\n')}>{x.online.length}/{x.members.length} online</span>
											<span class="faint">leader {x.leader?.replace(/^rabbit@/, '')}</span>
										{:else}<span class="faint">{x.node?.replace(/^rabbit@/, '')}</span>{/if}
									</td>
								{/if}
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	{#if data}
		<div class="row">
			<Pager page={pageNo} {pageSize} total={data.filtered} onpage={(p) => ((pageNo = p), load())} />
			<span class="spacer"></span>
			<label class="faint small" style="margin-top:10px"
				>per page <select bind:value={pageSize} onchange={() => apply()}><option value={50}>50</option><option value={100}>100</option><option value={500}>500</option></select></label
			>
		</div>
	{/if}
</div>

<Modal bind:open title="Add queue or stream" width={720}>
	<form id="qform" class="stack" onsubmit={create}>
		<div class="grid-3">
			<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={f.vhost} /></label>
			<label class="field"><span>Name</span><input bind:value={f.name} required /></label>
			<label class="field"
				><span>Type</span><select bind:value={f.type}>
					<option value="quorum">quorum (replicated)</option><option value="classic">classic</option><option value="stream">stream</option>
				</select></label
			>
		</div>
		{#if f.type === 'classic'}
			<div class="row">
				<label class="check"><input type="checkbox" bind:checked={f.durable} />Durable</label>
				<label class="check"><input type="checkbox" bind:checked={f.autoDelete} />Auto-delete</label>
			</div>
			{#if !f.durable}<div class="alert warn">RabbitMQ 4.x refuses non-durable queues unless the deprecated transient_nonexcl_queues feature is permitted.</div>{/if}
		{/if}
		<div class="args">
			{#each typeArgs as a (a.key)}
				<label class="field">
					<span>{a.label} <code class="faint">{a.key}</code></span>
					{#if a.kind === 'bool'}
						<select bind:value={argVals[a.key]}><option value="">not set</option><option value="true">true</option><option value="false">false</option></select>
					{:else if a.kind === 'choice'}
						<select bind:value={argVals[a.key]}><option value="">not set</option>{#each a.choices ?? [] as c}<option value={c}>{c}</option>{/each}</select>
					{:else}
						<input bind:value={argVals[a.key]} type={a.kind === 'int' ? 'number' : 'text'} placeholder={a.placeholder ?? ''} />
					{/if}
					{#if a.hint}<span class="hint">{a.hint}</span>{/if}
				</label>
			{/each}
		</div>
		<div class="field"><span>Other arguments</span><KeyValueEditor bind:value={extra} keyPlaceholder="x-argument" /></div>
		<p class="faint small" style="margin:0">Arguments cannot be changed after declaring; use policies for settings you may want to change later.</p>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="qform" disabled={creating}>Declare</button>
	{/snippet}
</Modal>

<style>
	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.chooser {
		margin-top: 10px;
		padding: 10px 14px;
		display: flex;
		flex-wrap: wrap;
		gap: 14px;
	}
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
	.small {
		font-size: 12px;
	}
	.args {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
		gap: 10px;
	}
</style>
