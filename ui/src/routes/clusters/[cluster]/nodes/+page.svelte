<script lang="ts">
	import { goto } from '$app/navigation';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { bytes, compact, num, duration } from '$lib/format';
	import { Sorter } from '$lib/sort.svelte';
	import SortTh from '$lib/components/SortTh.svelte';
	import Meter from '$lib/components/Meter.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import type { Schemas } from '$lib/api/client';

	type Node = Schemas['NodeInfo'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const snap = $derived(live.snapshot);
	const sorter = new Sorter<Node>('name');
	const rows = $derived(sorter.sort(snap?.nodes ?? []));
	const failing = (n: string) => (snap?.health ?? []).filter((h) => h.node === n && h.status !== 'ok');
</script>

<svelte:head><title>Nodes · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Nodes</h1>
			<div class="sub">{snap?.totals.nodesRunning ?? 0} of {snap?.totals.nodes ?? 0} running</div>
		</div>
	</div>
	{#if !snap}
		<Loading />
	{:else}
		{#if snap.error}<div class="alert bad" style="margin-bottom:14px">{snap.error}</div>{/if}
		<div class="card table-wrap">
			<table class="t">
				<thead>
					<tr>
						<SortTh {sorter} key="name">Node</SortTh>
						<SortTh {sorter} key="rabbitmqVersion">Version</SortTh>
						<th>State</th>
						<SortTh {sorter} key="memUsed" right desc>Memory</SortTh>
						<SortTh {sorter} key="diskFree" right>Disk free</SortTh>
						<SortTh {sorter} key="fdUsed" right desc>File descriptors</SortTh>
						<SortTh {sorter} key="procUsed" right desc>Erlang processes</SortTh>
						<SortTh {sorter} key="runQueue" right desc>Run queue</SortTh>
						<SortTh {sorter} key="uptimeMs" right>Uptime</SortTh>
						<th>Endpoint</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as n (n.name)}
						{@const bad = failing(n.name)}
						<tr class="clickable" onclick={() => goto(`/clusters/${live.id}/nodes/${encodeURIComponent(n.name)}`)}>
							<td>
								<div class="row" style="gap:6px;flex-wrap:nowrap">
									<span class="dot {!n.running ? 'bad' : n.healthy ? 'ok' : 'warn'}"></span><b>{n.name}</b>
								</div>
								<div class="faint" style="font-size:11.5px">{n.type} · {n.processors} cores</div>
							</td>
							<td class="mono">{n.rabbitmqVersion || '—'}<div class="faint" style="font-size:11.5px">Erlang {n.erlangVersion || '—'}</div></td>
							<td>
								{#if !n.running}<span class="badge bad">down</span>
								{:else}
									{#if n.memAlarm}<span class="badge bad">memory alarm</span>{/if}
									{#if n.diskAlarm}<span class="badge bad">disk alarm</span>{/if}
									{#if n.partitions.length}<span class="badge bad">partitioned</span>{/if}
									{#if n.beingDrained}<span class="badge warn">maintenance</span>{/if}
									{#if bad.length}<span class="badge warn" title={bad.map((b) => `${b.check}: ${b.reason ?? ''}`).join('\n')}>{bad.length} check{bad.length === 1 ? '' : 's'} failing</span>{/if}
									{#if n.healthy}<span class="badge ok">healthy</span>{/if}
								{/if}
							</td>
							{#if n.running}
								<td class="r" style="min-width:140px"><div class="faint small">{bytes(n.memUsed)} / {bytes(n.memLimit)}</div><Meter used={n.memUsed} total={n.memLimit} /></td>
								<td class="r num nowrap">{bytes(n.diskFree)}<div class="faint small">limit {bytes(n.diskFreeLimit)}</div></td>
								<td class="r" style="min-width:120px"><div class="faint small">{num(n.fdUsed)} / {num(n.fdTotal)}</div><Meter used={n.fdUsed} total={n.fdTotal} /></td>
								<td class="r" style="min-width:120px"><div class="faint small">{num(n.procUsed)} / {compact(n.procTotal)}</div><Meter used={n.procUsed} total={n.procTotal} /></td>
								<td class="r num">{n.runQueue}</td>
								<td class="r nowrap">{duration(n.uptimeMs)}</td>
							{:else}
								<td colspan="6"></td>
							{/if}
							<td class="mono small">{n.endpoint ?? ''}{#if !n.endpoint}<span class="faint" title="No configured management URL reaches this node">none</span>{/if}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.small {
		font-size: 11.5px;
	}
</style>
