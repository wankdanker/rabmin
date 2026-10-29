<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import type { Sorter } from '$lib/sort.svelte';
	let {
		sorter,
		key,
		right = false,
		desc = false,
		children
	}: { sorter: Sorter<T>; key: string; right?: boolean; desc?: boolean; children: Snippet } = $props();
</script>

<th class="sortable" class:r={right} onclick={() => sorter.toggle(key, desc ? -1 : 1)}>
	{@render children()}
	{#if sorter.key === key}<span class="arrow">{sorter.dir === 1 ? '▲' : '▼'}</span>{/if}
</th>

<style>
	.arrow {
		font-size: 9px;
		margin-left: 4px;
		color: var(--accent);
	}
</style>
