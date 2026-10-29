<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import JsonEditor from '$lib/components/JsonEditor.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';

	type Param = Schemas['Parameter'];
	type GParam = Schemas['GlobalParameter'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let params = $state<Param[] | null>(null);
	let globals = $state<GParam[] | null>(null);
	let tags = $state<{ k: string; v: string }[]>([]);
	let component = $state('');
	let error = $state<string | null>(null);
	const reserved = ['cluster_name', 'internal_cluster_id', 'cluster_tags'];

	async function load() {
		try {
			params = await unwrap(api.GET('/v1/clusters/{cluster}/parameters', { params: { path: { cluster: live.id } } }));
			globals = await unwrap(api.GET('/v1/clusters/{cluster}/global-parameters', { params: { path: { cluster: live.id } } }));
			const t = await unwrap(api.GET('/v1/clusters/{cluster}/cluster-tags', { params: { path: { cluster: live.id } } }));
			tags = Object.entries(t.tags).map(([k, v]) => ({ k, v }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(load);
	const components = $derived([...new Set((params ?? []).map((p) => p.component))].sort());
	const shown = $derived((params ?? []).filter((p) => !component || p.component === component));

	// Editor for both kinds.
	let open = $state(false);
	let mode = $state<'vhost' | 'global'>('vhost');
	let isNew = $state(true);
	let f = $state({ component: '', vhost: '/', name: '' });
	let value = $state('{}');
	let valueErr = $state<string | null>(null);
	function edit(kind: 'vhost' | 'global', p: Param | GParam | null) {
		mode = kind;
		isNew = !p;
		f = { component: (p as Param)?.component ?? component ?? '', vhost: (p as Param)?.vhost ?? '/', name: p?.name ?? '' };
		value = p ? JSON.stringify(p.value, null, 2) : '{}';
		open = true;
	}
	async function save(e: SubmitEvent) {
		e.preventDefault();
		let v: unknown;
		try {
			v = JSON.parse(value);
		} catch (err) {
			toast('Value: ' + errMsg(err), 'bad');
			return;
		}
		try {
			if (mode === 'vhost') {
				await unwrap(
					api.PUT('/v1/clusters/{cluster}/parameters/{component}/{vhost}/{name}', {
						params: { path: { cluster: live.id, component: f.component, vhost: f.vhost, name: f.name } },
						body: { value: v as Record<string, never> }
					})
				);
			} else {
				await unwrap(
					api.PUT('/v1/clusters/{cluster}/global-parameters/{name}', { params: { path: { cluster: live.id, name: f.name } }, body: { value: v as Record<string, never> } })
				);
			}
			toast(`Saved ${f.name}`);
			open = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
	async function remove(kind: 'vhost' | 'global', p: Param | GParam) {
		if (!(await confirm({ title: 'Delete parameter', message: `Delete ${p.name}?`, confirmText: 'Delete', danger: true }))) return;
		try {
			if (kind === 'vhost') {
				const x = p as Param;
				await unwrap(
					api.DELETE('/v1/clusters/{cluster}/parameters/{component}/{vhost}/{name}', { params: { path: { cluster: live.id, component: x.component, vhost: x.vhost, name: x.name } } })
				);
			} else {
				await unwrap(api.DELETE('/v1/clusters/{cluster}/global-parameters/{name}', { params: { path: { cluster: live.id, name: p.name } } }));
			}
			toast(`Deleted ${p.name}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	async function saveTags() {
		const out: Record<string, string> = {};
		for (const t of tags) if (t.k.trim()) out[t.k.trim()] = t.v;
		try {
			await unwrap(api.PUT('/v1/clusters/{cluster}/cluster-tags', { params: { path: { cluster: live.id } }, body: { tags: out } }));
			toast('Cluster tags saved');
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Parameters · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Parameters</h1>
			<div class="sub">Runtime parameters of plugins and limits, global parameters and cluster tags. Passwords in URIs are shown as ***.</div>
		</div>
		<span class="spacer"></span>
		<button class="btn" onclick={load}><Icon name="refresh" />Refresh</button>
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !params || !globals}
		<Loading />
	{:else}
		<div class="split">
			<div class="card">
				<div class="card-head">
					<h2>Vhost parameters</h2>
					<span class="spacer"></span>
					<select bind:value={component} style="width:auto">
						<option value="">All components</option>
						{#each components as c}<option value={c}>{c}</option>{/each}
					</select>
					{#if can('admin')}<button class="btn sm" onclick={() => edit('vhost', null)}><Icon name="plus" size={13} />Add</button>{/if}
				</div>
				{#if shown.length === 0}
					<Empty icon="kv" title="No parameters" />
				{:else}
					<table class="t">
						<thead><tr><th>Component</th><th>Vhost</th><th>Name</th><th>Value</th>{#if can('admin')}<th></th>{/if}</tr></thead>
						<tbody>
							{#each shown as p (p.component + p.vhost + p.name)}
								<tr>
									<td><span class="badge">{p.component}</span></td>
									<td class="mono">{p.vhost}</td>
									<td><b>{p.name}</b></td>
									<td class="val"><JsonView value={p.value} maxHeight={140} /></td>
									{#if can('admin')}
										<td class="r nowrap">
											<button class="btn sm" aria-label="Edit" onclick={() => edit('vhost', p)}><Icon name="edit" size={13} /></button>
											<button class="btn sm danger" aria-label="Delete" onclick={() => remove('vhost', p)}><Icon name="trash" size={13} /></button>
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>
			<div class="stack">
				<div class="card">
					<div class="card-head"><h2>Cluster tags</h2></div>
					<div class="card-body stack">
						{#each tags as t, i}
							<div class="trow">
								<input bind:value={t.k} placeholder="key" disabled={!can('admin')} />
								<input bind:value={t.v} placeholder="value" disabled={!can('admin')} />
								{#if can('admin')}<button class="btn sm ghost icon" aria-label="Remove" onclick={() => tags.splice(i, 1)}><Icon name="x" size={13} /></button>{/if}
							</div>
						{:else}
							<span class="faint">No tags.</span>
						{/each}
						{#if can('admin')}
							<div class="row">
								<button class="btn sm" onclick={() => tags.push({ k: '', v: '' })}><Icon name="plus" size={13} />Add tag</button>
								<button class="btn sm primary" onclick={saveTags}>Save tags</button>
							</div>
						{/if}
					</div>
				</div>
				<div class="card">
					<div class="card-head">
						<h2>Global parameters</h2>
						<span class="spacer"></span>
						{#if can('admin')}<button class="btn sm" onclick={() => edit('global', null)}><Icon name="plus" size={13} />Add</button>{/if}
					</div>
					<table class="t">
						<tbody>
							{#each globals as g (g.name)}
								<tr>
									<td><b>{g.name}</b>{#if reserved.includes(g.name)}<div class="faint small">managed by RabbitMQ{g.name === 'cluster_tags' ? ' (edit above)' : ''}</div>{/if}</td>
									<td class="val"><JsonView value={g.value} maxHeight={120} /></td>
									{#if can('admin')}
										<td class="r nowrap">
											{#if !reserved.includes(g.name)}
												<button class="btn sm" aria-label="Edit" onclick={() => edit('global', g)}><Icon name="edit" size={13} /></button>
												<button class="btn sm danger" aria-label="Delete" onclick={() => remove('global', g)}><Icon name="trash" size={13} /></button>
											{/if}
										</td>
									{/if}
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	{/if}
</div>

<Modal bind:open title={isNew ? (mode === 'vhost' ? 'Add vhost parameter' : 'Add global parameter') : `Edit ${f.name}`} width={680}>
	<form id="parform" class="stack" onsubmit={save}>
		{#if mode === 'vhost'}
			<div class="grid-3">
				<label class="field"><span>Component</span><input bind:value={f.component} required disabled={!isNew} list="comps" placeholder="federation-upstream" /></label>
				<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={f.vhost} /></label>
				<label class="field"><span>Name</span><input bind:value={f.name} required disabled={!isNew} /></label>
			</div>
			<datalist id="comps">{#each ['federation-upstream', 'federation-upstream-set', 'shovel', ...components] as c}<option value={c}></option>{/each}</datalist>
		{:else}
			<label class="field"><span>Name</span><input bind:value={f.name} required disabled={!isNew} /></label>
		{/if}
		<div class="field">
			<span>Value (JSON)</span>
			<JsonEditor bind:value bind:error={valueErr} rows={12} />
			{#if valueErr}<span class="hint" style="color:var(--bad)">{valueErr}</span>{/if}
			{#if value.includes(':***@')}<span class="hint" style="color:var(--warn)">Replace *** with the real password before saving.</span>{/if}
		</div>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="parform">Save</button>
	{/snippet}
</Modal>

<style>
	.split {
		display: grid;
		grid-template-columns: minmax(0, 1.5fr) minmax(320px, 1fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1100px) {
		.split {
			grid-template-columns: 1fr;
		}
	}
	.val {
		max-width: 420px;
	}
	.trow {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 6px;
	}
	.small {
		font-size: 12px;
	}
</style>
