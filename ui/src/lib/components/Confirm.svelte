<script lang="ts" module>
	type Req = {
		title: string;
		message: string;
		confirmText?: string;
		danger?: boolean;
		/** When set, the user must type this to confirm. */
		typeToConfirm?: string;
		resolve: (ok: boolean) => void;
	};
	let req = $state<Req | null>(null);

	/** Asks the user to confirm; resolves true when they do. */
	export function confirm(opts: Omit<Req, 'resolve'>): Promise<boolean> {
		return new Promise((resolve) => {
			req = { ...opts, resolve };
		});
	}
</script>

<script lang="ts">
	import Modal from './Modal.svelte';

	let typed = $state('');
	let open = $state(false);
	$effect(() => {
		open = !!req;
		typed = '';
	});
	$effect(() => {
		if (!open && req) finish(false);
	});

	function finish(ok: boolean) {
		const r = req;
		req = null;
		r?.resolve(ok);
	}
</script>

{#if req}
	<Modal bind:open title={req.title} width={480}>
		<p style="margin-top:0; white-space: pre-line">{req.message}</p>
		{#if req.typeToConfirm}
			<label class="field">
				<span>Type <code>{req.typeToConfirm}</code> to confirm</span>
				<input bind:value={typed} autocomplete="off" />
			</label>
		{/if}
		{#snippet footer()}
			<button class="btn" onclick={() => finish(false)}>Cancel</button>
			<button
				class="btn {req?.danger ? 'danger solid' : 'primary'}"
				disabled={!!req?.typeToConfirm && typed !== req.typeToConfirm}
				onclick={() => finish(true)}>{req?.confirmText ?? 'Confirm'}</button
			>
		{/snippet}
	</Modal>
{/if}
