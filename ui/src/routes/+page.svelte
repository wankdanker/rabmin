<script lang="ts">
	import { session } from '$lib/session.svelte';
	import Icon from '$lib/components/Icon.svelte';
	import Empty from '$lib/components/Empty.svelte';
</script>

<svelte:head><title>Overview · rabmin</title></svelte:head>

<div class="page">
	<div class="page-head">
		<div>
			<h1>Overview</h1>
			<div class="sub">rabmin {session.version}</div>
		</div>
	</div>
	<div class="card">
		<Empty icon="topology" title="No clusters yet">
			The cluster registry and monitoring are not built yet. Until then, manage rabmin users and API tokens, or explore the API.
			<div style="margin-top:14px"><a href="/api/docs" class="btn" target="_blank" rel="noopener"><Icon name="book" />API docs</a></div>
		</Empty>
	</div>
</div>
