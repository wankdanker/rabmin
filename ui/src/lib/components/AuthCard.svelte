<script lang="ts">
	import type { Snippet } from 'svelte';
	import Logo from './Logo.svelte';
	import { session } from '$lib/session.svelte';
	let { title, subtitle, children }: { title: string; subtitle?: string; children: Snippet } = $props();
</script>

<div class="wrap">
	<div class="card box">
		<div class="brand"><Logo size={30} /><span>rabmin</span></div>
		<h1>{title}</h1>
		{#if subtitle}<p class="muted">{subtitle}</p>{/if}
		{@render children()}
	</div>
	<div class="faint ver">rabmin {session.version}</div>
</div>

<style>
	.wrap {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 16px;
		background: radial-gradient(1200px 500px at 50% -10%, var(--accent-soft), transparent), var(--bg);
	}
	.box {
		width: 100%;
		max-width: 380px;
		padding: 28px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		font-weight: 700;
		font-size: 18px;
		margin-bottom: 22px;
	}
	h1 {
		font-size: 20px;
	}
	p {
		margin: 4px 0 18px;
	}
	.ver {
		margin-top: 14px;
		font-size: 12px;
	}
</style>
