<script lang="ts">
	let { values, color = 'var(--series-1)', height = 28, width = 120 }: { values: number[]; color?: string; height?: number; width?: number } =
		$props();
	const d = $derived.by(() => {
		if (values.length < 2) return { line: '', area: '' };
		const max = Math.max(...values, 1e-9);
		const step = width / (values.length - 1);
		const pts = values.map((v, i) => [i * step, height - 2 - (v / max) * (height - 4)]);
		const line = 'M' + pts.map((p) => p.map((n) => n.toFixed(1)).join(',')).join('L');
		return { line, area: `${line}L${width},${height}L0,${height}Z` };
	});
</script>

<svg {width} {height} viewBox="0 0 {width} {height}" preserveAspectRatio="none" aria-hidden="true">
	{#if d.line}
		<path d={d.area} fill={color} opacity="0.12" />
		<path d={d.line} fill="none" stroke={color} stroke-width="1.5" vector-effect="non-scaling-stroke" />
	{/if}
</svg>
