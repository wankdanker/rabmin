<script lang="ts">
	import { toasts, dismiss } from '$lib/toast.svelte';
	import Icon from './Icon.svelte';
</script>

<div class="toasts" aria-live="polite">
	{#each toasts as t (t.id)}
		<div class="toast {t.kind}">
			<Icon name={t.kind === 'bad' ? 'alert' : t.kind === 'ok' ? 'check' : 'activity'} />
			<span>{t.text}</span>
			<button class="btn ghost sm icon" onclick={() => dismiss(t.id)} aria-label="Dismiss"><Icon name="x" size={14} /></button>
		</div>
	{/each}
</div>

<style>
	.toasts {
		position: fixed;
		right: 16px;
		bottom: 16px;
		display: flex;
		flex-direction: column;
		gap: 8px;
		z-index: 60;
		max-width: min(440px, calc(100vw - 32px));
	}
	.toast {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		padding: 10px 10px 10px 14px;
		border-radius: var(--radius);
		background: var(--bg-elev);
		border: 1px solid var(--border);
		box-shadow: var(--shadow-lg);
		font-size: 13px;
		word-break: break-word;
	}
	.toast span {
		flex: 1;
		padding-top: 1px;
	}
	.toast.ok {
		border-left: 3px solid var(--ok);
	}
	.toast.ok :global(svg:first-child) {
		color: var(--ok);
	}
	.toast.bad {
		border-left: 3px solid var(--bad);
	}
	.toast.bad :global(svg:first-child) {
		color: var(--bad);
	}
	.toast.info {
		border-left: 3px solid var(--info);
	}
</style>
