<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { getLive } from '$lib/context';
	import { findCluster } from '$lib/clusters.svelte';
	import { api, unwrap, errMsg, type Schemas } from '$lib/api/client';
	import { can } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';
	import { bytes, bytesRate, num, ago, dateTime } from '$lib/format';
	import Icon from '$lib/components/Icon.svelte';
	import Loading from '$lib/components/Loading.svelte';
	import Empty from '$lib/components/Empty.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import JsonView from '$lib/components/JsonView.svelte';
	import Pager from '$lib/components/Pager.svelte';

	type Conn = Schemas['Connection'];
	const live = getLive();
	const cluster = $derived(findCluster(live.id));

	let tab = $state<'amqp' | 'stream'>('amqp');
	let q = $state(page.url.searchParams.get('q') ?? '');
	let vhost = $state(page.url.searchParams.get('vhost') ?? '');
	let user = $state(page.url.searchParams.get('user') ?? '');
	let node = $state('');
	let protocol = $state('');
	let sort = $state('recvRate');
	let desc = $state(true);
	let pageNo = $state(1);
	let pageSize = $state(50);
	let auto = $state(true);
	let data = $state<Schemas['ConnectionsPage'] | null>(null);
	let streams = $state<Schemas['StreamConnectionsPage'] | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let expanded = $state<string | null>(null);
	let detail = $state<Schemas['ConnectionDetail'] | null>(null);

	const nodes = $derived(live.snapshot?.nodes.map((n) => n.name) ?? []);

	async function load() {
		loading = true;
		const query = { q: q || undefined, vhost: vhost || undefined, user: user || undefined, node: node || undefined, sort, desc, page: pageNo, pageSize };
		try {
			if (tab === 'amqp') {
				data = await unwrap(
					api.GET('/v1/clusters/{cluster}/connections', { params: { path: { cluster: live.id }, query: { ...query, protocol: protocol || undefined } } })
				);
			} else {
				streams = await unwrap(api.GET('/v1/clusters/{cluster}/stream-connections', { params: { path: { cluster: live.id }, query } }));
			}
			error = null;
		} catch (e) {
			error = errMsg(e);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
		const t = setInterval(() => auto && !document.hidden && load(), 5000);
		return () => clearInterval(t);
	});

	function apply(e?: Event) {
		e?.preventDefault();
		pageNo = 1;
		load();
	}

	function sortBy(k: string) {
		if (sort === k) desc = !desc;
		else {
			sort = k;
			desc = !['clientName', 'user', 'vhost', 'node', 'protocol'].includes(k);
		}
		apply();
	}

	async function toggle(c: Conn) {
		if (expanded === c.name) {
			expanded = null;
			return;
		}
		expanded = c.name;
		detail = null;
		try {
			detail = await unwrap(api.GET('/v1/clusters/{cluster}/connections/{name}', { params: { path: { cluster: live.id, name: c.name } } }));
		} catch (e) {
			toast(errMsg(e), 'bad');
		}
	}

	// Close one connection.
	let closeOpen = $state(false);
	let closing = $state<Conn | null>(null);
	let reason = $state('');
	function askClose(c: Conn) {
		closing = c;
		reason = '';
		closeOpen = true;
	}
	async function doClose(e: SubmitEvent) {
		e.preventDefault();
		if (!closing) return;
		try {
			await unwrap(
				api.DELETE('/v1/clusters/{cluster}/connections/{name}', { params: { path: { cluster: live.id, name: closing.name }, query: { reason: reason || undefined } } })
			);
			toast(`Closed ${closing.clientName || closing.name}`);
			closeOpen = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	// Close many.
	let bulkOpen = $state(false);
	let bulk = $state({ user: '', vhost: '', clientName: '', reason: '' });
	let bulkMatches = $state<number | null>(null);
	let bulkConfirm = $state('');
	function openBulk(c?: Conn) {
		bulk = { user: c?.user ?? '', vhost: '', clientName: c?.clientName ?? '', reason: '' };
		bulkConfirm = '';
		bulkMatches = null;
		bulkOpen = true;
		preview();
	}
	async function preview() {
		if (!bulk.user && !bulk.vhost && !bulk.clientName) {
			bulkMatches = null;
			return;
		}
		try {
			const all = await unwrap(
				api.GET('/v1/clusters/{cluster}/connections', {
					params: { path: { cluster: live.id }, query: { user: bulk.user || undefined, vhost: bulk.vhost || undefined, pageSize: 2000 } }
				})
			);
			bulkMatches = all.items.filter((c) => c.type !== 'direct' && (!bulk.clientName || c.clientName === bulk.clientName)).length;
		} catch {
			bulkMatches = null;
		}
	}
	async function doBulk(e: SubmitEvent) {
		e.preventDefault();
		try {
			const r = await unwrap(
				api.POST('/v1/clusters/{cluster}/connections/close', {
					params: { path: { cluster: live.id } },
					body: { user: bulk.user || undefined, vhost: bulk.vhost || undefined, clientName: bulk.clientName || undefined, reason: bulk.reason || undefined }
				})
			);
			toast(`Closed ${r.closed} connection${r.closed === 1 ? '' : 's'}${r.errors.length ? `, ${r.errors.length} failed` : ''}`, r.errors.length ? 'bad' : 'ok');
			bulkOpen = false;
			load();
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}

	const cols: [string, string, boolean][] = [
		['clientName', 'Client', false],
		['user', 'User / vhost', false],
		['node', 'Node', false],
		['protocol', 'Protocol', false],
		['channels', 'Channels', true],
		['recvRate', 'From client', true],
		['sendRate', 'To client', true],
		['connectedAt', 'Connected', true]
	];
</script>

<svelte:head><title>Connections · {cluster?.name ?? live.id} · rabmin</title></svelte:head>

<div class="page" style="max-width:none">
	<div class="page-head">
		<div>
			<h1>Connections</h1>
			<div class="sub">
				{#if tab === 'amqp' && data}{num(data.filtered)} of {num(data.total)} connections{:else if tab === 'stream' && streams}{num(streams.filtered)} stream connections{/if}
			</div>
		</div>
		<span class="spacer"></span>
		<div class="seg">
			<button class:on={tab === 'amqp'} onclick={() => ((tab = 'amqp'), apply())}>AMQP, MQTT, STOMP</button>
			<button class:on={tab === 'stream'} onclick={() => ((tab = 'stream'), (sort = 'recvRate'), apply())}>Stream</button>
		</div>
		<label class="check"><input type="checkbox" bind:checked={auto} />Auto refresh</label>
		<button class="btn" onclick={() => load()} disabled={loading}><Icon name="refresh" />Refresh</button>
		{#if can('operator') && tab === 'amqp'}<button class="btn" onclick={() => openBulk()}><Icon name="x" />Close many…</button>{/if}
	</div>

	<form class="card filters" onsubmit={apply}>
		<label class="field"><span>Search</span><input type="search" bind:value={q} placeholder="client, product, address…" /></label>
		<label class="field"><span>Virtual host</span><input bind:value={vhost} placeholder="exact" /></label>
		<label class="field"><span>User</span><input bind:value={user} placeholder="exact" /></label>
		<label class="field"
			><span>Node</span><select bind:value={node} onchange={() => apply()}>
				<option value="">All nodes</option>
				{#each nodes as n}<option value={n}>{n}</option>{/each}
			</select></label
		>
		{#if tab === 'amqp'}
			<label class="field"
				><span>Protocol</span><select bind:value={protocol} onchange={() => apply()}>
					<option value="">All</option><option value="AMQP 0-9-1">AMQP 0-9-1</option><option value="AMQP 1.0">AMQP 1.0</option><option value="MQTT">MQTT</option
					><option value="STOMP">STOMP</option><option value="Web">Web MQTT/STOMP</option>
				</select></label
			>
		{/if}
		<div class="field"><span>&nbsp;</span><button class="btn primary"><Icon name="filter" />Apply</button></div>
	</form>

	{#if error}<div class="alert bad" style="margin-top:14px">{error}</div>{/if}

	<div class="card table-wrap" style="margin-top:14px">
		{#if tab === 'stream'}
			{#if !streams}
				<Loading />
			{:else if !streams.available}
				<Empty icon="stream" title="Stream management is not enabled">Enable the rabbitmq_stream_management plugin to see stream protocol connections.</Empty>
			{:else if streams.items.length === 0}
				<Empty icon="plug" title="No stream connections" />
			{:else}
				<table class="t">
					<thead><tr><th>Client</th><th>User / vhost</th><th>Node</th><th class="r">Publishers</th><th class="r">Consumers</th><th class="r">From client</th><th class="r">To client</th><th>Connected</th></tr></thead>
					<tbody>
						{#each streams.items as c (c.name)}
							<tr>
								<td><b>{c.clientName || '(unnamed)'}</b><div class="faint small">{c.peerHost}:{c.peerPort}</div></td>
								<td>{c.user}<div class="faint small">{c.vhost}</div></td>
								<td>{c.node}</td>
								<td class="r num">{c.publishers}</td>
								<td class="r num">{c.consumers}</td>
								<td class="r num">{bytesRate(c.recvRate)}</td>
								<td class="r num">{bytesRate(c.sendRate)}</td>
								<td class="nowrap">{ago(new Date(c.connectedAt))}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		{:else if !data}
			<Loading />
		{:else if data.items.length === 0}
			<Empty icon="plug" title="No connections match" />
		{:else}
			<table class="t">
				<thead>
					<tr>
						{#each cols as [k, label, right]}
							<th class="sortable" class:r={right} onclick={() => sortBy(k)}
								>{label}{#if sort === k}<span class="arrow">{desc ? '▼' : '▲'}</span>{/if}</th
							>
						{/each}
						{#if can('operator')}<th></th>{/if}
					</tr>
				</thead>
				<tbody>
					{#each data.items as c (c.name)}
						<tr class="clickable" onclick={() => toggle(c)}>
							<td>
								<b>{c.clientName || (c.type === 'direct' ? '(direct)' : '(unnamed)')}</b>
								<div class="faint small">{c.type === 'direct' ? c.name : `${c.peerHost}:${c.peerPort}`}{c.product ? ` · ${c.product}${c.version ? ' ' + c.version : ''}` : ''}</div>
							</td>
							<td>{c.user}<div class="faint small">{c.vhost}</div></td>
							<td class="nowrap">{c.node}</td>
							<td class="nowrap">
								{c.protocol}
								{#if c.tls}<span class="badge ok" title={c.tlsProtocol}>TLS</span>{/if}
								{#if c.state && c.state !== 'running'}<span class="badge warn">{c.state}</span>{/if}
							</td>
							<td class="r num">{c.channels}</td>
							<td class="r num">{bytesRate(c.recvRate)}</td>
							<td class="r num">{bytesRate(c.sendRate)}</td>
							<td class="r nowrap">{ago(new Date(c.connectedAt))}</td>
							{#if can('operator')}
								<td class="nowrap r" onclick={(e) => e.stopPropagation()}>
									{#if c.type !== 'direct'}
										<button class="btn sm danger" title="Close connection" aria-label="Close connection" onclick={() => askClose(c)}><Icon name="x" size={13} /></button>
									{/if}
								</td>
							{/if}
						</tr>
						{#if expanded === c.name}
							<tr class="detail">
								<td colspan="9">
									{#if !detail}<Loading />{:else}
										<div class="grid-2">
											<div class="stack">
												<dl class="kv-list">
													<dt>Name</dt><dd class="mono">{c.name}</dd>
													<dt>Connected at</dt><dd>{dateTime(new Date(c.connectedAt))}</dd>
													<dt>Authentication</dt><dd>{c.authMechanism || '—'}</dd>
													<dt>TLS</dt><dd>{c.tls ? c.tlsProtocol || 'yes' : 'no'}</dd>
													<dt>Heartbeat</dt><dd>{c.heartbeat ? `${c.heartbeat}s` : 'off'}</dd>
													<dt>Frame max</dt><dd>{num(c.frameMax)} bytes</dd>
													<dt>Channel max</dt><dd>{num(c.channelMax)}</dd>
													<dt>Received / sent</dt><dd>{bytes(c.recvBytes)} / {bytes(c.sendBytes)}</dd>
													{#if c.sendPending}<dt>Send pending</dt><dd><span class="badge warn">{bytes(c.sendPending)}</span></dd>{/if}
												</dl>
												{#if can('operator') && c.type !== 'direct'}
													<div class="row">
														<button class="btn sm" onclick={() => openBulk(c)}>Close all from {c.clientName ? `“${c.clientName}”` : c.user}…</button>
													</div>
												{/if}
											</div>
											<div>
												<div class="label" style="margin-bottom:4px">Client properties</div>
												<JsonView value={detail.clientProperties} maxHeight={260} />
											</div>
										</div>
										<div class="label" style="margin:12px 0 4px">Channels ({detail.channels.length})</div>
										{#if detail.channels.length}
											<table class="t inner">
												<thead><tr><th>#</th><th>Mode</th><th class="r">Prefetch</th><th class="r">Consumers</th><th class="r">Unacked</th><th class="r">Unconfirmed</th><th class="r">Publish/s</th><th class="r">Deliver/s</th></tr></thead>
												<tbody>
													{#each detail.channels as ch}
														<tr>
															<td><a href="/clusters/{live.id}/channels?q={encodeURIComponent(ch.name)}">{ch.number}</a></td>
															<td>{ch.confirm ? 'confirm' : ch.transactional ? 'transactional' : '—'}</td>
															<td class="r num">{ch.prefetch || '—'}{ch.globalPrefetch ? ` / ${ch.globalPrefetch} global` : ''}</td>
															<td class="r num">{ch.consumers}</td>
															<td class="r num">{ch.unacked}</td>
															<td class="r num">{ch.unconfirmed}</td>
															<td class="r num">{ch.publishRate.toFixed(1)}</td>
															<td class="r num">{ch.deliverRate.toFixed(1)}</td>
														</tr>
													{/each}
												</tbody>
											</table>
										{:else}<span class="faint">No channels.</span>{/if}
									{/if}
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
	{#if tab === 'amqp' && data}
		<Pager page={pageNo} pageSize={pageSize} total={data.filtered} onpage={(p) => ((pageNo = p), load())} />
	{/if}
</div>

<Modal bind:open={closeOpen} title="Close connection">
	<form id="closeform" class="stack" onsubmit={doClose}>
		<p style="margin:0">Close <b>{closing?.clientName || closing?.name}</b> ({closing?.user} on {closing?.vhost})? The client receives the reason and may reconnect.</p>
		<label class="field"><span>Reason sent to the client</span><input bind:value={reason} placeholder="Closed by an operator via rabmin" maxlength="255" /></label>
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (closeOpen = false)}>Cancel</button>
		<button class="btn danger solid" form="closeform">Close connection</button>
	{/snippet}
</Modal>

<Modal bind:open={bulkOpen} title="Close many connections">
	<form id="bulkform" class="stack" onsubmit={doBulk}>
		<p class="muted" style="margin:0">Closes every client connection matching all filters. Direct (in-node) connections such as shovels are skipped.</p>
		<div class="grid-3">
			<label class="field"><span>User</span><input bind:value={bulk.user} onchange={preview} /></label>
			<label class="field"><span>Virtual host</span><input bind:value={bulk.vhost} onchange={preview} /></label>
			<label class="field"><span>Client name</span><input bind:value={bulk.clientName} onchange={preview} /></label>
		</div>
		<label class="field"><span>Reason</span><input bind:value={bulk.reason} maxlength="255" /></label>
		{#if bulkMatches !== null}
			<div class="alert {bulkMatches ? 'warn' : 'info'}">{bulkMatches} connection{bulkMatches === 1 ? '' : 's'} match.</div>
			{#if bulkMatches > 0}
				<label class="field"><span>Type <b>{bulkMatches}</b> to confirm</span><input bind:value={bulkConfirm} /></label>
			{/if}
		{/if}
	</form>
	{#snippet footer()}
		<button class="btn" onclick={() => (bulkOpen = false)}>Cancel</button>
		<button class="btn danger solid" form="bulkform" disabled={!bulkMatches || bulkConfirm !== String(bulkMatches)}>Close connections</button>
	{/snippet}
</Modal>

<style>
	.filters {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
		gap: 10px;
		padding: 12px 14px;
	}
	.detail td {
		background: var(--bg-sunken) !important;
	}
	.small {
		font-size: 11.5px;
	}
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
	.seg {
		display: flex;
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}
	.seg button {
		border: none;
		background: var(--bg-elev);
		color: var(--text-2);
		padding: 6px 10px;
		font: inherit;
		cursor: pointer;
	}
	.seg button.on {
		background: var(--accent-soft);
		color: var(--accent);
		font-weight: 600;
	}
	table.inner {
		background: var(--bg-elev);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}
</style>
