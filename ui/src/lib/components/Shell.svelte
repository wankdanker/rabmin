<script lang="ts">
	import type { Snippet } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { session, can } from '$lib/session.svelte';
	import Icon from './Icon.svelte';
	import Logo from './Logo.svelte';

	let { children }: { children: Snippet } = $props();
	let mobileOpen = $state(false);
	let theme = $state<'light' | 'dark' | 'auto'>(readTheme());

	function readTheme(): 'light' | 'dark' | 'auto' {
		try {
			const t = localStorage.getItem('rabmin.theme');
			return t === 'light' || t === 'dark' ? t : 'auto';
		} catch {
			return 'auto';
		}
	}
	function cycleTheme() {
		theme = theme === 'auto' ? 'dark' : theme === 'dark' ? 'light' : 'auto';
		try {
			if (theme === 'auto') {
				localStorage.removeItem('rabmin.theme');
				delete document.documentElement.dataset.theme;
			} else {
				localStorage.setItem('rabmin.theme', theme);
				document.documentElement.dataset.theme = theme;
			}
		} catch {}
	}

	async function logout() {
		await api.POST('/v1/auth/logout');
		session.me = null;
		goto('/login');
	}

	const path = $derived(page.url.pathname);
	function active(href: string, exact = false) {
		return exact ? path === href : path === href || path.startsWith(href + '/');
	}

	$effect(() => {
		path;
		mobileOpen = false;
	});

</script>

<div class="shell">
	<header class="mobile-bar">
		<button class="btn ghost icon" onclick={() => (mobileOpen = !mobileOpen)} aria-label="Menu"><Icon name="menu" /></button>
		<a href="/" class="brand"><Logo size={22} />rabmin</a>
	</header>

	<aside class:open={mobileOpen}>
		<a href="/" class="brand top"><Logo size={24} /><span>rabmin</span></a>

		<nav>
			<a href="/" class="item" class:active={path === '/'}><Icon name="globe" />Overview</a>

			<div class="section"><span>rabmin</span></div>
			{#if can('admin')}
				<a href="/settings/users" class="item" class:active={active('/settings/users')}><Icon name="users" />Users</a>
			{/if}
			<a href="/settings/tokens" class="item" class:active={active('/settings/tokens')}><Icon name="key" />API tokens</a>
			{#if can('admin')}
				<a href="/settings/audit" class="item" class:active={active('/settings/audit')}><Icon name="log" />Audit log</a>
			{/if}
			<a href="/api/docs" class="item" target="_blank" rel="noopener"><Icon name="book" />API docs<Icon name="external" size={12} /></a>
		</nav>

		<div class="foot">
			<a href="/settings/profile" class="user" title="Your profile">
				<span class="avatar">{session.me?.user.username.slice(0, 1).toUpperCase()}</span>
				<span class="who">
					<span class="ellipsis">{session.me?.user.username}</span>
					<span class="faint role">{session.me?.role}</span>
				</span>
			</a>
			<button class="btn ghost icon" onclick={cycleTheme} title="Theme: {theme}" aria-label="Toggle theme">
				<Icon name={theme === 'light' ? 'sun' : theme === 'dark' ? 'moon' : 'eye'} />
			</button>
			<button class="btn ghost icon" onclick={logout} title="Log out" aria-label="Log out"><Icon name="logout" /></button>
		</div>
	</aside>
	{#if mobileOpen}<button class="scrim" aria-label="Close menu" onclick={() => (mobileOpen = false)}></button>{/if}

	<main>{@render children()}</main>
</div>

<style>
	.shell {
		display: flex;
		min-height: 100vh;
	}
	aside {
		width: var(--sidebar-w);
		flex: none;
		background: var(--bg-elev);
		border-right: 1px solid var(--border);
		display: flex;
		flex-direction: column;
		position: sticky;
		top: 0;
		height: 100vh;
	}
	main {
		flex: 1;
		min-width: 0;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 9px;
		font-weight: 700;
		font-size: 16px;
		color: var(--text);
		text-decoration: none;
	}
	.brand.top {
		padding: 16px 16px 12px;
	}
	nav {
		flex: 1;
		overflow-y: auto;
		padding: 4px 8px 12px;
	}
	.section {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 11px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--text-3);
		padding: 16px 8px 6px;
	}
	.add {
		color: var(--text-3);
		display: flex;
	}
	.add:hover {
		color: var(--accent);
	}
	.item {
		display: flex;
		align-items: center;
		gap: 9px;
		padding: 6px 8px;
		border-radius: var(--radius-sm);
		color: var(--text-2);
		font-weight: 500;
		text-decoration: none;
		min-width: 0;
	}
	.item:hover {
		background: var(--bg-hover);
		color: var(--text);
		text-decoration: none;
	}
	.item.active {
		background: var(--accent-soft);
		color: var(--accent);
	}
	.item.sys .ellipsis {
		flex: 1;
	}
	.item.sys.current {
		color: var(--text);
	}
	.sub {
		margin: 2px 0 6px 12px;
		padding-left: 8px;
		border-left: 1px solid var(--border);
	}
	.sub .item {
		font-size: 13px;
		padding: 5px 8px;
	}
	.acc-label {
		font-size: 11px;
		color: var(--text-3);
		padding: 10px 8px 3px;
		font-weight: 600;
	}
	.none {
		padding: 4px 8px;
		font-size: 13px;
	}
	.foot {
		border-top: 1px solid var(--border);
		padding: 10px;
		display: flex;
		align-items: center;
		gap: 4px;
	}
	.user {
		display: flex;
		align-items: center;
		gap: 8px;
		flex: 1;
		min-width: 0;
		color: var(--text);
		text-decoration: none;
		padding: 4px;
		border-radius: var(--radius-sm);
	}
	.user:hover {
		background: var(--bg-hover);
		text-decoration: none;
	}
	.avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--accent-soft);
		color: var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-weight: 700;
		flex: none;
	}
	.who {
		display: flex;
		flex-direction: column;
		min-width: 0;
		line-height: 1.2;
	}
	.role {
		font-size: 11.5px;
	}
	.mobile-bar {
		display: none;
	}
	.scrim {
		display: none;
	}
	@media (max-width: 900px) {
		.shell {
			flex-direction: column;
		}
		.mobile-bar {
			display: flex;
			align-items: center;
			gap: 8px;
			padding: 8px 12px;
			background: var(--bg-elev);
			border-bottom: 1px solid var(--border);
			position: sticky;
			top: 0;
			z-index: 20;
		}
		aside {
			position: fixed;
			left: 0;
			top: 0;
			z-index: 40;
			transform: translateX(-100%);
			transition: transform 0.18s ease;
			box-shadow: var(--shadow-lg);
		}
		aside.open {
			transform: none;
		}
		.scrim {
			display: block;
			position: fixed;
			inset: 0;
			background: rgb(0 0 0 / 35%);
			border: none;
			z-index: 30;
		}
	}
</style>
