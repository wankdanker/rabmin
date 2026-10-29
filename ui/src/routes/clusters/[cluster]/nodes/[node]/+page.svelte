<script lang="ts">
	import { page } from '$app/state';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { bytes, num, compact, duration, pct } from '$lib/format';
	import Meter from '$lib/components/Meter.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import JsonView from '$lib/components/JsonView.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const name = $derived(page.params.node!);
	let detail = $state<Schemas['NodeDetail'] | null>(null);
	let error = $state<string | null>(null);
	let showRaw = $state(false);

	async function load() {
		try {
			detail = await unwrap(api.GET('/v1/clusters/{cluster}/nodes/{node}', { params: { path: { cluster: live.id, node: name } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	$effect(() => {
		name;
		load();
		const t = setInterval(() => !document.hidden && load(), 10000);
		return () => clearInterval(t);
	});

	const groups = $derived.by(() => {
		const m = new Map<string, { total: number; parts: Schemas['MemoryPart'][] }>();
		for (const p of detail?.memory ?? []) {
			const g = m.get(p.group) ?? { total: 0, parts: [] };
			g.total += p.bytes;
			g.parts.push(p);
			m.set(p.group, g);
		}
		return [...m.entries()].map(([group, v]) => ({ group, ...v, parts: v.parts.sort((a, b) => b.bytes - a.bytes) })).sort((a, b) => b.total - a.total);
	});
	const memSum = $derived(groups.reduce((a, g) => a + g.total, 0));
	const health = $derived(
		(live.snapshot?.health ?? [])
			.filter((h) => h.node === name)
			.sort((a, b) => (live.snapshot?.healthChecks ?? []).indexOf(a.check) - (live.snapshot?.healthChecks ?? []).indexOf(b.check))
	);
	const palette = ['var(--series-1)', 'var(--series-2)', 'var(--series-3)', 'var(--series-4)', 'var(--ok)', 'var(--text-3)', 'var(--bad)', 'var(--info)', 'var(--warn)'];

	async function resetStats() {
		if (!(await confirm({ title: 'Reset statistics', message: `Clear the management statistics collected on ${name}? Messages, queues and connections are not affected.`, confirmText: 'Reset' })))
			return;
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/nodes/{node}/reset-stats', { params: { path: { cluster: live.id, node: name } } }));
			toast('Statistics reset');
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
</script>

<svelte:head><title>{name} · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<div class="faint" style="font-size:12.5px"><a href="/clusters/{live.id}/nodes">Nodes</a> /</div>
			<h1>{name}</h1>
			{#if detail}
				<div class="sub">
					RabbitMQ {detail.node.rabbitmqVersion} · Erlang {detail.node.erlangVersion} · {detail.node.type} node · {detail.node.processors} cores
					{#if detail.node.running}· up {duration(detail.node.uptimeMs)}{/if}
				</div>
			{/if}
		</div>
		<span class="spacer"></span>
		{#if can('admin')}<button class="btn" onclick={resetStats}><Icon name="refresh" />Reset stats</button>{/if}
	</div>

	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !detail}
		{#if !error}<Loading />{/if}
	{:else}
		{@const n = detail.node}
		{#if !n.running}
			<div class="alert bad">This node is not running.</div>
		{:else}
			<div class="grid-4">
				<div class="card stat">
					<div class="label">Memory {#if n.memAlarm}<span class="badge bad">alarm</span>{/if}</div>
					<div class="v num">{bytes(n.memUsed)}</div>
					<Meter used={n.memUsed} total={n.memLimit} />
					<div class="faint small">high watermark {bytes(n.memLimit)}</div>
				</div>
				<div class="card stat">
					<div class="label">Disk free {#if n.diskAlarm}<span class="badge bad">alarm</span>{/if}</div>
					<div class="v num">{bytes(n.diskFree)}</div>
					<div class="faint small">alarm below {bytes(n.diskFreeLimit)}</div>
				</div>
				<div class="card stat">
					<div class="label">File descriptors</div>
					<div class="v num">{num(n.fdUsed)}</div>
					<Meter used={n.fdUsed} total={n.fdTotal} />
					<div class="faint small">of {num(n.fdTotal)}</div>
				</div>
				<div class="card stat">
					<div class="label">Erlang processes</div>
					<div class="v num">{num(n.procUsed)}</div>
					<Meter used={n.procUsed} total={n.procTotal} />
					<div class="faint small">of {compact(n.procTotal)} · run queue {n.runQueue}</div>
				</div>
			</div>

			<div class="split" style="margin-top:14px">
				<div class="card">
					<div class="card-head">
						<h2>Memory breakdown</h2>
						<span class="spacer"></span>
						<span class="faint small">{bytes(detail.memoryTotal)} by {detail.memoryStrategy}</span>
					</div>
					<div class="card-body stack">
						<div class="bar">
							{#each groups as g, i}
								<div style="width:{pct(g.total, memSum)}%;background:{palette[i % palette.length]}" title="{g.group}: {bytes(g.total)}"></div>
							{/each}
						</div>
						<table class="t">
							<tbody>
								{#each groups as g, i}
									<tr class="grp">
										<td><span class="sw" style="background:{palette[i % palette.length]}"></span><b>{g.group}</b></td>
										<td class="r num">{bytes(g.total)}</td>
										<td class="r num faint">{pct(g.total, memSum).toFixed(1)}%</td>
									</tr>
									{#each g.parts as p}
										<tr><td class="part mono">{p.category}</td><td class="r num">{bytes(p.bytes)}</td><td class="r num faint">{pct(p.bytes, memSum).toFixed(1)}%</td></tr>
									{/each}
								{/each}
							</tbody>
						</table>
					</div>
				</div>
				<div class="stack">
					<div class="card">
						<div class="card-head"><h2>Health checks</h2></div>
						<table class="t">
							<tbody>
								{#each health as hc}
									<tr>
										<td>{hc.check}</td>
										<td class="r">
											<span class="badge {hc.status === 'ok' ? 'ok' : hc.status === 'failed' ? 'bad' : 'warn'}">{hc.status}</span>
										</td>
									</tr>
									{#if hc.reason}<tr><td colspan="2" class="faint small">{hc.reason}</td></tr>{/if}
								{:else}
									<tr><td class="faint">{n.endpoint ? 'Not checked yet.' : 'No configured management URL reaches this node, so its node-local checks cannot run.'}</td></tr>
								{/each}
							</tbody>
						</table>
					</div>
					<div class="card">
						<div class="card-head"><h2>Partitions and plugins</h2></div>
						<div class="card-body stack">
							<div>
								{#if n.partitions.length}<div class="alert bad">Partitioned from {n.partitions.join(', ')}</div>
								{:else}<span class="badge ok">no partitions</span>{/if}
								{#if n.beingDrained}<span class="badge warn">in maintenance mode</span>{/if}
							</div>
							<div class="row" style="gap:4px">{#each n.enabledPlugins as p}<span class="badge">{p.replace(/^rabbitmq_/, '')}</span>{/each}</div>
						</div>
					</div>
				</div>
			</div>
		{/if}

		<div class="card" style="margin-top:14px">
			<div class="card-head">
				<h2>Management object</h2>
				<span class="faint small">applications, config files, auth mechanisms, exchange types, cluster links…</span>
				<span class="spacer"></span>
				<button class="btn sm" onclick={() => (showRaw = !showRaw)}>{showRaw ? 'Hide' : 'Show'}</button>
			</div>
			{#if showRaw}<div class="card-body"><JsonView value={detail.raw} /></div>{/if}
		</div>
	{/if}
</div>

<style>
	.grid-4 {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: 14px;
	}
	.stat {
		padding: 12px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.label {
		font-size: 12px;
		color: var(--text-2);
	}
	.v {
		font-size: 22px;
		font-weight: 600;
	}
	.small {
		font-size: 12px;
	}
	.split {
		display: grid;
		grid-template-columns: minmax(0, 1.4fr) minmax(280px, 1fr);
		gap: 14px;
		align-items: start;
	}
	@media (max-width: 1000px) {
		.split {
			grid-template-columns: 1fr;
		}
	}
	.bar {
		display: flex;
		height: 14px;
		border-radius: 4px;
		overflow: hidden;
		border: 1px solid var(--border);
	}
	.sw {
		display: inline-block;
		width: 10px;
		height: 10px;
		border-radius: 2px;
		margin-right: 7px;
	}
	.part {
		padding-left: 30px !important;
		font-size: 12px;
		color: var(--text-2);
	}
	.grp td {
		background: var(--bg-sunken);
	}
</style>
