<script lang="ts">
	import { goto } from '$app/navigation';
	import { refreshClusters } from '$lib/clusters.svelte';
	import { toast } from '$lib/toast.svelte';
	import ClusterForm from '$lib/components/ClusterForm.svelte';
</script>

<svelte:head><title>Add cluster · rabmin</title></svelte:head>

<div class="page" style="max-width: 980px">
	<div class="page-head">
		<div>
			<h1>Add a RabbitMQ cluster</h1>
			<div class="sub">A single node or a cluster, reached through its management API. RabbitMQ 3.13 or later; 4.x is the main target.</div>
		</div>
	</div>
	<ClusterForm
		onsaved={async (c) => {
			await refreshClusters();
			toast(`Added ${c.name}`);
			goto(`/clusters/${c.id}`);
		}}
	/>
</div>
