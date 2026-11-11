<script lang="ts">
	import '#lib/styles/tokens.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import TabBar from '#lib/components/TabBar.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { feed } from '#lib/feed.svelte.ts';
	import { t } from '#lib/i18n/index.svelte.ts';

	let { children } = $props();

	// Screens that work signed out (and the dev ones).
	const OPEN = ['/', '/start', '/auth/callback', '/signin', '/kit'];
	// Screens without the tab bar.
	const BARE = ['/', '/start', '/auth/callback', '/signin', '/welcome'];
	const path = $derived(page.url.pathname);
	const open = $derived(OPEN.includes(path));

	$effect(() => {
		session.refresh();
	});
	$effect(() => {
		const s = session.status;
		if (s === 'signedOut' && !open) goto('/', { replaceState: true });
		if (s !== 'signedIn' || path === '/auth/callback') return;
		const hasWorkspace = (session.me?.workspaces.length ?? 0) > 0;
		if (!hasWorkspace && path !== '/welcome') goto('/welcome', { replaceState: true });
		else if (hasWorkspace && (path === '/' || path === '/start'))
			goto('/feed', { replaceState: true });
	});
	const showTabs = $derived(session.status === 'signedIn' && !BARE.includes(path));
</script>

<div class="app" class:tabs={showTabs}>
	{#if session.status === 'offline' && !open}
		<p class="offline">{t('error.offline')}</p>
	{/if}
	{#if session.status === 'signedIn' || open}
		{@render children()}
	{:else if session.status === 'loading'}
		<p class="loading">{t('loading')}</p>
	{/if}
</div>
{#if showTabs}<TabBar needsMe={feed.needsMe} />{/if}

<style>
	.app {
		max-width: var(--column-max);
		min-height: 100dvh;
		margin: 0 auto;
		background: var(--white);
	}
	.app.tabs {
		padding-bottom: calc(var(--tabbar-height) + env(safe-area-inset-bottom));
	}
	.offline,
	.loading {
		margin: 0;
		padding: var(--space-4) var(--gutter);
		color: var(--muted);
	}
	.offline {
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-weight: 600;
	}
</style>
