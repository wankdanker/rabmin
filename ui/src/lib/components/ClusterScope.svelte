<script lang="ts">
	import type { Snippet } from 'svelte';
	import { untrack } from 'svelte';
	import { LiveCluster } from '$lib/live.svelte';
	import { setLive } from '$lib/context';
	import { clusters, findCluster } from '$lib/clusters.svelte';
	import Empty from './Empty.svelte';

	let { id, children }: { id: string; children: Snippet } = $props();

	const live = new LiveCluster(untrack(() => id));
	setLive(live);
	const cluster = $derived(findCluster(live.id));

	$effect(() => {
		live.start();
		return () => live.close();
	});
</script>

{#if !cluster && clusters.loaded}
	<div class="page">
		<div class="card"><Empty icon="alert" title="Cluster not found">There is no cluster with id <code>{live.id}</code>.</Empty></div>
	</div>
{:else}
	{@render children()}
{/if}
