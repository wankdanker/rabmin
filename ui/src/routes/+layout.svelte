<script lang="ts">
	import '../app.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, unwrap, setUnauthorizedHandler } from '$lib/api/client';
	import { session } from '$lib/session.svelte';
	import { refreshClusters } from '$lib/clusters.svelte';
	import Shell from '$lib/components/Shell.svelte';
	import Toasts from '$lib/components/Toasts.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Loading from '$lib/components/Loading.svelte';

	let { children } = $props();
	let ready = $state(false);

	const isAuthPage = $derived(page.url.pathname === '/login' || page.url.pathname === '/setup');

	setUnauthorizedHandler(() => {
		session.me = null;
		if (!isAuthPage) goto('/login?next=' + encodeURIComponent(page.url.pathname + page.url.search));
	});

	async function init() {
		try {
			const status = await unwrap(api.GET('/v1/status'));
			session.version = status.version;
			if (status.needsSetup) {
				if (page.url.pathname !== '/setup') await goto('/setup');
				return;
			}
			if (!status.authenticated) {
				if (!isAuthPage) await goto('/login?next=' + encodeURIComponent(page.url.pathname + page.url.search));
				return;
			}
			session.me = await unwrap(api.GET('/v1/auth/me'));
			await refreshClusters();
			if (isAuthPage) await goto('/');
		} catch {
			// Shown by the page that needs it.
		} finally {
			ready = true;
		}
	}

	init();
</script>

{#if !ready}
	<Loading />
{:else if isAuthPage}
	{@render children()}
{:else if session.me}
	<Shell>{@render children()}</Shell>
{:else}
	<Loading />
{/if}
<Toasts />
<Confirm />
