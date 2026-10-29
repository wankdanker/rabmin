<script lang="ts">
	import CopyButton from './CopyButton.svelte';
	let { value, maxHeight = 480 }: { value: unknown; maxHeight?: number } = $props();
	const text = $derived(typeof value === 'string' ? value : JSON.stringify(value, null, 2));

	function highlight(src: string): string {
		const esc = src.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		return esc.replace(
			/("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
			(m) => {
				let cls = 'n';
				if (m.startsWith('"')) cls = m.endsWith(':') ? 'k' : 's';
				else if (m === 'true' || m === 'false') cls = 'b';
				else if (m === 'null') cls = 'z';
				return `<span class="${cls}">${m}</span>`;
			}
		);
	}
</script>

<div class="jv">
	<div class="copy"><CopyButton {text} /></div>
	<!-- eslint-disable-next-line svelte/no-at-html-tags -->
	<pre style="max-height: {maxHeight}px">{@html highlight(text)}</pre>
</div>

<style>
	.jv {
		position: relative;
		background: var(--bg-sunken);
		border: 1px solid var(--border);
		border-radius: var(--radius-sm);
	}
	pre {
		padding: 12px 14px;
		overflow: auto;
	}
	.copy {
		position: absolute;
		top: 6px;
		right: 6px;
	}
	pre :global(.k) {
		color: var(--series-4);
	}
	pre :global(.s) {
		color: var(--ok);
	}
	pre :global(.n) {
		color: var(--series-3);
	}
	pre :global(.b),
	pre :global(.z) {
		color: var(--series-2);
	}
</style>
