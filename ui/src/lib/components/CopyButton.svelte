<script lang="ts">
	import Icon from './Icon.svelte';
	let { text, label = '' }: { text: string; label?: string } = $props();
	let done = $state(false);
	async function copy() {
		try {
			await navigator.clipboard.writeText(text);
			done = true;
			setTimeout(() => (done = false), 1200);
		} catch {}
	}
</script>

<button class="btn sm {label ? '' : 'ghost icon'}" onclick={copy} title="Copy" aria-label="Copy">
	<Icon name={done ? 'check' : 'copy'} size={14} />{label}
</button>
