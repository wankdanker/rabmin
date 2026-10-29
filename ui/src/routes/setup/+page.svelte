<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { session } from '$lib/session.svelte';
	import AuthCard from '$lib/components/AuthCard.svelte';

	let username = $state('admin');
	let password = $state('');
	let confirmPw = $state('');
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (password !== confirmPw) {
			error = 'Passwords do not match';
			return;
		}
		busy = true;
		error = null;
		try {
			session.me = await unwrap(api.POST('/v1/setup', { body: { username, password } }));
			await goto('/systems/new');
		} catch (err) {
			error = errMsg(err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Set up · rabmin</title></svelte:head>

<AuthCard title="Welcome to rabmin" subtitle="Create the first admin account to get started">
	<form onsubmit={submit} class="stack">
		{#if error}<div class="alert bad">{error}</div>{/if}
		<label class="field"><span>Username</span><input bind:value={username} autocomplete="username" required /></label>
		<label class="field"
			><span>Password</span><input type="password" bind:value={password} autocomplete="new-password" minlength="8" required />
			<span class="hint">At least 8 characters.</span></label
		>
		<label class="field"
			><span>Confirm password</span><input type="password" bind:value={confirmPw} autocomplete="new-password" required /></label
		>
		<button class="btn primary" style="justify-content:center" disabled={busy}>{busy ? 'Creating…' : 'Create admin'}</button>
	</form>
</AuthCard>
