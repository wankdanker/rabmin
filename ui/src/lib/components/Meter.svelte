<script lang="ts">
	import { pct } from '$lib/format';
	let { used, total, label }: { used: number; total: number; label?: string } = $props();
	const p = $derived(pct(used, total));
	const tone = $derived(p > 90 ? 'bad' : p > 75 ? 'warn' : 'ok');
</script>

<div class="meter" title={label ?? `${p.toFixed(1)}%`}>
	<div class="fill {tone}" style="width: {total > 0 ? p : 0}%"></div>
</div>

<style>
	.meter {
		height: 6px;
		border-radius: 3px;
		background: var(--bg-sunken);
		overflow: hidden;
		min-width: 60px;
		border: 1px solid var(--border);
	}
	.fill {
		height: 100%;
		background: var(--accent);
	}
	.fill.warn {
		background: var(--warn);
	}
	.fill.bad {
		background: var(--bad);
	}
</style>
