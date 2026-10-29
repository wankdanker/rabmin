<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { session } from '$lib/session.svelte';
	import { refreshClusters } from '$lib/clusters.svelte';
	import AuthCard from '$lib/components/AuthCard.svelte';

	let username = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = null;
		try {
			session.me = await unwrap(api.POST('/v1/auth/login', { body: { username, password } }));
			await refreshClusters();
			const next = page.url.searchParams.get('next');
			await goto(next && next.startsWith('/') && !next.startsWith('//') ? next : '/');
		} catch (err) {
			error = errMsg(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Log in · rabmin</title></svelte:head>

<AuthCard title="Log in" subtitle="Manage your RabbitMQ clusters">
	<form onsubmit={submit} class="stack">
		{#if error}<div class="alert bad">{error}</div>{/if}
		<label class="field"><span>Username</span><input bind:value={username} autocomplete="username" required /></label>
		<label class="field"
			><span>Password</span><input type="password" bind:value={password} autocomplete="current-password" required /></label
		>
		<button class="btn primary" style="justify-content:center" disabled={busy}>{busy ? 'Logging in…' : 'Log in'}</button>
	</form>
</AuthCard>
