<script lang="ts">
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { userTags } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import VHostSelect from '$lib/components/VHostSelect.svelte';

	type RUser = Schemas['RabbitUser'];
	type Perm = Schemas['Permission'];
	type TPerm = Schemas['TopicPermission'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let users = $state<RUser[] | null>(null);
	let error = $state<string | null>(null);
	let filter = $state('');
	let selected = $state<string[]>([]);
	let current = $state<string | null>(null);
	let perms = $state<Perm[]>([]);
	let tperms = $state<TPerm[]>([]);

	async function load() {
		try {
			users = await unwrap(api.GET('/v1/clusters/{cluster}/users', { params: { path: { cluster: live.id } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	async function loadPerms(u: string) {
		perms = await unwrap(api.GET('/v1/clusters/{cluster}/permissions', { params: { path: { cluster: live.id }, query: { user: u } } }));
		tperms = await unwrap(api.GET('/v1/clusters/{cluster}/topic-permissions', { params: { path: { cluster: live.id }, query: { user: u } } }));
	}
	onMount(load);
	const rows = $derived((users ?? []).filter((u) => !filter || `${u.name} ${u.tags.join(' ')} ${u.vhosts.join(' ')}`.toLowerCase().includes(filter.toLowerCase())));

	async function select(u: string) {
		current = current === u ? null : u;
		if (current) {
			try {
				await loadPerms(current);
			} catch (e) {
				toast(errMsg(e), 'bad');
			}
		}
	}

	// Create / edit.
	let open = $state(false);
	let editing = $state<RUser | null>(null);
	let f = $state({ name: '', tags: [] as string[], custom: '', password: '', noPassword: false, maxConnections: '', maxChannels: '' });
	function edit(u: RUser | null) {
		editing = u;
		f = {
			name: u?.name ?? '',
			tags: u?.tags.filter((t) => userTags.includes(t)) ?? [],
			custom: u?.tags.filter((t) => !userTags.includes(t)).join(', ') ?? '',
			password: '',
			noPassword: false,
			maxConnections: u?.maxConnections != null ? String(u.maxConnections) : '',
			maxChannels: u?.maxChannels != null ? String(u.maxChannels) : ''
		};
		open = true;
	}
	async function save(e: SubmitEvent) {
		e.preventDefault();
		const tags = [...f.tags, ...f.custom.split(',').map((t) => t.trim()).filter(Boolean)];
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/users/{user}', {
					params: { path: { cluster: live.id, user: f.name }, query: { create: !editing } },
					body: {
						tags,
						password: f.password || undefined,
						noPassword: !editing && f.noPassword ? true : undefined,
						maxConnections: f.maxConnections === '' ? -1 : Number(f.maxConnections),
						maxChannels: f.maxChannels === '' ? -1 : Number(f.maxChannels)
					}
				})
			);
			toast(editing ? `Saved ${f.name}` : `Created ${f.name}`);
			open = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
	async function removeUsers(names: string[]) {
		if (
			!(await confirm({
				title: names.length === 1 ? 'Delete user' : `Delete ${names.length} users`,
				message: `Delete ${names.join(', ')}? Their permissions go with them and RabbitMQ closes their connections.`,
				confirmText: 'Delete',
				danger: true,
				typeToConfirm: names.length === 1 ? names[0] : String(names.length)
			}))
		)
			return;
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/users/delete', { params: { path: { cluster: live.id } }, body: { users: names } }));
			toast(`Deleted ${names.length} user${names.length === 1 ? '' : 's'}`);
			selected = [];
			if (current && names.includes(current)) current = null;
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	// Permissions.
	let p = $state({ vhost: '/', configure: '.*', write: '.*', read: '.*' });
	async function setPerm(e: SubmitEvent) {
		e.preventDefault();
		if (!current) return;
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/permissions/{vhost}/{user}', {
					params: { path: { cluster: live.id, vhost: p.vhost, user: current } },
					body: { configure: p.configure, write: p.write, read: p.read }
				})
			);
			toast('Permissions set');
			await loadPerms(current);
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
	async function delPerm(x: Perm) {
		if (!(await confirm({ title: 'Revoke permissions', message: `Revoke ${x.user}'s permissions in ${x.vhost}?`, confirmText: 'Revoke', danger: true }))) return;
		try {
			await unwrap(api.DELETE('/v1/clusters/{cluster}/permissions/{vhost}/{user}', { params: { path: { cluster: live.id, vhost: x.vhost, user: x.user } } }));
			await loadPerms(x.user);
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	let tp = $state({ vhost: '/', exchange: 'amq.topic', write: '.*', read: '.*' });
	async function setTPerm(e: SubmitEvent) {
		e.preventDefault();
		if (!current) return;
		try {
			await unwrap(
				api.PUT('/v1/clusters/{cluster}/topic-permissions/{vhost}/{user}', {
					params: { path: { cluster: live.id, vhost: tp.vhost, user: current } },
					body: { exchange: tp.exchange, write: tp.write, read: tp.read }
				})
			);
			toast('Topic permission set');
			await loadPerms(current);
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
	async function delTPerm(x: TPerm) {
		if (!(await confirm({ title: 'Remove topic permission', message: `Remove ${x.user}'s topic permission on ${x.exchange} in ${x.vhost}?`, confirmText: 'Remove', danger: true })))
			return;
		try {
			await unwrap(
				api.DELETE('/v1/clusters/{cluster}/topic-permissions/{vhost}/{user}', {
					params: { path: { cluster: live.id, vhost: x.vhost, user: x.user }, query: { exchange: x.exchange } }
				})
			);
			await loadPerms(x.user);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>Users · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Users &amp; permissions</h1>
			<div class="sub">RabbitMQ users of this cluster, not rabmin's own users.</div>
		</div>
		<span class="spacer"></span>
		<input type="search" placeholder="Filter by name, tag, vhost…" bind:value={filter} style="width:240px" />
		{#if can('admin') && selected.length}<button class="btn danger" onclick={() => removeUsers(selected)}><Icon name="trash" />Delete {selected.length}</button>{/if}
		{#if can('admin')}<button class="btn primary" onclick={() => edit(null)}><Icon name="plus" />Add user</button>{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !users}
		<Loading />
	{:else}
		<div class="card table-wrap">
			<table class="t">
				<thead>
					<tr>{#if can('admin')}<th style="width:28px"></th>{/if}<th>User</th><th>Tags</th><th>Virtual hosts</th><th>Limits</th><th>Password</th>{#if can('admin')}<th></th>{/if}</tr>
				</thead>
				<tbody>
					{#each rows as u (u.name)}
						<tr class="clickable" class:sel={current === u.name} onclick={() => select(u.name)}>
							{#if can('admin')}
								<td onclick={(e) => e.stopPropagation()}><input type="checkbox" bind:group={selected} value={u.name} aria-label="Select {u.name}" /></td>
							{/if}
							<td><b>{u.name}</b></td>
							<td>{#each u.tags as t}<span class="badge {t === 'administrator' ? 'accent' : ''}">{t}</span> {:else}<span class="faint">none</span>{/each}</td>
							<td class="small">{u.vhosts.join(', ') || '—'}</td>
							<td class="small nowrap">
								{#if u.maxConnections != null}<div>{u.maxConnections} connections</div>{/if}
								{#if u.maxChannels != null}<div>{u.maxChannels} channels</div>{/if}
							</td>
							<td>{#if u.hasPassword}<span class="badge ok">set</span>{:else}<span class="badge" title="Certificate or OAuth2 login only">none</span>{/if}</td>
							{#if can('admin')}
								<td class="r nowrap" onclick={(e) => e.stopPropagation()}>
									<button class="btn sm" aria-label="Edit" onclick={() => edit(u)}><Icon name="edit" size={13} /></button>
									<button class="btn sm danger" aria-label="Delete" onclick={() => removeUsers([u.name])}><Icon name="trash" size={13} /></button>
								</td>
							{/if}
						</tr>
						{#if current === u.name}
							<tr class="detail">
								<td colspan="7">
									<div class="grid-2">
										<div class="stack">
											<div class="label">Permissions</div>
											<table class="t inner">
												<thead><tr><th>Vhost</th><th>Configure</th><th>Write</th><th>Read</th>{#if can('admin')}<th></th>{/if}</tr></thead>
												<tbody>
													{#each perms as x}
														<tr>
															<td class="mono">{x.vhost}</td><td class="mono">{x.configure || '∅'}</td><td class="mono">{x.write || '∅'}</td><td class="mono">{x.read || '∅'}</td>
															{#if can('admin')}<td class="r"><button class="btn sm danger" aria-label="Revoke" onclick={() => delPerm(x)}><Icon name="x" size={13} /></button></td>{/if}
														</tr>
													{:else}
														<tr><td colspan="5" class="faint">No permissions: this user cannot use any vhost.</td></tr>
													{/each}
												</tbody>
											</table>
											{#if can('admin')}
												<form class="pform" onsubmit={setPerm}>
													<VHostSelect cluster={live.id} bind:value={p.vhost} />
													<input bind:value={p.configure} class="mono" aria-label="configure" placeholder="configure" />
													<input bind:value={p.write} class="mono" aria-label="write" placeholder="write" />
													<input bind:value={p.read} class="mono" aria-label="read" placeholder="read" />
													<button class="btn sm primary">Set</button>
												</form>
												<div class="hint">Regular expressions over resource names; empty grants nothing. Setting replaces the user's permissions in that vhost.</div>
											{/if}
										</div>
										<div class="stack">
											<div class="label">Topic permissions</div>
											<table class="t inner">
												<thead><tr><th>Vhost</th><th>Exchange</th><th>Write</th><th>Read</th>{#if can('admin')}<th></th>{/if}</tr></thead>
												<tbody>
													{#each tperms as x}
														<tr>
															<td class="mono">{x.vhost}</td><td>{x.exchange}</td><td class="mono">{x.write || '∅'}</td><td class="mono">{x.read || '∅'}</td>
															{#if can('admin')}<td class="r"><button class="btn sm danger" aria-label="Remove" onclick={() => delTPerm(x)}><Icon name="x" size={13} /></button></td>{/if}
														</tr>
													{:else}
														<tr><td colspan="5" class="faint">None: routing keys are not restricted.</td></tr>
													{/each}
												</tbody>
											</table>
											{#if can('admin')}
												<form class="pform" onsubmit={setTPerm}>
													<VHostSelect cluster={live.id} bind:value={tp.vhost} />
													<input bind:value={tp.exchange} aria-label="exchange" placeholder="exchange" required />
													<input bind:value={tp.write} class="mono" aria-label="write" placeholder="write" />
													<input bind:value={tp.read} class="mono" aria-label="read" placeholder="read" />
													<button class="btn sm primary">Set</button>
												</form>
											{/if}
										</div>
									</div>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open title={editing ? `Edit ${editing.name}` : 'Add RabbitMQ user'}>
	<form id="uform" class="stack" onsubmit={save}>
		<label class="field"><span>Name</span><input bind:value={f.name} required disabled={!!editing} autocomplete="off" /></label>
		<div class="field">
			<span>Tags</span>
			<div class="row">{#each userTags as t}<label class="check"><input type="checkbox" bind:group={f.tags} value={t} />{t}</label>{/each}</div>
			<input bind:value={f.custom} placeholder="custom tags, comma separated" />
		</div>
		<label class="field">
			<span>Password</span>
			<input type="password" bind:value={f.password} autocomplete="new-password" placeholder={editing ? (editing.hasPassword ? 'leave blank to keep the current password' : 'no password set') : ''} />
		</label>
		{#if !editing}<label class="check"><input type="checkbox" bind:checked={f.noPassword} />No password (certificate or OAuth2 login only)</label>{/if}
		<div class="grid-2">
			<label class="field"><span>Max connections</span><input type="number" min="0" bind:value={f.maxConnections} placeholder="unlimited" /></label>
			<label class="field"><span>Max channels</span><input type="number" min="0" bind:value={f.maxChannels} placeholder="unlimited" /></label>
		</div>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (open = false)}>Cancel</button>
		<button class="btn primary" form="uform">{editing ? 'Save' : 'Create'}</button>
	{/snippet}
</Modal>

<style>
	.small {
		font-size: 12px;
	}
	.detail td {
		background: var(--bg-sunken) !important;
	}
	tr.sel td {
		background: var(--accent-soft);
	}
	table.inner {
		background: var(--bg-elev);
		border: 1px solid var(--border);
	}
	.pform {
		display: grid;
		grid-template-columns: 1.2fr 1fr 1fr 1fr auto;
		gap: 6px;
	}
	.label {
		font-size: 12px;
		font-weight: 600;
		color: var(--text-2);
	}
</style>
