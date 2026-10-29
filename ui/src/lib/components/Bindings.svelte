<script lang="ts">
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { confirm } from './Confirm.svelte';
	import { seg, exName } from '$lib/rabbit';
	import KeyValueEditor from './KeyValueEditor.svelte';
	import Icon from './Icon.svelte';

	type Binding = Schemas['Binding'];
	/** side: which end this page is. "source" lists bindings from the exchange; "destination" those to the exchange or queue. */
	let {
		cluster,
		vhost,
		name,
		side,
		kind,
		bindings,
		onchange
	}: {
		cluster: string;
		vhost: string;
		name: string;
		side: 'source' | 'destination';
		kind: 'exchange' | 'queue';
		bindings: Binding[];
		onchange: () => void;
	} = $props();

	let adding = $state(false);
	let other = $state('');
	let otherType = $state<'queue' | 'exchange'>('queue');
	let routingKey = $state('');
	let args = $state<Record<string, unknown>>({});

	const link = (type: string, n: string) =>
		type === 'queue' ? `/clusters/${cluster}/queues/${seg(vhost)}/${seg(n)}` : `/clusters/${cluster}/exchanges/${seg(vhost)}/${seg(n === '' ? 'amq.default' : n)}`;

	async function add(e: SubmitEvent) {
		e.preventDefault();
		const body =
			side === 'source'
				? { vhost, source: name, destination: other, destinationType: otherType, routingKey, arguments: args }
				: { vhost, source: other, destination: name, destinationType: kind, routingKey, arguments: args };
		try {
			await unwrap(api.POST('/v1/clusters/{cluster}/bindings', { params: { path: { cluster } }, body }));
			toast('Binding added');
			adding = false;
			other = routingKey = '';
			args = {};
			onchange();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	async function remove(b: Binding) {
		if (!(await confirm({ title: 'Remove binding', message: `Unbind ${exName(b.source)} → ${b.destination} (${b.routingKey || 'no routing key'})?`, confirmText: 'Remove', danger: true })))
			return;
		try {
			await unwrap(
				api.DELETE('/v1/clusters/{cluster}/bindings/{vhost}/{source}/{destinationType}/{destination}/{propertiesKey}', {
					params: {
						path: { cluster, vhost, source: b.source, destinationType: b.destinationType as 'queue', destination: b.destination, propertiesKey: b.propertiesKey }
					}
				})
			);
			toast('Binding removed');
			onchange();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
</script>

<div class="stack">
	{#if bindings.length === 0}
		<div class="faint">No bindings.</div>
	{:else}
		<table class="t">
			<thead><tr><th>{side === 'source' ? 'To' : 'From'}</th><th>Routing key</th><th>Arguments</th>{#if can('operator')}<th></th>{/if}</tr></thead>
			<tbody>
				{#each bindings as b (b.source + b.destinationType + b.destination + b.propertiesKey)}
					<tr>
						<td>
							{#if side === 'source'}
								<span class="badge">{b.destinationType}</span> <a href={link(b.destinationType, b.destination)}>{b.destination}</a>
							{:else}
								<span class="badge">exchange</span> <a href={link('exchange', b.source)}>{exName(b.source)}</a>
							{/if}
						</td>
						<td class="mono">{b.routingKey || '—'}</td>
						<td class="mono small">{JSON.stringify(b.arguments) === '{}' ? '' : JSON.stringify(b.arguments)}</td>
						{#if can('operator')}
							<td class="r">
								{#if b.source !== ''}<button class="btn sm danger" aria-label="Remove binding" onclick={() => remove(b)}><Icon name="x" size={13} /></button>{/if}
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
	{#if can('operator') && !(kind === 'exchange' && name === '')}
		{#if !adding}
			<div><button class="btn sm" onclick={() => (adding = true)}><Icon name="plus" size={13} />Add binding</button></div>
		{:else}
			<form class="card-body stack addb" onsubmit={add}>
				<div class="grid-3">
					{#if side === 'source'}
						<label class="field"
							><span>Destination type</span><select bind:value={otherType}><option value="queue">queue</option><option value="exchange">exchange</option></select></label
						>
						<label class="field"><span>Destination</span><input bind:value={other} required /></label>
					{:else}
						<label class="field"><span>Source exchange</span><input bind:value={other} required /></label>
					{/if}
					<label class="field"><span>Routing key</span><input bind:value={routingKey} placeholder="orders.#" /></label>
				</div>
				<div class="field">
					<span>Arguments <span class="faint">(headers exchanges: x-match plus header values)</span></span>
					<KeyValueEditor bind:value={args} suggestions={['x-match']} />
				</div>
				<div class="row">
					<button class="btn primary sm">Bind</button>
					<button type="button" class="btn sm" onclick={() => (adding = false)}>Cancel</button>
				</div>
			</form>
		{/if}
	{/if}
</div>

<style>
	.small {
		font-size: 12px;
	}
	.addb {
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		background: var(--bg-sunken);
	}
</style>
