<script lang="ts">
	import { num } from '$lib/format';
	let { page, pageSize, total, onpage }: { page: number; pageSize: number; total: number; onpage: (p: number) => void } = $props();
	const pages = $derived(Math.max(1, Math.ceil(total / pageSize)));
</script>

{#if total > pageSize}
	<div class="row" style="margin-top:10px">
		<button class="btn sm" disabled={page <= 1} onclick={() => onpage(page - 1)}>Previous</button>
		<span class="faint">{num((page - 1) * pageSize + 1)}–{num(Math.min(page * pageSize, total))} of {num(total)} · page {page} of {pages}</span>
		<button class="btn sm" disabled={page >= pages} onclick={() => onpage(page + 1)}>Next</button>
	</div>
{/if}
