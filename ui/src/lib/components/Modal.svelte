<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		open = $bindable(false),
		title,
		width = 560,
		children,
		footer
	}: { open?: boolean; title: string; width?: number; children: Snippet; footer?: Snippet } = $props();

	function onkey(e: KeyboardEvent) {
		if (open && e.key === 'Escape') open = false;
	}
</script>

<svelte:window onkeydown={onkey} />

{#if open}
	<div class="backdrop" role="presentation" onmousedown={(e) => e.target === e.currentTarget && (open = false)}>
		<div class="modal card" style="max-width: {width}px" role="dialog" aria-modal="true" aria-label={title}>
			<div class="card-head">
				<h2>{title}</h2>
				<span class="spacer"></span>
				<button class="btn ghost icon" onclick={() => (open = false)} aria-label="Close"><Icon name="x" /></button>
			</div>
			<div class="body">{@render children()}</div>
			{#if footer}
				<div class="foot">{@render footer()}</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgb(0 0 0 / 45%);
		display: flex;
		align-items: flex-start;
		justify-content: center;
		padding: 8vh 16px 16px;
		z-index: 50;
		overflow-y: auto;
	}
	.modal {
		width: 100%;
		box-shadow: var(--shadow-lg);
	}
	.body {
		padding: 16px;
	}
	.foot {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		padding: 12px 16px;
		border-top: 1px solid var(--border);
	}
</style>
