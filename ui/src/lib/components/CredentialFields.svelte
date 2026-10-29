<script lang="ts">
	import type { Schemas } from '$lib/api/client';

	type CredIn = Schemas['CredentialIn'];
	let {
		value = $bindable(),
		stored = null,
		idPrefix = 'cred'
	}: { value: CredIn; stored?: Schemas['CredentialOut'] | null; idPrefix?: string } = $props();

	const keep = (has: boolean | undefined) => (stored && stored.type === value.type && has ? 'Stored · leave blank to keep' : '');
</script>

<div class="stack">
	<label class="field">
		<span>Authentication</span>
		<select bind:value={value.type} id="{idPrefix}-type">
			<option value="basic">Username and password</option>
			<option value="bearer">OAuth2 bearer token</option>
		</select>
	</label>
	{#if value.type === 'basic'}
		<div class="grid-2">
			<label class="field"><span>Username</span><input bind:value={value.username} autocomplete="off" id="{idPrefix}-user" /></label>
			<label class="field"
				><span>Password</span><input
					type="password"
					bind:value={value.password}
					autocomplete="new-password"
					id="{idPrefix}-pass"
					placeholder={keep(stored?.hasPassword)}
				/></label
			>
		</div>
	{:else}
		<label class="field"
			><span>Access token</span><input type="password" bind:value={value.token} autocomplete="off" placeholder={keep(stored?.hasToken)} />
			<span class="hint">For brokers whose management API uses OAuth2 (rabbitmq_auth_backend_oauth2). Tokens expire; update it here when it does.</span></label
		>
	{/if}
</div>
