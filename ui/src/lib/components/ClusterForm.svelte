<script lang="ts">
	import { untrack } from 'svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import CredentialFields from './CredentialFields.svelte';
	import Icon from './Icon.svelte';

	type ClusterOut = Schemas['ClusterOut'];
	let { existing = null, onsaved }: { existing?: ClusterOut | null; onsaved: (c: ClusterOut) => void } = $props();

	// The form edits a copy of the cluster as it was when opened.
	const ex = untrack(() => existing);
	const isNew = !ex;
	let name = $state(ex?.name ?? '');
	let id = $state(ex?.id ?? '');
	let description = $state(ex?.description ?? '');
	let mgmt = $state((ex?.managementUrls ?? ['http://localhost:15672']).join('\n'));
	let prom = $state((ex?.prometheusUrls ?? []).join('\n'));
	let amqp = $state((ex?.amqpUrls ?? []).join('\n'));
	let useTLS = $state(!!ex?.tls);
	let tls = $state<Schemas['TLSIn']>({ ca: ex?.tls?.ca ?? '', cert: ex?.tls?.cert ?? '', key: '', insecureSkipVerify: ex?.tls?.insecureSkipVerify ?? false });
	let auth = $state<Schemas['CredentialIn']>({
		type: (ex?.auth.type === 'bearer' ? 'bearer' : 'basic') as Schemas['CredentialIn']['type'],
		username: ex?.auth.username ?? ''
	});

	let busy = $state(false);
	let error = $state<string | null>(null);
	let testing = $state(false);
	let result = $state<Schemas['TestResult'] | null>(null);

	const lines = (s: string) => s.split(/[\n,]/).map((u) => u.trim()).filter(Boolean);
	const autoId = $derived(name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 63));

	function guessProm() {
		prom = lines(mgmt)
			.map((u) => {
				try {
					const x = new URL(u);
					x.port = x.port === '15671' ? '15691' : '15692';
					x.pathname = '';
					return x.toString().replace(/\/$/, '');
				} catch {
					return '';
				}
			})
			.filter(Boolean)
			.join('\n');
	}

	async function test() {
		testing = true;
		result = null;
		try {
			result = await unwrap(
				api.POST('/v1/clusters/test', {
					body: { clusterId: existing?.id, managementUrls: lines(mgmt), prometheusUrls: lines(prom), tls: useTLS ? tls : undefined, auth }
				})
			);
		} catch (e) {
			result = { ok: false, error: errMsg(e), nodes: 0, tags: [], isAdministrator: false, canMonitor: false, plugins: [], statsDisabled: false, endpoints: [], prometheus: [], warnings: [] };
		} finally {
			testing = false;
		}
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = null;
		try {
			const body: Schemas['ClusterIn'] = {
				name,
				description,
				managementUrls: lines(mgmt),
				prometheusUrls: lines(prom),
				amqpUrls: lines(amqp),
				tls: useTLS ? tls : undefined,
				removeTls: !useTLS && !!existing?.tls,
				auth
			};
			let saved: ClusterOut;
			if (isNew) {
				body.id = id || undefined;
				saved = await unwrap(api.POST('/v1/clusters', { body }));
			} else {
				saved = await unwrap(api.PUT('/v1/clusters/{cluster}', { params: { path: { cluster: existing!.id } }, body }));
			}
			onsaved(saved);
		} catch (err) {
			error = errMsg(err);
		} finally {
			busy = false;
		}
	}
</script>

