<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { loadVHosts } from '$lib/vhosts.svelte';
	import { compact, rate } from '$lib/format';
	import { seg } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type VHost = Schemas['VHost'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let list = $state<VHost[] | null>(null);
	let error = $state<string | null>(null);

	async function load() {
		try {
			list = await loadVHosts(live.id, true);
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

	let open = $state(false);
	let editing = $state<VHost | null>(null);
	let f = $state({ name: '', description: '', tags: '', defaultQueueType: 'classic', protected: false, maxConnections: '', maxQueues: '' });
	let busy = $state(false);

	function edit(v: VHost | null) {
		editing = v;
		f = {
			name: v?.name ?? '',
			description: v?.description ?? '',
			tags: v?.tags.join(', ') ?? '',
			defaultQueueType: v?.defaultQueueType || 'classic',
			protected: v?.protected ?? false,
			maxConnections: v?.maxConnections != null ? String(v.maxConnections) : '',
			maxQueues: v?.maxQueues != null ? String(v.maxQueues) : ''
		};
		open = true;
	}

	const limit = (s: string) => (s.trim() === '' ? -1 : Number(s));

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/vhosts/{vhost}', {
					params: { path: { cluster: live.id, vhost: f.name } },
					body: {
						description: f.description,
						tags: f.tags.split(',').map((t) => t.trim()).filter(Boolean),
						defaultQueueType: f.defaultQueueType as 'classic',
						protected: f.protected,
						maxConnections: limit(f.maxConnections),
						maxQueues: limit(f.maxQueues)
					}
				})
			);
			toast(editing ? `Saved ${f.name}` : `Created ${f.name}`);
			open = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			busy = false;
		}
	}

	async function remove(v: VHost) {
		if (v.protected) {
			toast(`${v.name} is protected from deletion; turn protection off first`, 'bad');
			return;
		}
		if (
			!(await confirm({
				title: 'Delete virtual host',
				message: `Delete ${v.name} with every queue, exchange, binding, policy and permission in it? ${compact(v.messages)} messages will be lost. This cannot be undone.`,
				confirmText: 'Delete virtual host',
				danger: true,
				typeToConfirm: v.name
			}))
		)
			return;
		try {
			await unwrap(api.DELETE('/v1/clusters/{cluster}/vhosts/{vhost}', { params: { path: { cluster: live.id, vhost: v.name } } }));
			toast(`Deleted ${v.name}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function start(v: VHost, node: string) {
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/vhosts/{vhost}/start', { params: { path: { cluster: live.id, vhost: v.name } }, body: { node } }));
			toast(`Started ${v.name} on ${node}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Virtual hosts · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Virtual hosts</h1>
			<div class="sub">{list?.length ?? 0} virtual hosts</div>
		</div>
		<span class="spacer"></span>
		<button class="btn" onclick={load}><Icon name="refresh" />Refresh</button>
		{#if can('admin')}<button class="btn primary" onclick={() => edit(null)}><Icon name="plus" />Add virtual host</button>{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !list}
		<Loading />
	{:else}
		<div class="card table-wrap">
			<table class="t">
				<thead>
					<tr>
						<th>Name</th><th>Default queue type</th><th class="r">Ready</th><th class="r">Unacked</th><th class="r">Publish</th><th class="r">Deliver</th><th>Limits</th
						><th>Nodes</th>{#if can('admin')}<th></th>{/if}
					</tr>
				</thead>
				<tbody>
					{#each list as v (v.name)}
						<tr>
							<td>
								<b class="mono">{v.name}</b>
								{#if v.protected}<span class="badge info" title="Deletion protection is on">protected</span>{/if}
								{#each v.tags as t}<span class="badge">{t}</span>{/each}
								{#if v.description}<div class="faint small">{v.description}</div>{/if}
							</td>
							<td>{v.defaultQueueType || 'classic'}</td>
							<td class="r num"><a href="/clusters/{live.id}/queues?vhost={seg(v.name)}">{compact(v.ready)}</a></td>
							<td class="r num">{compact(v.unacked)}</td>
							<td class="r num">{rate(v.publishRate)}</td>
							<td class="r num">{rate(v.deliverRate)}</td>
							<td class="small nowrap">
								{#if v.maxConnections != null}<div>{v.maxConnections} connections</div>{/if}
								{#if v.maxQueues != null}<div>{v.maxQueues} queues</div>{/if}
								{#if v.maxConnections == null && v.maxQueues == null}<span class="faint">none</span>{/if}
							</td>
							<td>
								{#each v.nodeStates as n}
									{#if n.state === 'running'}<span class="dot ok" title="{n.node}: running"></span>
									{:else}<button class="badge bad as-btn" title="Start on {n.node}" disabled={!can('admin')} onclick={() => start(v, n.node)}
											>{n.node.replace(/^rabbit@/, '')}: {n.state}</button
										>{/if}
								{/each}
							</td>
							{#if can('admin')}
								<td class="r nowrap">
									<button class="btn sm" aria-label="Edit" onclick={() => edit(v)}><Icon name="edit" size={13} /></button>
									<button class="btn sm danger" aria-label="Delete" onclick={() => remove(v)}><Icon name="trash" size={13} /></button>
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open title={editing ? `Edit ${editing.name}` : 'Add virtual host'}>
	<form id="vhform" class="stack" onsubmit={save}>
		<div class="grid-2">
			<label class="field"><span>Name</span><input bind:value={f.name} required disabled={!!editing} /></label>
			<label class="field"
				><span>Default queue type</span><select bind:value={f.defaultQueueType}>
					<option value="classic">classic</option><option value="quorum">quorum</option><option value="stream">stream</option>
				</select><span class="hint">For declarations that do not name a type.</span></label
			>
		</div>
		<label class="field"><span>Description</span><input bind:value={f.description} /></label>
		<label class="field"><span>Tags</span><input bind:value={f.tags} placeholder="production, eu" /></label>
		<div class="grid-2">
			<label class="field"><span>Max connections</span><input type="number" min="0" bind:value={f.maxConnections} placeholder="unlimited" /></label>
			<label class="field"><span>Max queues</span><input type="number" min="0" bind:value={f.maxQueues} placeholder="unlimited" /></label>
		</div>
		<label class="check"><input type="checkbox" bind:checked={f.protected} />Protect from deletion</label>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="vhform" disabled={busy}>{editing ? 'Save' : 'Create'}</button>
	{/snippet}
</Modal>

<style>
	.small {
		font-size: 12px;
	}
	.as-btn {
		border: none;
		font: inherit;
		cursor: pointer;
	}
	.dot {
		margin-right: 3px;
	}
</style>
