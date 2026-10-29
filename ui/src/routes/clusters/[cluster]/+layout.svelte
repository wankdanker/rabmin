<script lang="ts">
	import { page } from '$app/state';
	import ClusterScope from '$lib/components/ClusterScope.svelte';

	let { children } = $props();
</script>

{#key page.params.cluster}
	<ClusterScope id={page.params.cluster!}>{@render children()}</ClusterScope>
{/key}
