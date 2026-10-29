<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { policyKeys, appliesTo, coerce, seg } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';
	import JsonEditor from '$lib/components/JsonEditor.svelte';

	type Policy = Schemas['Policy'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let operator = $state(false);
	let vhost = $state('');
	let list = $state<Policy[] | null>(null);
	let error = $state<string | null>(null);

	async function load() {
		try {
			const opts = { params: { path: { cluster: live.id }, query: { vhost: vhost || undefined } } };
			list = operator
				? await unwrap(api.GET('/v1/clusters/{cluster}/operator-policies', opts))
				: await unwrap(api.GET('/v1/clusters/{cluster}/policies', opts));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(load);

	// Editor.
	let open = $state(false);
	let editing = $state<Policy | null>(null);
	let f = $state({ vhost: '/', name: '', pattern: '.*', applyTo: 'queues', priority: 0 });
	let defRows = $state<{ key: string; value: string }[]>([]);
	let rawMode = $state(false);
	let raw = $state('{}');
	let rawErr = $state<string | null>(null);
	let preview = $state<Schemas['PolicyPreview'] | null>(null);
	let previewErr = $state<string | null>(null);

	const kinds = $derived(appliesTo(f.applyTo));
	const keyOptions = $derived(policyKeys.filter((k) => (!operator || k.operator) && k.applies.some((a) => kinds.includes(a))));
	const spec = (key: string) => policyKeys.find((k) => k.key === key);

	function definition(): Record<string, unknown> {
		if (rawMode) return JSON.parse(raw || '{}');
		const d: Record<string, unknown> = {};
		for (const r of defRows) {
			if (!r.key) continue;
			const s = spec(r.key);
			d[r.key] = s ? coerce(s.kind, r.value) : r.value;
		}
		return d;
	}

	function edit(p: Policy | null) {
		editing = p;
		f = { vhost: p?.vhost ?? (vhost || '/'), name: p?.name ?? '', pattern: p?.pattern ?? '.*', applyTo: p?.applyTo ?? 'queues', priority: p?.priority ?? 0 };
		const d = (p?.definition ?? {}) as Record<string, unknown>;
		defRows = Object.entries(d).map(([key, v]) => ({ key, value: typeof v === 'string' ? v : JSON.stringify(v) }));
		if (!defRows.length) defRows = [{ key: '', value: '' }];
		raw = JSON.stringify(d, null, 2);
		rawMode = false;
		preview = null;
		open = true;
		runPreview();
	}
	function toggleRaw() {
		try {
			if (!rawMode) raw = JSON.stringify(definition(), null, 2);
			else {
				const d = JSON.parse(raw || '{}') as Record<string, unknown>;
				defRows = Object.entries(d).map(([key, v]) => ({ key, value: typeof v === 'string' ? v : JSON.stringify(v) }));
			}
			rawMode = !rawMode;
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	let previewTimer: ReturnType<typeof setTimeout> | null = null;
	function runPreview() {
		if (previewTimer) clearTimeout(previewTimer);
		previewTimer = setTimeout(async () => {
			if (!f.pattern || !f.vhost) return;
			try {
				preview = await unwrap(
					api.POST('/v1/clusters/{cluster}/policies/preview', {
						params: { path: { cluster: live.id } },
						body: { vhost: f.vhost, name: f.name || undefined, pattern: f.pattern, applyTo: f.applyTo as 'all', priority: Number(f.priority) || 0, operator }
					})
				);
				previewErr = null;
			} catch (e) {
				previewErr = errMsg(e);
			}
		}, 300);
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		let def: Record<string, unknown>;
		try {
			def = definition();
		} catch (err) {
			toast('Definition: ' + errMsg(err), 'bad');
			return;
		}
		const opts = { params: { path: { cluster: live.id, vhost: f.vhost, name: f.name } }, body: { pattern: f.pattern, applyTo: f.applyTo as 'all', priority: Number(f.priority) || 0, definition: def } };
		try {
			if (operator) await unwrap(api.PUT('/v1/clusters/{cluster}/operator-policies/{vhost}/{name}', opts));
			else await unwrap(api.PUT('/v1/clusters/{cluster}/policies/{vhost}/{name}', opts));
			toast(`Saved ${f.name}`);
			open = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	async function remove(p: Policy) {
		if (!(await confirm({ title: `Delete ${operator ? 'operator policy' : 'policy'}`, message: `Delete ${p.name} in ${p.vhost}? The settings it applies are removed from the matching objects.`, confirmText: 'Delete', danger: true })))
			return;
		try {
			const opts = { params: { path: { cluster: live.id, vhost: p.vhost, name: p.name } } };
			if (operator) await unwrap(api.DELETE('/v1/clusters/{cluster}/operator-policies/{vhost}/{name}', opts));
			else await unwrap(api.DELETE('/v1/clusters/{cluster}/policies/{vhost}/{name}', opts));
			toast(`Deleted ${p.name}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Policies · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Policies</h1>
			<div class="sub">
				{operator ? 'Operator policies cap what users can set: the stricter value of a policy and an operator policy wins.' : 'Policies set queue and exchange options by name pattern; the highest priority match applies.'}
			</div>
		</div>
		<span class="spacer"></span>
		<div class="seg">
			<button class:on={!operator} onclick={() => ((operator = false), load())}>Policies</button>
			<button class:on={operator} onclick={() => ((operator = true), load())}>Operator policies</button>
		</div>
		<label class="field" style="min-width:170px"><VHostSelect cluster={live.id} bind:value={vhost} all onchange={load} /></label>
		{#if can('admin')}<button class="btn primary" onclick={() => edit(null)}><Icon name="plus" />Add {operator ? 'operator policy' : 'policy'}</button>{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	<div class="card table-wrap">
		{#if !list}
			<Loading />
		{:else if list.length === 0}
			<Empty icon="filter" title="No {operator ? 'operator policies' : 'policies'}" />
		{:else}
			<table class="t">
				<thead><tr><th>Name</th><th>Vhost</th><th>Pattern</th><th>Applies to</th><th class="r">Priority</th><th>Definition</th>{#if can('admin')}<th></th>{/if}</tr></thead>
				<tbody>
					{#each list as p (p.vhost + '/' + p.name)}
						<tr>
							<td><b>{p.name}</b></td>
							<td class="mono">{p.vhost}</td>
							<td class="mono">{p.pattern}</td>
							<td>{p.applyTo}</td>
							<td class="r num">{p.priority}</td>
							<td>
								{#each Object.entries(p.definition as Record<string, unknown>) as [k, v]}<span class="badge"><b>{k}</b>: {typeof v === 'string' ? v : JSON.stringify(v)}</span> {/each}
							</td>
							{#if can('admin')}
								<td class="r nowrap">
									<a class="btn sm" href="/clusters/{live.id}/queues?vhost={seg(p.vhost)}&policy={seg(p.name)}" title="Queues it applies to"><Icon name="queue" size={13} /></a>
									<button class="btn sm" aria-label="Edit" onclick={() => edit(p)}><Icon name="edit" size={13} /></button>
									<button class="btn sm danger" aria-label="Delete" onclick={() => remove(p)}><Icon name="trash" size={13} /></button>
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<Modal bind:open title={editing ? `Edit ${editing.name}` : operator ? 'Add operator policy' : 'Add policy'} width={820}>
	<form id="pform" class="stack" onsubmit={save}>
		<div class="grid-3">
			<label class="field"><span>Virtual host</span><VHostSelect cluster={live.id} bind:value={f.vhost} onchange={runPreview} /></label>
			<label class="field"><span>Name</span><input bind:value={f.name} required disabled={!!editing} /></label>
			<label class="field"><span>Priority</span><input type="number" min="0" bind:value={f.priority} oninput={runPreview} /></label>
		</div>
		<div class="grid-2">
			<label class="field"><span>Pattern</span><input bind:value={f.pattern} class="mono" required oninput={runPreview} placeholder="^orders\." /></label>
			<label class="field"
				><span>Applies to</span><select bind:value={f.applyTo} onchange={runPreview}>
					{#each ['all', 'queues', 'classic_queues', 'quorum_queues', 'streams', 'exchanges'] as a}<option value={a}>{a}</option>{/each}
				</select></label
			>
		</div>
		<div class="field">
			<span class="row" style="gap:8px">Definition <button type="button" class="btn sm ghost" onclick={toggleRaw}>{rawMode ? 'Form' : 'JSON'}</button></span>
			{#if rawMode}
				<JsonEditor bind:value={raw} bind:error={rawErr} rows={8} />
				{#if rawErr}<span class="hint" style="color:var(--bad)">{rawErr}</span>{/if}
			{:else}
				<div class="stack" style="gap:6px">
					{#each defRows as r, i}
						{@const s = spec(r.key)}
						<div class="drow">
							<select bind:value={r.key}>
								<option value="">choose a key…</option>
								{#each keyOptions as k}<option value={k.key}>{k.key}</option>{/each}
								{#if r.key && !keyOptions.some((k) => k.key === r.key)}<option value={r.key}>{r.key} (not for {f.applyTo})</option>{/if}
							</select>
							{#if s?.kind === 'choice'}
								<select bind:value={r.value}>{#each s.choices ?? [] as c}<option value={c}>{c}</option>{/each}</select>
							{:else}
								<input bind:value={r.value} type={s?.kind === 'int' ? 'number' : 'text'} placeholder="value" />
							{/if}
							<button type="button" class="btn sm ghost icon" aria-label="Remove" onclick={() => defRows.splice(i, 1)}><Icon name="x" size={13} /></button>
						</div>
					{/each}
					<div><button type="button" class="btn sm" onclick={() => defRows.push({ key: '', value: '' })}><Icon name="plus" size={13} />Add key</button></div>
				</div>
			{/if}
			<span class="hint">RabbitMQ validates the definition and rejects unknown keys.{operator ? ' Operator policies accept only limits such as max-length, message-ttl and delivery-limit.' : ''}</span>
		</div>
		<div class="card preview">
			<div class="card-head"><h3>Would apply to</h3><span class="spacer"></span>{#if preview}<span class="faint small">{preview.matches.length} object{preview.matches.length === 1 ? '' : 's'}</span>{/if}</div>
			<div class="card-body">
				{#if previewErr}<div class="alert bad">{previewErr}</div>{/if}
				{#if preview?.note}<div class="alert warn">{preview.note}</div>{/if}
				{#if preview && preview.matches.length === 0 && !preview.note}<span class="faint">Nothing in {f.vhost} matches yet.</span>{/if}
				{#if preview && preview.matches.length}
					<table class="t">
						<tbody>
							{#each preview.matches.slice(0, 50) as m}
								<tr>
									<td><span class="badge">{m.kind === 'queue' ? m.type : 'exchange'}</span> {m.name}</td>
									<td class="small">{m.current ? `now: ${m.current}` : ''}</td>
									<td class="r">
										{#if !m.wins}<span class="badge warn" title="A higher-priority policy matches">outranked</span>
										{:else if m.tie}<span class="badge warn" title="Same priority as {m.tie}; RabbitMQ picks one">tie with {m.tie}</span>
										{:else}<span class="badge ok">applies</span>{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>
		</div>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="pform">Save</button>
	{/snippet}
</Modal>

<style>
	.seg {
		display: flex;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}
	.seg button {
		border: none;
		background: var(--bg-elev);
		color: var(--text-2);
		padding: 6px 10px;
		font: inherit;
		cursor: pointer;
	}
	.seg button.on {
		background: var(--accent-soft);
		color: var(--accent);
		font-weight: 600;
	}
	.drow {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 6px;
	}
	.preview {
		max-height: 260px;
		overflow: auto;
	}
	.small {
		font-size: 12px;
	}
</style>
