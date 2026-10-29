<script lang="ts">
	import { onMount } from 'svelte';
	import { loadVHosts, vhostsOf } from '$lib/vhosts.svelte';

	let {
		cluster,
		value = $bindable(),
		all = false,
		onchange
	}: { cluster: string; value: string; all?: boolean; onchange?: () => void } = $props();

	onMount(() => {
		loadVHosts(cluster).catch(() => {});
	});
	const list = $derived(vhostsOf(cluster));
</script>

<select bind:value onchange={() => onchange?.()}>
	{#if all}<option value="">All virtual hosts</option>{/if}
	{#each list as v (v.name)}<option value={v.name}>{v.name}</option>{/each}
	{#if !all && value && !list.some((v) => v.name === value)}<option value={value}>{value}</option>{/if}
</select>