<form onsubmit={save} class="stack">
	{#if error}<div class="alert bad">{error}</div>{/if}

	<div class="card">
		<div class="card-head"><h2>Cluster</h2></div>
		<div class="card-body stack">
			<div class="grid-2">
				<label class="field"><span>Name</span><input bind:value={name} required placeholder="Production" /></label>
				{#if isNew}
					<label class="field"
						><span>ID</span><input bind:value={id} placeholder={autoId || 'production'} pattern="[a-z0-9][a-z0-9-]*" />
						<span class="hint">Used in URLs and the API. Cannot change later.</span></label
					>
				{:else}
					<label class="field"><span>ID</span><input value={existing?.id} disabled /></label>
				{/if}
			</div>
			<label class="field"><span>Description</span><input bind:value={description} placeholder="Optional" /></label>
		</div>
	</div>

	<div class="card">
		<div class="card-head">
			<div>
				<h2>Management API</h2>
				<div class="faint" style="font-size:12.5px">rabmin reads and changes the cluster through the management plugin.</div>
			</div>
		</div>
		<div class="card-body stack">
			<label class="field">
				<span>Management URLs</span>
				<textarea rows="3" bind:value={mgmt} required placeholder="http://rabbit-1:15672&#10;http://rabbit-2:15672&#10;http://rabbit-3:15672"></textarea>
				<span class="hint">One per line, ideally one per node: rabmin fails over between them and runs each node's health checks through its own URL.</span>
			</label>
			<CredentialFields bind:value={auth} stored={existing?.auth} idPrefix="mgmt" />
			<label class="check"><input type="checkbox" bind:checked={useTLS} />Custom TLS settings</label>
			{#if useTLS}
				<div class="grid-3">
					<label class="field"><span>CA certificate (PEM)</span><textarea rows="4" bind:value={tls.ca}></textarea></label>
					<label class="field"><span>Client certificate (PEM)</span><textarea rows="4" bind:value={tls.cert}></textarea></label>
					<label class="field"
						><span>Client key (PEM)</span><textarea rows="4" bind:value={tls.key} placeholder={existing?.tls?.hasKey ? 'Stored · leave blank to keep' : ''}
						></textarea></label
					>
				</div>
				<label class="check"><input type="checkbox" bind:checked={tls.insecureSkipVerify} />Skip certificate verification (not recommended)</label>
			{/if}
		</div>
	</div>

	<div class="card">
		<div class="card-head">
			<div>
				<h2>Metrics and messaging</h2>
				<div class="faint" style="font-size:12.5px">Optional. Prometheus keeps rates working when the management stats database is disabled.</div>
			</div>
		</div>
		<div class="card-body stack">
			<label class="field">
				<span class="row" style="gap:8px">Prometheus URLs <button type="button" class="btn sm ghost" onclick={guessProm}>Derive from management URLs</button></span>
				<textarea rows="3" bind:value={prom} placeholder="http://rabbit-1:15692"></textarea>
				<span class="hint">rabbitmq_prometheus endpoints, one per node.</span>
			</label>
			<label class="field">
				<span>AMQP URLs</span>
				<textarea rows="2" bind:value={amqp} placeholder="amqp://rabbit-1:5672"></textarea>
				<span class="hint">Without credentials. Used by publishing and message browsing in later releases.</span>
			</label>
		</div>
	</div>

	<div class="card">
		<div class="card-head">
			<h2>Test</h2>
			<span class="spacer"></span>
			<button type="button" class="btn" onclick={test} disabled={testing || lines(mgmt).length === 0}>
				{#if testing}<span class="spin"></span>{:else}<Icon name="zap" />{/if}Test connection
			</button>
		</div>
		{#if result}
			<div class="card-body stack">
				{#if !result.ok}
					<div class="alert bad">Connection failed: {result.error}</div>
				{:else}
					<div class="alert {result.isAdministrator ? 'info' : 'warn'}">
						Connected to <b>{result.clusterName}</b>: {result.productName}
						{result.rabbitmqVersion} on Erlang {result.erlangVersion}, {result.nodes} node{result.nodes === 1 ? '' : 's'}. Logged in as
						<b>{result.user}</b>{result.tags.length ? ` (${result.tags.join(', ')})` : ' (no tags)'}.
					</div>
				{/if}
				{#each result.warnings as w}<div class="alert warn">{w}</div>{/each}
				{#if result.endpoints.length || result.prometheus.length}
					<table class="t">
						<thead><tr><th>Endpoint</th><th>Node</th><th>Status</th><th class="r">Latency</th></tr></thead>
						<tbody>
							{#each result.endpoints as e}
								<tr>
									<td class="mono">{e.url}</td><td>{e.node ?? '—'}</td>
									<td>{#if e.ok}<span class="badge ok">management OK</span>{:else}<span class="badge bad" title={e.error}>failed</span> <span class="faint" style="font-size:12px">{e.error}</span>{/if}</td>
									<td class="r num">{e.latencyMs} ms</td>
								</tr>
							{/each}
							{#each result.prometheus as p}
								<tr>
									<td class="mono">{p.url}</td><td>{p.node ?? '—'}</td>
									<td>{#if p.ok}<span class="badge ok">Prometheus OK</span>{:else}<span class="badge bad">failed</span> <span class="faint" style="font-size:12px">{p.error}</span>{/if}</td>
									<td></td>
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
				{#if result.plugins.length}
					<div>
						<div class="label" style="margin-bottom:4px">Plugins</div>
						<div class="row" style="gap:4px">{#each result.plugins as p}<span class="badge">{p.replace(/^rabbitmq_/, '')}</span>{/each}</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<div class="row">
		<button class="btn primary" disabled={busy}>{busy ? 'Saving…' : isNew ? 'Add cluster' : 'Save changes'}</button>
	</div>
</form>
