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

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	let flags = $state<Schemas['FeatureFlag'][] | null>(null);
	let deprecated = $state<Schemas['DeprecatedFeature'][] | null>(null);
	let error = $state<string | null>(null);
	let filter = $state('');
	let busy = $state(false);

	async function load() {
		try {
			const p = { params: { path: { cluster: live.id } } };
			[flags, deprecated] = await Promise.all([
				unwrap(api.GET('/v1/clusters/{cluster}/feature-flags', p)),
				unwrap(api.GET('/v1/clusters/{cluster}/deprecated-features', p))
			]);
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	onMount(load);

	const shown = $derived(
		(flags ?? []).filter((f) => !filter || f.name.includes(filter) || f.description.toLowerCase().includes(filter.toLowerCase()))
	);
	const pending = $derived((flags ?? []).filter((f) => f.state === 'disabled' && f.stability === 'stable'));
	const inUse = $derived((deprecated ?? []).filter((d) => d.used));

	async function enable(names: string[] | undefined, label: string) {
		const message = names
			? `Enable ${label}? Feature flags cannot be disabled again, and every node must support the flag.`
			: `Enable all ${pending.length} disabled stable feature flags? Feature flags cannot be disabled again.`;
		if (!(await confirm({ title: 'Enable feature flag', message, confirmText: 'Enable', danger: true }))) return;
		busy = true;
		try {
			const r = await unwrap(api.POST('/v1/clusters/{cluster}/feature-flags/enable', { params: { path: { cluster: live.id } }, body: { names } }));
			toast(r.enabled.length ? `Enabled ${r.enabled.join(', ')}` : 'Nothing to enable');
			load();
		} catch (e) {
			toast(errMsg(e), 'bad');
		} finally {
			busy = false;
		}
	}
	const stateBadge = (s: string) => (s === 'enabled' ? 'ok' : s === 'disabled' ? 'warn' : 'info');
</script>

<svelte:head><title>Feature flags · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Feature flags</h1>
			<div class="sub">Flags gate features that need every node to agree. Enable stable flags after upgrading all nodes; they cannot be turned off.</div>
		</div>
		<span class="spacer"></span>
		<button class="btn" onclick={load}><Icon name="refresh" />Refresh</button>
		{#if can('admin') && pending.length}
			<button class="btn primary" onclick={() => enable(undefined, '')} disabled={busy}><Icon name="zap" />Enable all stable ({pending.length})</button>
		{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !flags || !deprecated}
		{#if !error}<Loading />{/if}
	{:else}
		{#if pending.length}
			<div class="alert warn">{pending.length} stable feature flag{pending.length === 1 ? ' is' : 's are'} disabled. The next RabbitMQ upgrade may require {pending.length === 1 ? 'it' : 'them'}.</div>
		{/if}
		{#if inUse.length}
			<div class="alert warn">
				Deprecated features in use: {inUse.map((d) => d.name).join(', ')}. They stop working when a later release removes them.
			</div>
		{/if}
		<div class="card">
			<div class="card-head">
				<h2>Feature flags ({flags.length})</h2>
				<span class="spacer"></span>
				<input placeholder="Filter" bind:value={filter} style="width:200px" />
			</div>
			<table class="t">
				<thead><tr><th>Name</th><th>State</th><th>Stability</th><th>Description</th><th>Provided by</th>{#if can('admin')}<th></th>{/if}</tr></thead>
				<tbody>
					{#each shown as f (f.name)}
						<tr>
							<td class="mono nowrap">{#if f.docUrl}<a href={f.docUrl} target="_blank" rel="noreferrer">{f.name}</a>{:else}{f.name}{/if}</td>
							<td><span class="badge {stateBadge(f.state)}">{f.state.replace('_', ' ')}</span></td>
							<td><span class="badge {f.stability === 'experimental' ? 'bad' : f.stability === 'required' ? 'accent' : ''}">{f.stability}</span></td>
							<td>{f.description}</td>
							<td class="faint">{f.providedBy ?? ''}</td>
							{#if can('admin')}
								<td class="r">
									{#if f.state === 'disabled'}<button class="btn sm" onclick={() => enable([f.name], f.name)} disabled={busy}>Enable</button>{/if}
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
		<div class="card" style="margin-top:14px">
			<div class="card-head"><h2>Deprecated features ({deprecated.length})</h2></div>
			<table class="t">
				<thead><tr><th>Name</th><th>In use</th><th>Phase</th><th>State</th><th>Description</th></tr></thead>
				<tbody>
					{#each deprecated as d (d.name)}
						<tr>
							<td class="mono nowrap">{#if d.docUrl}<a href={d.docUrl} target="_blank" rel="noreferrer">{d.name}</a>{:else}{d.name}{/if}</td>
							<td>{#if d.used}<span class="badge bad">in use</span>{:else}<span class="faint">no</span>{/if}</td>
							<td>{d.phase.replaceAll('_', ' ')}</td>
							<td><span class="badge {d.state === 'denied' ? 'warn' : ''}">{d.state}</span></td>
							<td>{d.description}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
