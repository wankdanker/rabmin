<script lang="ts">
	let {
		value = $bindable(''),
		rows = 18,
		error = $bindable<string | null>(null)
	}: { value?: string; rows?: number; error?: string | null } = $props();

	$effect(() => {
		try {
			if (value.trim()) JSON.parse(value);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		}
	});

	function format() {
		try {
			value = JSON.stringify(JSON.parse(value), null, 2);
		} catch {}
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Tab') {
			e.preventDefault();
			const ta = e.currentTarget as HTMLTextAreaElement;
			const s = ta.selectionStart;
			value = value.slice(0, s) + '  ' + value.slice(ta.selectionEnd);
			requestAnimationFrame(() => (ta.selectionStart = ta.selectionEnd = s + 2));
		}
	}
</script>

<div class="je">
	<textarea bind:value {rows} spellcheck="false" {onkeydown} class:invalid={!!error}></textarea>
	<div class="row bar">
		{#if error}<span class="err">{error}</span>{:else}<span class="faint">Valid JSON</span>{/if}
		<span class="spacer"></span>
		<button class="btn sm" type="button" onclick={format} disabled={!!error}>Format</button>
	</div>
</div>

<style>
	.je textarea {
		min-height: 120px;
		line-height: 1.5;
	}
	.je textarea.invalid {
		border-color: var(--bad);
	}
	.bar {
		margin-top: 6px;
		font-size: 12px;
	}
	.err {
		color: var(--bad);
	}
</style>
