<script lang="ts">
	import { onMount } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can, session } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { ago, dateTime } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';

	type Token = Schemas['APIToken'];
	type Role = 'viewer' | 'operator' | 'admin';
	let tokens = $state<Token[] | null>(null);
	let all = $state(false);
	let open = $state(false);
	let name = $state('');
	let role = $state<Role>((session.me?.role as Role) ?? 'viewer');
	let days = $state(90);
	let created = $state<string | null>(null);

	async function load() {
		try {
			tokens = await unwrap(api.GET('/v1/tokens', { params: { query: { all } } }));
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	onMount(load);

	async function create(e: SubmitEvent) {
		e.preventDefault();
		try {
			const t = await unwrap(api.POST('/v1/tokens', { body: { name, role, expiresInDays: Number(days) } }));
			created = t.token;
			name = '';
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	async function revoke(t: Token) {
		if (!(await confirm({ title: 'Revoke token', message: `Revoke "${t.name}" (${t.prefix}…)? Anything using it stops working.`, confirmText: 'Revoke', danger: true }))) return;
		try {
			await unwrap(api.DELETE('/v1/tokens/{id}', { params: { path: { id: t.id } } }));
			toast('Token revoked');
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	const example = $derived(`curl -H "Authorization: Bearer ${created ?? 'rm_…'}" ${location.origin}/api/v1/auth/me`);
</script>

<svelte:head><title>API tokens · rabmin</title></svelte:head>

<div class="page" style="max-width:1000px">
	<div class="page-head">
		<div>
			<h1>API tokens</h1>
			<div class="sub">For scripts, CI and other tools. Everything in the UI is available through the <a href="/api/docs" target="_blank" rel="noopener">API</a>.</div>
		</div>
		<span class="spacer"></span>
		{#if can('admin')}<label class="check"><input type="checkbox" bind:checked={all} onchange={load} />All users</label>{/if}
		<button class="btn primary" onclick={() => ((open = true), (created = null))}><Icon name="plus" />New token</button>
	</div>
	<div class="card table-wrap">
		{#if !tokens}<Loading />
		{:else if tokens.length === 0}<Empty icon="key" title="No tokens" />
		{:else}
			<table class="t">
				<thead><tr><th>Name</th><th>Token</th>{#if all}<th>User</th>{/if}<th>Role</th><th>Created</th><th>Expires</th><th>Last used</th><th></th></tr></thead>
				<tbody>
					{#each tokens as t (t.id)}
						<tr>
							<td><b>{t.name}</b></td>
							<td class="mono">{t.prefix}…</td>
							{#if all}<td>{t.username}</td>{/if}
							<td>{t.role}</td>
							<td>{dateTime(t.createdAt)}</td>
							<td>{t.expiresAt ? dateTime(t.expiresAt) : 'never'}</td>
							<td>{t.lastUsedAt ? ago(t.lastUsedAt) : 'never'}</td>
							<td class="r"><button class="btn sm danger" onclick={() => revoke(t)}>Revoke</button></td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<Modal bind:open title="New API token" width={560}>
	{#if created}
		<div class="stack">
			<div class="alert warn">Copy the token now. It will not be shown again.</div>
			<div class="row"><code class="tok">{created}</code><CopyButton text={created} label="Copy" /></div>
			<div class="label">Try it</div>
			<pre class="ex">{example}</pre>
		</div>
	{:else}
		<form id="tform" class="stack" onsubmit={create}>
			<label class="field"><span>Name</span><input bind:value={name} required placeholder="ci-deploy" /></label>
			<div class="grid-2">
				<label class="field">
					<span>Role</span>
					<select bind:value={role}>
						<option value="viewer">Viewer</option>
						{#if can('operator')}<option value="operator">Operator</option>{/if}
						{#if can('admin')}<option value="admin">Admin</option>{/if}
					</select>
				</label>
				<label class="field">
					<span>Expires</span>
					<select bind:value={days}><option value={7}>7 days</option><option value={30}>30 days</option><option value={90}>90 days</option><option value={365}>1 year</option><option value={0}>Never</option></select>
				</label>
			</div>
		</form>
	{/if}
	{#snippet footer()}
		{#if created}<button class="btn primary" onclick={() => (open = false)}>Done</button>
		{:else}<button class="btn" onclick={() => (open = false)}>Cancel</button><button class="btn primary" form="tform">Create token</button>{/if}
	{/snippet}
</Modal>

<style>
	.tok {
		flex: 1;
		padding: 8px 10px;
		background: var(--bg-sunken);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		word-break: break-all;
	}
	.ex {
		background: var(--bg-sunken);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		padding: 10px;
	}
</style>
