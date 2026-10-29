<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { findCluster, refreshClusters } from '$lib/clusters.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { ago } from '$lib/format';
	import ClusterForm from '$lib/components/ClusterForm.svelte';
	import CredentialFields from '$lib/components/CredentialFields.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type Profile = Schemas['CredentialProfileOut'];
	const id = $derived(page.params.cluster!);
	const cluster = $derived(findCluster(id));

	let open = $state(false);
	let editing = $state<Profile | null>(null);
	let pName = $state('');
	let pId = $state('');
	let pAuth = $state<Schemas['CredentialIn']>({ type: 'basic' });
	let busy = $state(false);
	let test = $state<Schemas['TestResult'] | null>(null);

	function openProfile(p: Profile | null) {
		editing = p;
		pName = p?.name ?? '';
		pId = p?.id ?? '';
		pAuth = { type: (p?.auth.type === 'bearer' ? 'bearer' : 'basic') as Schemas['CredentialIn']['type'], username: p?.auth.username };
		test = null;
		open = true;
	}

	async function saveProfile(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			if (editing) {
				await unwrap(api.PUT('/v1/clusters/{cluster}/credentials/{credential}', { params: { path: { cluster: id, credential: editing.id } }, body: { name: pName, auth: pAuth } }));
			} else {
				await unwrap(api.POST('/v1/clusters/{cluster}/credentials', { params: { path: { cluster: id } }, body: { id: pId || undefined, name: pName, auth: pAuth } }));
			}
			await refreshClusters();
			toast('Credential profile saved');
			open = false;
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			busy = false;
		}
	}

	async function testProfile() {
		if (!cluster) return;
		try {
			test = await unwrap(
				api.POST('/v1/clusters/test', { body: { clusterId: id, credentialId: editing?.id, managementUrls: cluster.managementUrls.slice(0, 1), auth: pAuth } })
			);
		} catch (e) {
			test = { ok: false, error: errMsg(e), nodes: 0, tags: [], isAdministrator: false, canMonitor: false, plugins: [], statsDisabled: false, endpoints: [], prometheus: [], warnings: [] };
		}
	}

	async function deleteProfile(p: Profile) {
		if (!(await confirm({ title: 'Remove credential profile', message: `Remove ${p.name}? Nothing changes in RabbitMQ; rabmin just forgets these credentials.`, confirmText: 'Remove', danger: true })))
			return;
		try {
			await unwrap(api.DELETE('/v1/clusters/{cluster}/credentials/{credential}', { params: { path: { cluster: id, credential: p.id } } }));
			await refreshClusters();
			toast(`Removed ${p.name}`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function resetStats() {
		if (!(await confirm({ title: 'Reset statistics', message: 'Clear the management statistics on every node? Messages, queues and connections are not affected.', confirmText: 'Reset all' })))
			return;
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/reset-stats', { params: { path: { cluster: id } } }));
			toast('Statistics reset on every node');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function deleteCluster() {
		if (!(await confirm({ title: 'Remove cluster', message: `Remove ${cluster?.name} from rabmin? The RabbitMQ nodes are not touched.`, confirmText: 'Remove cluster', danger: true, typeToConfirm: id })))
			return;
		try {
			await unwrap(api.DELETE('/v1/clusters/{cluster}', { params: { path: { cluster: id } } }));
			await refreshClusters();
			toast('Cluster removed');
			goto('/');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Settings · {cluster?.name ?? id} · rabmin</title></svelte:head>

<div class="page" style="max-width:980px">
	<div class="page-head">
		<div>
			<h1>{cluster?.name ?? id} settings</h1>
			<div class="sub">Endpoints, credentials and credential profiles.</div>
		</div>
	</div>

	{#if !can('admin')}
		<div class="alert info">Only admins can change cluster settings.</div>
	{:else if cluster}
		<div class="stack">
			{#key cluster.updatedAt}
				<ClusterForm
					existing={cluster}
					onsaved={async () => {
						await refreshClusters();
						toast('Settings saved');
					}}
				/>
			{/key}

			<div class="card">
				<div class="card-head">
					<div>
						<h2>Credential profiles</h2>
						<div class="faint" style="font-size:12.5px">
							Extra RabbitMQ users for this cluster, so browsing and publishing can run with less privilege than the management user.
						</div>
					</div>
					<span class="spacer"></span>
					<button class="btn" onclick={() => openProfile(null)}><Icon name="plus" />Add</button>
				</div>
				{#if cluster.credentials.length === 0}
					<Empty icon="users" title="No credential profiles" />
				{:else}
					<table class="t">
						<thead><tr><th>Name</th><th>ID</th><th>Authentication</th><th>Updated</th><th></th></tr></thead>
						<tbody>
							{#each cluster.credentials as p (p.id)}
								<tr>
									<td><b>{p.name}</b></td>
									<td class="mono">{p.id}</td>
									<td>{p.auth.type === 'bearer' ? 'bearer token' : `user ${p.auth.username}`}</td>
									<td>{ago(p.updatedAt)}</td>
									<td class="r nowrap">
										<button class="btn sm" aria-label="Edit" onclick={() => openProfile(p)}><Icon name="edit" size={13} /></button>
										<button class="btn sm danger" aria-label="Remove" onclick={() => deleteProfile(p)}><Icon name="trash" size={13} /></button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</div>

			<div class="card">
				<div class="card-head"><h2>Statistics</h2></div>
				<div class="card-body row">
					<span class="muted">Clear the management statistics database on every node, for example after a load test.</span>
					<span class="spacer"></span>
					<button class="btn" onclick={resetStats}><Icon name="refresh" />Reset statistics</button>
				</div>
			</div>

			<div class="card danger-zone">
				<div class="card-head"><h2>Remove cluster</h2></div>
				<div class="card-body row">
					<span class="muted">Stops monitoring and forgets the stored credentials. Your RabbitMQ nodes are not touched.</span>
					<span class="spacer"></span>
					<button class="btn danger" onclick={deleteCluster}><Icon name="trash" />Remove cluster</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<Modal bind:open title={editing ? `Edit ${editing.name}` : 'Add credential profile'}>
	<form id="profileform" class="stack" onsubmit={saveProfile}>
		<div class="grid-2">
			<label class="field"><span>Name</span><input bind:value={pName} required placeholder="Read-only browser" /></label>
			<label class="field"><span>ID</span><input bind:value={pId} disabled={!!editing} placeholder="derived from name" /></label>
		</div>
		<CredentialFields bind:value={pAuth} stored={editing?.auth} idPrefix="profile" />
		<div class="row"><button type="button" class="btn" onclick={testProfile}><Icon name="zap" />Test</button></div>
		{#if test}
			{#if test.ok}
				<div class="alert info">Logged in as <b>{test.user}</b>{test.tags.length ? ` (${test.tags.join(', ')})` : ' (no tags)'} on {test.clusterName}.</div>
			{:else}<div class="alert bad">{test.error}</div>{/if}
		{/if}
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="profileform" disabled={busy}>Save</button>
	{/snippet}
</Modal>

<style>
	.danger-zone {
		border-color: color-mix(in srgb, var(--bad) 35%, var(--border));
	}
</style>
