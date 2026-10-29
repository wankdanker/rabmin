<script lang="ts">
	import { api, unwrap, errMsg } from '$lib/api/client';
	import { session } from '$lib/session.svelte';
	import { toast } from '$lib/toast.svelte';

	let current = $state('');
	let next = $state('');
	let again = $state('');

	async function change(e: SubmitEvent) {
		e.preventDefault();
		if (next !== again) {
			toast('New passwords do not match', 'bad');
			return;
		}
		try {
			await unwrap(api.PUT('/v1/auth/password', { body: { current, new: next } }));
			toast('Password changed');
			current = next = again = '';
		} catch (err) {
			toast(errMsg(err), 'bad');
		}
	}
</script>

<svelte:head><title>Profile · rabmin</title></svelte:head>

<div class="page" style="max-width:640px">
	<div class="page-head">
		<div>
			<h1>{session.me?.user.username}</h1>
			<div class="sub">Role: {session.me?.role} · signed in with a {session.me?.authMethod}</div>
		</div>
	</div>
	<form class="card" onsubmit={change}>
		<div class="card-head"><h2>Change password</h2></div>
		<div class="card-body stack">
			<label class="field"><span>Current password</span><input type="password" bind:value={current} required autocomplete="current-password" /></label>
			<label class="field"><span>New password</span><input type="password" bind:value={next} minlength="8" required autocomplete="new-password" /></label>
			<label class="field"><span>Repeat new password</span><input type="password" bind:value={again} required autocomplete="new-password" /></label>
			<div><button class="btn primary">Change password</button></div>
		</div>
	</form>
</div>
