<script lang="ts">
	import { onMount } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { session } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { dateTime } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Loading from '$lib/components/Loading.svelte';

	type User = Schemas['User'];
	type Role = 'viewer' | 'operator' | 'admin';
	let users = $state<User[] | null>(null);
	let open = $state(false);
	let username = $state('');
	let password = $state('');
	let role = $state<Role>('viewer');
	let resetFor = $state<User | null>(null);
	let resetPw = $state('');

	async function load() {
		try {
			users = await unwrap(api.GET('/v1/users'));
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	onMount(load);

	async function create(e: SubmitEvent) {
		e.preventDefault();
		try {
			await unwrap(api.POST('/v1/users', { body: { username, password, role } }));
			toast(`Created ${username}`);
			open = false;
			username = password = '';
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	async function update(u: User, body: { role?: Role; disabled?: boolean; password?: string }) {
		try {
			await unwrap(api.PATCH('/v1/users/{id}', { params: { path: { id: u.id } }, body }));
			toast(`Updated ${u.username}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	async function remove(u: User) {
		if (!(await confirm({ title: 'Delete user', message: `Delete ${u.username} and their API tokens?`, confirmText: 'Delete', danger: true }))) return;
		try {
			await unwrap(api.DELETE('/v1/users/{id}', { params: { path: { id: u.id } } }));
			toast(`Deleted ${u.username}`);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Users · rabmin</title></svelte:head>

<div class="page" style="max-width:1000px">
	<div class="page-head">
		<div>
			<h1>Users</h1>
			<div class="sub"><b>Viewers</b> can look. <b>Operators</b> can also change queues, exchanges, messages and connections. <b>Admins</b> manage clusters, RabbitMQ users and permissions, rabmin users and maintenance.</div>
		</div>
		<span class="spacer"></span>
		<button class="btn primary" onclick={() => (open = true)}><Icon name="plus" />Add user</button>
	</div>
	<div class="card table-wrap">
		{#if !users}<Loading />
		{:else}
			<table class="t">
				<thead><tr><th>User</th><th>Role</th><th>Status</th><th>Created</th><th></th></tr></thead>
				<tbody>
					{#each users as u (u.id)}
						<tr>
							<td><b>{u.username}</b>{#if u.id === session.me?.user.id}<span class="badge">you</span>{/if}</td>
							<td>
								<select value={u.role} onchange={(e) => update(u, { role: (e.currentTarget as HTMLSelectElement).value as Role })} style="width:auto" disabled={u.id === session.me?.user.id}>
									<option value="viewer">viewer</option><option value="operator">operator</option><option value="admin">admin</option>
								</select>
							</td>
							<td>{#if u.disabled}<span class="badge bad">disabled</span>{:else}<span class="badge ok">active</span>{/if}</td>
							<td>{dateTime(u.createdAt)}</td>
							<td class="r nowrap">
								<button class="btn sm" onclick={() => ((resetFor = u), (resetPw = ''))}>Reset password</button>
								{#if u.id !== session.me?.user.id}
									<button class="btn sm" onclick={() => update(u, { disabled: !u.disabled })}>{u.disabled ? 'Enable' : 'Disable'}</button>
									<button class="btn sm danger" onclick={() => remove(u)}><Icon name="trash" size={13} /></button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

<Modal bind:open title="Add user" width={460}>
	<form id="uform" class="stack" onsubmit={create}>
		<label class="field"><span>Username</span><input bind:value={username} required autocomplete="off" /></label>
		<label class="field"><span>Password</span><input type="password" bind:value={password} minlength="8" required autocomplete="new-password" /></label>
		<label class="field"
			><span>Role</span><select bind:value={role}><option value="viewer">Viewer</option><option value="operator">Operator</option><option value="admin">Admin</option></select></label
		>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="uform">Create</button>
	{/snippet}
</Modal>

{#if resetFor}
	<Modal open={true} title="Reset password for {resetFor.username}" width={420}>
		<label class="field"><span>New password</span><input type="password" bind:value={resetPw} minlength="8" autocomplete="new-password" /></label>
		{#snippet footer()}
			<button class="btn" onclick={() => (resetFor = null)}>Cancel</button>
			<button class="btn primary" disabled={resetPw.length < 8} onclick={async () => { await update(resetFor!, { password: resetPw }); resetFor = null; }}>Set password</button>
		{/snippet}
	</Modal>
{/if}
