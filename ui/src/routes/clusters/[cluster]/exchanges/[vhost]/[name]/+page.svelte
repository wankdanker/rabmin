<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from '$lib/components/Confirm.svelte';
	import { rate } from '$lib/format';
	import { exName } from '$lib/rabbit';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import Bindings from '$lib/components/Bindings.svelte';
	import PublishForm from '$lib/components/PublishForm.svelte';

	const live = getLive();
	const cluster = $derived(findCluster(live.id));
	const vhost = $derived(page.params.vhost!);
	const pathName = $derived(page.params.name!);
	const name = $derived(pathName === 'amq.default' ? '' : pathName);
	let d = $state<Schemas['ExchangeDetail'] | null>(null);
	let error = $state<string | null>(null);
	let showRaw = $state(false);

	async function load() {
		try {
			d = await unwrap(api.GET('/v1/clusters/{cluster}/exchanges/{vhost}/{name}', { params: { path: { cluster: live.id, vhost, name: pathName } } }));
			error = null;
		} catch (e) {
			error = errMsg(e);
		}
	}
	$effect(() => {
		pathName;
		load();
		const t = setInterval(() => !document.hidden && load(), 10000);
		return () => clearInterval(t);
	});

	async function remove() {
		if (
			!(await confirm({
				title: 'Delete exchange',
				message: `Delete ${name} in ${vhost}? Its ${(d?.source.length ?? 0) + (d?.destination.length ?? 0)} bindings go with it. Publishers using it will get errors.`,
				confirmText: 'Delete exchange',
				danger: true,
				typeToConfirm: name
			}))
		)
			return;
		try {
			await unwrap(api.DELETE('/v1/clusters/{cluster}/exchanges/{vhost}/{name}', { params: { path: { cluster: live.id, vhost, name } } }));
			toast(`Deleted ${name}`);
			goto(`/clusters/${live.id}/exchanges`);
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}
	const builtin = $derived(name === '' || name.startsWith('amq.'));
</script>

<svelte:head><title>{exName(name)} · Exchanges · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<div class="faint" style="font-size:12.5px"><a href="/clusters/{live.id}/exchanges">Exchanges</a> / <span class="mono">{vhost}</span> /</div>
			<h1>{exName(name)}</h1>
			{#if d}
				<div class="sub">
					<span class="badge">{d.exchange.type}</span>
					{d.exchange.durable ? 'durable' : 'transient'}{d.exchange.autoDelete ? ', auto-delete' : ''}{d.exchange.internal ? ', internal' : ''}
					{#if d.exchange.policy}· policy <b>{d.exchange.policy}</b>{/if}
					· in {rate(d.exchange.rateIn)} · out {rate(d.exchange.rateOut)}
				</div>
			{/if}
		</div>
		<span class="spacer"></span>
		{#if can('operator') && !builtin}<button class="btn danger" onclick={remove}><Icon name="trash" />Delete</button>{/if}
	</div>
	{#if error}<div class="alert bad">{error}</div>{/if}
	{#if !d}
		{#if !error}<Loading />{/if}
	{:else}
		{#if name === ''}
			<div class="alert info" style="margin-bottom:14px">
				The default exchange routes a message to the queue whose name equals the routing key. Every queue is bound to it implicitly.
			</div>
		{/if}
		<div class="grid-2">
			<div class="stack">
				{#if name !== ''}
					<div class="card">
						<div class="card-head"><h2>Bindings from this exchange</h2></div>
						<div class="card-body"><Bindings cluster={live.id} {vhost} {name} side="source" kind="exchange" bindings={d.source} onchange={load} /></div>
					</div>
					<div class="card">
						<div class="card-head"><h2>Bindings to this exchange</h2></div>
						<div class="card-body"><Bindings cluster={live.id} {vhost} {name} side="destination" kind="exchange" bindings={d.destination} onchange={load} /></div>
					</div>
				{/if}
				{#if JSON.stringify(d.exchange.arguments) !== '{}'}
					<div class="card">
						<div class="card-head"><h2>Arguments</h2></div>
						<div class="card-body"><JsonView value={d.exchange.arguments} /></div>
					</div>
				{/if}
			</div>
			<div class="stack">
				{#if can('operator') && !d.exchange.internal}
					<div class="card">
						<div class="card-head"><h2>Publish a message</h2></div>
						<div class="card-body"><PublishForm cluster={live.id} {vhost} exchange={name} /></div>
					</div>
				{/if}
				<div class="card">
					<div class="card-head">
						<h2>Management object</h2>
						<span class="spacer"></span>
						<button class="btn sm" onclick={() => (showRaw = !showRaw)}>{showRaw ? 'Hide' : 'Show'}</button>
					</div>
					{#if showRaw}<div class="card-body"><JsonView value={d.raw} /></div>{/if}
				</div>
			</div>
		</div>
	{/if}
</div>
