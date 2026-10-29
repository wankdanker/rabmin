<script lang="ts">
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, rawFetch, type Schemas } from '$lib/api/client';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';

	type Change = Schemas['DefinitionChange'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));

	// Export
	let exportVHost = $state('');
	let exporting = $state(false);
	async function download() {
		exporting = true;
		try {
			const q = exportVHost ? '?vhost=' + encodeURIComponent(exportVHost) : '';
			const res = await rawFetch(`/v1/clusters/${encodeURIComponent(live.id)}/definitions${q}`);
			const blob = await res.blob();
			const cd = res.headers.get('Content-Disposition') ?? '';
			const file = /filename="?([^";]+)"?/.exec(cd)?.[1] ?? `${live.id}-definitions.json`;
			const a = document.createElement('a');
			a.href = URL.createObjectURL(blob);
			a.download = file;
			a.click();
			URL.revokeObjectURL(a.href);
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			exporting = false;
		}
	}

	// Import
	let text = $state('');
	let fileName = $state('');
	let importVHost = $state('');
	let diff = $state<Schemas['DefinitionsDiff'] | null>(null);
	let selected = $state<Record<string, boolean>>({});
	let showUnchanged = $state(false);
	let busy = $state(false);
	let expanded = $state<string | null>(null);

	function parsed(): unknown {
		return JSON.parse(text);
	}
	async function pick(e: Event) {
		const f = (e.target as HTMLInputElement).files?.[0];
		if (!f) return;
		fileName = f.name;
		text = await f.text();
		diff = null;
	}
	async function compare() {
		let defs: unknown;
		try {
			defs = parsed();
		} catch (e) {
			toast('Not valid JSON: ' + errMsg(e), 'bad');
			return;
		}
		busy = true;
		try {
			diff = await unwrap(
				api.POST('/v1/clusters/{cluster}/definitions/diff', {
					params: { path: { cluster: live.id } },
					body: { definitions: defs, vhost: importVHost || undefined }
				})
			);
			selected = Object.fromEntries(diff.changes.filter(importable).map((c) => [c.id, true]));
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}
	const importable = (c: Change) => c.change === 'added' || c.change === 'changed';
	const rows = $derived((diff?.changes ?? []).filter((c) => showUnchanged || c.change !== 'unchanged'));
	const chosen = $derived(Object.entries(selected).filter(([, v]) => v).map(([k]) => k));
	const allImportable = $derived((diff?.changes ?? []).filter(importable));
	function toggleAll(on: boolean) {
		selected = Object.fromEntries(allImportable.map((c) => [c.id, on]));
	}
	async function runImport() {
		if (!chosen.length) return;
		if (
			!(await confirm({
				title: 'Import definitions',
				message: `Import ${chosen.length} object${chosen.length === 1 ? '' : 's'} into ${cluster?.name ?? live.id}${importVHost ? ` (vhost ${importVHost})` : ''}? Changed objects are overwritten.`,
				confirmText: 'Import'
			}))
		)
			return;
		busy = true;
		try {
			const r = await unwrap(
				api.POST('/v1/clusters/{cluster}/definitions/import', {
					params: { path: { cluster: live.id } },
					body: { definitions: parsed(), vhost: importVHost || undefined, select: chosen }
				})
			);
			toast(`Imported ${r.imported} object${r.imported === 1 ? '' : 's'}`);
			await compare();
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}
	const badge: Record<string, string> = { added: 'ok', changed: 'warn', conflict: 'bad', unchanged: '', 'cluster-only': 'info' };
	const order = ['added', 'changed', 'conflict', 'cluster-only', 'unchanged'];
</script>

<svelte:head><title>Definitions · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Definitions</h1>
			<div class="sub">Export the topology of the cluster or one virtual host, and import a definitions file after reviewing what it would change.</div>
		</div>
	</div>

	<div class="grid-2">
		<div class="card">
			<div class="card-head"><h2>Export</h2></div>
			<div class="card-body stack">
				<label class="field"><span>Scope</span><VHostSelect cluster={live.id} bind:value={exportVHost} all /></label>
				<p class="faint small">The file contains users with their password hashes. Keep it as safe as the broker's credentials.</p>
				<div class="row"><button class="btn primary" onclick={download} disabled={exporting}><Icon name="download" />Download JSON</button></div>
			</div>
		</div>
		<div class="card">
			<div class="card-head"><h2>Import</h2></div>
			<div class="card-body stack">
				<div class="grid-2">
					<label class="field">
						<span>File</span>
						<input type="file" accept=".json,application/json" onchange={pick} />
					</label>
					<label class="field"><span>File is an export of</span><VHostSelect cluster={live.id} bind:value={importVHost} all /></label>
				</div>
				<label class="field">
					<span>Or paste JSON{fileName ? ` (loaded ${fileName})` : ''}</span>
					<textarea class="mono" rows="6" bind:value={text} oninput={() => (diff = null)} placeholder={'{"queues": [], "exchanges": [], ...}'}></textarea>
				</label>
				<p class="faint small">
					Import never deletes. Queues and exchanges that exist with other properties are skipped by RabbitMQ and shown as conflicts.
				</p>
				<div class="row"><button class="btn primary" onclick={compare} disabled={!text.trim() || busy}><Icon name="swap" />Compare with cluster</button></div>
			</div>
		</div>
	</div>

	{#if diff}
		<div class="card" style="margin-top:14px">
			<div class="card-head">
				<h2>Changes</h2>
				<div class="row" style="gap:6px">
					{#each order as k}{#if diff.counts[k]}<span class="badge {badge[k]}">{diff.counts[k]} {k}</span>{/if}{/each}
				</div>
				<span class="spacer"></span>
				<label class="check"><input type="checkbox" bind:checked={showUnchanged} />Show unchanged</label>
				{#if allImportable.length}
					<button class="btn sm" onclick={() => toggleAll(chosen.length !== allImportable.length)}>
						{chosen.length === allImportable.length ? 'Select none' : 'Select all'}
					</button>
					<button class="btn sm primary" onclick={runImport} disabled={!chosen.length || busy}><Icon name="upload" size={13} />Import {chosen.length}</button>
				{/if}
			</div>
			{#if rows.length === 0}
				<Empty icon="check" title="Nothing to import">The cluster already has everything in this file.</Empty>
			{:else}
				<table class="t">
					<thead><tr><th style="width:28px"></th><th>Change</th><th>Kind</th><th>Vhost</th><th>Name</th><th>Fields</th></tr></thead>
					<tbody>
						{#each rows as c (c.id)}
							<tr class="click" onclick={() => (expanded = expanded === c.id ? null : c.id)}>
								<td onclick={(e) => e.stopPropagation()}>
									{#if importable(c)}<input type="checkbox" bind:checked={selected[c.id]} aria-label="Import {c.name}" />{/if}
								</td>
								<td><span class="badge {badge[c.change]}">{c.change}</span></td>
								<td>{c.kind.replace('_', ' ')}</td>
								<td class="mono">{c.vhost ?? ''}</td>
								<td class="mono">{c.name}</td>
								<td class="faint">{c.fields.join(', ')}</td>
							</tr>
							{#if expanded === c.id}
								<tr class="detail">
									<td></td>
									<td colspan="5">
										<div class="grid-2">
											<div><div class="label">Cluster</div>{#if c.before !== undefined && c.before !== null}<JsonView value={c.before} maxHeight={260} />{:else}<span class="faint">—</span>{/if}</div>
											<div><div class="label">File</div>{#if c.after !== undefined && c.after !== null}<JsonView value={c.after} maxHeight={260} />{:else}<span class="faint">—</span>{/if}</div>
										</div>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/if}
</div>

<style>
	.small {
		font-size: 12px;
	}
	.label {
		font-size: 12px;
		color: var(--text-2);
		margin-bottom: 4px;
	}
	tr.click {
		cursor: pointer;
	}
	tr.detail td {
		background: var(--bg-2, transparent);
	}
	textarea {
		width: 100%;
		font-size: 12.5px;
	}
</style>
