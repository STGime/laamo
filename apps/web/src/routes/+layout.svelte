<script lang="ts">
	import '#lib/styles/tokens.css';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import TabBar from '#lib/components/TabBar.svelte';
	import { api, session } from '#lib/session.svelte.ts';
	import { takePending } from '#lib/demo.ts';
	import { feed } from '#lib/feed.svelte.ts';
	import { t } from '#lib/i18n/index.svelte.ts';

	let { children } = $props();

	// Screens that work signed out (and the dev ones).
	const OPEN = ['/', '/try', '/start', '/auth/callback', '/signin', '/kit'];
	// Screens without the tab bar.
	const BARE = ['/', '/try', '/start', '/auth/callback', '/signin', '/welcome'];
	const path = $derived(page.url.pathname);
	// A shared list is public too.
	const shared = $derived(path.startsWith('/shared/'));
	const open = $derived(OPEN.includes(path) || shared);

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
	// The helper tried before sign-up: make it now, with the same inputs,
	// and run it once so its first result is in the Feed.
	$effect(() => {
		const ws = session.workspace;
		if (session.status !== 'signedIn' || !ws) return;
		const p = takePending();
		if (!p) return;
		const answers: Record<string, string> = { city: p.city, chat: 'feed' };
		if (p.business) answers.business = p.business;
		api.POST('/workspaces/{workspaceId}/helpers', {
			params: { path: { workspaceId: ws.id } },
			body: { template: p.template, answers, run_test: true }
		});
	});
	const showTabs = $derived(session.status === 'signedIn' && !BARE.includes(path) && !shared);
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
