<script lang="ts">
	import { onMount } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { toast } from '$lib/toast.svelte';
	import { dateTime } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';

	type Entry = Schemas['AuditEntry'];
	let entries = $state<Entry[] | null>(null);
	let actor = $state('');
	let action = $state('');
	let target = $state('');
	let more = $state(false);

	async function load(append = false) {
		try {
			const before = append && entries?.length ? entries[entries.length - 1].id : undefined;
			const page = await unwrap(
				api.GET('/v1/audit', { params: { query: { actor: actor || undefined, action: action || undefined, target: target || undefined, before, limit: 100 } } })
			);
			entries = append ? [...(entries ?? []), ...page] : page;
			more = page.length === 100;
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	onMount(() => load());
</script>

<svelte:head><title>Audit log · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Audit log</h1>
			<div class="sub">Every change made through rabmin, by the UI or the API.</div>
		</div>
	</div>
	<form class="card row filters" onsubmit={(e) => (e.preventDefault(), load())}>
		<input bind:value={actor} placeholder="User" style="max-width:180px" />
		<input bind:value={action} placeholder="Action prefix, e.g. delete-" style="max-width:240px" />
		<input bind:value={target} placeholder="Target contains…" style="max-width:280px" />
		<button class="btn primary"><Icon name="filter" />Filter</button>
	</form>
	<div class="card table-wrap" style="margin-top:14px">
		{#if !entries}<Loading />
		{:else if entries.length === 0}<Empty icon="log" title="Nothing logged yet" />
		{:else}
			<table class="t">
				<thead><tr><th>Time</th><th>User</th><th>Action</th><th>Target</th><th>Result</th><th>From</th></tr></thead>
				<tbody>
					{#each entries as e (e.id)}
						<tr>
							<td class="nowrap">{dateTime(e.time)}</td>
							<td>{e.actor}</td>
							<td><code>{e.action}</code></td>
							<td class="mono" style="font-size:12px;word-break:break-all">{e.target}{#if e.detail}<div class="faint">{e.detail}</div>{/if}</td>
							<td>{#if e.ok}<span class="badge ok">ok</span>{:else}<span class="badge bad" title={e.error}>{e.error || 'failed'}</span>{/if}</td>
							<td class="faint mono" style="font-size:12px">{e.remote}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	{#if more}<div style="margin-top:10px"><button class="btn" onclick={() => load(true)}>Load older</button></div>{/if}
</div>

<style>
	.filters {
		padding: 10px 14px;
	}
</style>
