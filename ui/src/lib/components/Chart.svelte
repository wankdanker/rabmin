<script lang="ts">
	type Series = { name: string; values: number[]; color: string };
	let {
		times,
		series,
		height = 200,
		format = (n: number) => n.toFixed(0)
	}: { times: number[]; series: Series[]; height?: number; format?: (n: number) => string } = $props();

	let width = $state(600);
	let hover = $state<number | null>(null);
	const pad = { l: 52, r: 12, t: 10, b: 24 };

	const geom = $derived.by(() => {
		const n = times.length;
		const w = Math.max(width - pad.l - pad.r, 10);
		const h = height - pad.t - pad.b;
		let max = 0;
		for (const s of series) for (const v of s.values) max = Math.max(max, v);
		max = niceMax(max);
		const t0 = times[0] ?? 0;
		const t1 = times[n - 1] ?? 1;
		const x = (t: number) => pad.l + (t1 === t0 ? w : ((t - t0) / (t1 - t0)) * w);
		const y = (v: number) => pad.t + h - (max ? (v / max) * h : 0);
		const paths = series.map((s) => {
			if (n < 2) return { line: '', area: '' };
			const pts = s.values.map((v, i) => `${x(times[i]).toFixed(1)},${y(v).toFixed(1)}`);
			const line = 'M' + pts.join('L');
			return { line, area: `${line}L${x(t1).toFixed(1)},${pad.t + h}L${x(t0).toFixed(1)},${pad.t + h}Z` };
		});
		const yticks = [0, 0.25, 0.5, 0.75, 1].map((f) => ({ v: max * f, y: y(max * f) }));
		const xticks: { t: number; x: number }[] = [];
		if (n > 1) {
			const count = Math.max(2, Math.floor(w / 110));
			for (let i = 0; i <= count; i++) {
				const t = t0 + ((t1 - t0) * i) / count;
				xticks.push({ t, x: x(t) });
			}
		}
		return { x, y, w, h, paths, yticks, xticks, t0, t1 };
	});

	function niceMax(v: number) {
		if (v <= 0) return 1;
		const e = Math.pow(10, Math.floor(Math.log10(v)));
		for (const m of [1, 2, 2.5, 5, 10]) if (m * e >= v) return m * e;
		return 10 * e;
	}

	function onmove(e: PointerEvent) {
		const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
		const px = e.clientX - rect.left;
		if (times.length < 2) return;
		const f = (px - pad.l) / geom.w;
		const t = geom.t0 + f * (geom.t1 - geom.t0);
		let best = 0;
		for (let i = 1; i < times.length; i++) if (Math.abs(times[i] - t) < Math.abs(times[best] - t)) best = i;
		hover = best;
	}

	const fmtTime = (t: number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
</script>

<div class="chart" bind:clientWidth={width}>
	{#if times.length < 2}
		<div class="wait faint" style="height:{height}px">Collecting samples…</div>
	{:else}
		<svg {width} {height} role="img" aria-label="Chart" onpointermove={onmove} onpointerleave={() => (hover = null)}>
			{#each geom.yticks as t}
				<line x1={pad.l} x2={width - pad.r} y1={t.y} y2={t.y} class="grid" />
				<text x={pad.l - 6} y={t.y + 4} text-anchor="end" class="tick">{format(t.v)}</text>
			{/each}
			{#each geom.xticks as t}
				<text x={t.x} y={height - 6} text-anchor="middle" class="tick">{fmtTime(t.t)}</text>
			{/each}
			{#each series as s, i}
				<path d={geom.paths[i].area} fill={s.color} opacity="0.1" />
				<path d={geom.paths[i].line} fill="none" stroke={s.color} stroke-width="1.6" />
			{/each}
			{#if hover !== null}
				{@const hx = geom.x(times[hover])}
				<line x1={hx} x2={hx} y1={pad.t} y2={pad.t + geom.h} class="cross" />
				{#each series as s}
					<circle cx={hx} cy={geom.y(s.values[hover] ?? 0)} r="3.5" fill={s.color} />
				{/each}
			{/if}
		</svg>
		{#if hover !== null}
			{@const hx = geom.x(times[hover])}
			<div class="tip card" style="left: {Math.min(hx + 12, width - 180)}px">
				<div class="faint">{new Date(times[hover]).toLocaleTimeString()}</div>
				{#each series as s}
					<div class="tip-row"><span class="sw" style="background:{s.color}"></span>{s.name}<b class="num">{format(s.values[hover] ?? 0)}</b></div>
				{/each}
			</div>
		{/if}
	{/if}
	<div class="legend">
		{#each series as s}
			<span><span class="sw" style="background:{s.color}"></span>{s.name}</span>
		{/each}
	</div>
</div>

<style>
	.chart {
		position: relative;
		width: 100%;
	}
	svg {
		display: block;
		touch-action: none;
	}
	.grid {
		stroke: var(--border);
		stroke-width: 1;
	}
	.cross {
		stroke: var(--text-3);
		stroke-dasharray: 3 3;
	}
	.tick {
		fill: var(--text-3);
		font-size: 11px;
		font-variant-numeric: tabular-nums;
	}
	.wait {
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.tip {
		position: absolute;
		top: 8px;
		padding: 8px 10px;
		font-size: 12px;
		pointer-events: none;
		min-width: 160px;
		box-shadow: var(--shadow-lg);
	}
	.tip-row {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.tip-row b {
		margin-left: auto;
		padding-left: 12px;
		font-weight: 600;
	}
	.legend {
		display: flex;
		gap: 14px;
		font-size: 12px;
		color: var(--text-2);
		padding-left: 52px;
		flex-wrap: wrap;
	}
	.legend > span {
		display: inline-flex;
		align-items: center;
		gap: 6px;
	}
	.sw {
		width: 10px;
		height: 3px;
		border-radius: 2px;
		display: inline-block;
	}
</style>
