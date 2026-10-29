<script lang="ts">
	import { guessValue } from '$lib/rabbit';
	import Icon from './Icon.svelte';

	/** Edits a flat object of string keys; values are typed by guessValue (numbers and booleans stay typed). */
	let {
		value = $bindable(),
		keyPlaceholder = 'key',
		valuePlaceholder = 'value',
		suggestions = []
	}: { value: Record<string, unknown>; keyPlaceholder?: string; valuePlaceholder?: string; suggestions?: string[] } = $props();

	let rows = $state<{ k: string; v: string }[]>(Object.entries(value ?? {}).map(([k, v]) => ({ k, v: typeof v === 'string' ? v : JSON.stringify(v) })));
	const listId = 'kv-' + Math.random().toString(36).slice(2);

	$effect(() => {
		const out: Record<string, unknown> = {};
		for (const r of rows) if (r.k.trim()) out[r.k.trim()] = guessValue(r.v);
		value = out;
	});
</script>

<div class="kv">
	{#each rows as r, i}
		<div class="r">
			<input bind:value={r.k} placeholder={keyPlaceholder} list={suggestions.length ? listId : undefined} />
			<input bind:value={r.v} placeholder={valuePlaceholder} />
			<button type="button" class="btn sm ghost icon" aria-label="Remove" onclick={() => rows.splice(i, 1)}><Icon name="x" size={13} /></button>
		</div>
	{/each}
	<button type="button" class="btn sm" onclick={() => rows.push({ k: '', v: '' })}><Icon name="plus" size={13} />Add</button>
	{#if suggestions.length}<datalist id={listId}>{#each suggestions as s}<option value={s}></option>{/each}</datalist>{/if}
</div>

<style>
	.kv {
		display: flex;
		flex-direction: column;
		gap: 6px;
		align-items: flex-start;
	}
	.r {
		display: grid;
		grid-template-columns: 1fr 1fr auto;
		gap: 6px;
		width: 100%;
	}
</style>
