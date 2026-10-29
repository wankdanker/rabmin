<script lang="ts">
	import { untrack } from 'svelte';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { findCluster } from '$lib/clusters.svelte';
	import { toast } from '$lib/toast.svelte';
	import KeyValueEditor from './KeyValueEditor.svelte';
	import Icon from './Icon.svelte';

	/** Publishes through the management API. exchange "" is the default exchange; queue sets its routing key. */
	let { cluster, vhost, exchange, queue = '' }: { cluster: string; vhost: string; exchange: string; queue?: string } = $props();

	let routingKey = $state(untrack(() => queue));
	let payload = $state('');
	let encoding = $state<'string' | 'base64'>('string');
	let contentType = $state('application/json');
	let persistent = $state(true);
	let priority = $state('');
	let messageId = $state('');
	let correlationId = $state('');
	let expiration = $state('');
	let headers = $state<Record<string, unknown>>({});
	let credential = $state('');
	let more = $state(false);
	let busy = $state(false);
	let result = $state<{ routed: boolean; at: Date } | null>(null);
	const profiles = $derived(findCluster(cluster)?.credentials ?? []);

	async function send(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		try {
			const r = await unwrap(
				api.POST('/v1/clusters/{cluster}/exchanges/{vhost}/{name}/publish', {
					params: { path: { cluster, vhost, name: exchange === '' ? 'amq.default' : exchange } },
					body: {
						routingKey,
						payload,
						payloadEncoding: encoding,
						credential: credential || undefined,
						properties: {
							contentType: contentType || undefined,
							deliveryMode: persistent ? 2 : 1,
							priority: priority === '' ? undefined : Number(priority),
							messageId: messageId || undefined,
							correlationId: correlationId || undefined,
							expiration: expiration || undefined,
							headers: Object.keys(headers).length ? headers : undefined
						}
					}
				})
			);
			result = { routed: r.routed, at: new Date() };
			if (!r.routed) toast('Published, but no queue received it (unroutable)', 'bad');
			else toast('Published');
		} catch (err) {
			toast(errMsg(err), 'bad');
		} finally {
			busy = false;
		}
	}
</script>

<form class="stack" onsubmit={send}>
	<div class="grid-2">
		<label class="field">
			<span>Routing key</span><input bind:value={routingKey} placeholder={queue ? queue : 'orders.created.eu'} disabled={!!queue} />
			{#if queue}<span class="hint">Published through the default exchange, straight to {queue}.</span>{/if}
		</label>
		<label class="field"><span>Content type</span><input bind:value={contentType} list="ctypes" /></label>
	</div>
	<datalist id="ctypes"><option value="application/json"></option><option value="text/plain"></option><option value="application/octet-stream"></option></datalist>
	<label class="field">
		<span class="row" style="gap:10px"
			>Payload
			<label class="check"><input type="radio" bind:group={encoding} value="string" />text</label>
			<label class="check"><input type="radio" bind:group={encoding} value="base64" />base64</label></span
		>
		<textarea rows="5" class="mono" bind:value={payload} placeholder={encoding === 'base64' ? 'aGVsbG8=' : '{"hello": "world"}'}></textarea>
	</label>
	<div class="row">
		<label class="check"><input type="checkbox" bind:checked={persistent} />Persistent (delivery mode 2)</label>
		<button type="button" class="btn sm ghost" onclick={() => (more = !more)}>{more ? 'Fewer' : 'More'} properties</button>
	</div>
	{#if more}
		<div class="grid-2">
			<label class="field"><span>Message id</span><input bind:value={messageId} /></label>
			<label class="field"><span>Correlation id</span><input bind:value={correlationId} /></label>
			<label class="field"><span>Priority</span><input type="number" min="0" max="255" bind:value={priority} /></label>
			<label class="field"><span>Expiration (ms)</span><input bind:value={expiration} placeholder="60000" /></label>
		</div>
		<div class="field"><span>Headers</span><KeyValueEditor bind:value={headers} keyPlaceholder="header" /></div>
		{#if profiles.length}
			<label class="field"
				><span>Publish as</span><select bind:value={credential}>
					<option value="">Management user</option>
					{#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
				</select></label
			>
		{/if}
	{/if}
	<div class="row">
		<button class="btn primary" disabled={busy}><Icon name="send" />Publish</button>
		{#if result}
			<span class="badge {result.routed ? 'ok' : 'warn'}">{result.routed ? 'routed' : 'unroutable'}</span>
			<span class="faint" style="font-size:12px">{result.at.toLocaleTimeString()}</span>
		{/if}
	</div>
</form>
