<script lang="ts">
	import Sparkline from './Sparkline.svelte';
	let {
		label,
		value,
		sub,
		spark,
		tone
	}: { label: string; value: string; sub?: string; spark?: number[]; tone?: 'ok' | 'warn' | 'bad' } = $props();
</script>

<div class="card stat">
	<div class="label">{label}</div>
	<div class="value num {tone ?? ''}">{value}</div>
	{#if sub}<div class="sub faint">{sub}</div>{/if}
	{#if spark && spark.length > 1}
		<div class="spark"><Sparkline values={spark} width={160} height={26} /></div>
	{/if}
</div>

<style>
	.stat {
		padding: 12px 14px;
		position: relative;
		overflow: hidden;
		min-height: 84px;
	}
	.label {
		font-size: 12px;
		color: var(--text-2);
	}
	.value {
		font-size: 22px;
		font-weight: 600;
		margin-top: 2px;
		letter-spacing: -0.02em;
	}
	.value.ok {
		color: var(--ok);
	}
	.value.warn {
		color: var(--warn);
	}
	.value.bad {
		color: var(--bad);
	}
	.sub {
		font-size: 12px;
	}
	.spark {
		position: absolute;
		right: 0;
		bottom: 0;
		left: 0;
		opacity: 0.55;
		pointer-events: none;
		display: flex;
		justify-content: flex-end;
	}
	.spark :global(svg) {
		width: 100%;
	}
</style>
