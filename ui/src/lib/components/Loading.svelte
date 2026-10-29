<script lang="ts">
	let { text = 'Loading…' }: { text?: string } = $props();
</script>

<div class="loading faint"><span class="spin"></span>{text}</div>

<style>
	.loading {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 28px;
		justify-content: center;
	}
</style>
