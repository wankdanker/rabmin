<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from './Icon.svelte';
	let { icon = 'box', title, children }: { icon?: IconName; title: string; children?: Snippet } = $props();
</script>

<div class="empty">
	<div class="ic"><Icon name={icon} size={28} stroke={1.4} /></div>
	<div class="t">{title}</div>
	{#if children}<div class="c">{@render children()}</div>{/if}
</div>

<style>
	.ic {
		color: var(--text-3);
		margin-bottom: 8px;
	}
	.t {
		font-weight: 600;
		color: var(--text);
	}
	.c {
		margin-top: 6px;
		font-size: 13px;
	}
</style>
