<script lang="ts">
	import { goto } from '$app/navigation';
	import { navigating, page } from '$app/state';
	import { busy } from '#lib/busy.svelte.ts';
	import NotInvited from '#lib/components/NotInvited.svelte';
	import TabBar from '#lib/components/TabBar.svelte';
	import { api, session } from '#lib/session.svelte.ts';
	import { takePending } from '#lib/demo.ts';
	import { feed, listen } from '#lib/feed.svelte.ts';
	import { onOpen, push } from '#lib/push.svelte.ts';
	import { t } from '#lib/i18n/index.svelte.ts';

	let { children } = $props();

	// Screens that work signed out (and the dev ones). The public site
	// (routes/(site)) has its own layout.
	const OPEN = ['/start', '/auth/callback', '/signin', '/kit'];
	// Screens without the tab bar.
	const BARE = ['/start', '/auth/callback', '/signin', '/welcome'];
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
		else if (hasWorkspace && path === '/start') goto('/feed', { replaceState: true });
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
	// Notifications: see what this device can do, and keep its subscription
	// with the person signed in. A tapped notification opens its page.
	$effect(() => {
		if (session.status === 'signedIn' && session.workspace) push.check();
	});
	$effect(() => onOpen((path) => goto(path)));
	// The dot on the Feed tab: something new came in while another screen
	// was open. On the Feed itself everything counts as seen.
	const onFeed = $derived(path === '/feed');
	$effect(() => {
		if (session.status !== 'signedIn' || !session.workspace) return;
		const look = () => (onFeed ? feed.seen() : feed.peek());
		look();
		return listen(look, () => session.fresh());
	});
	// Something is loading: a page being opened, or data on its way. Shown
	// after a short moment only, so quick answers don't flicker.
	const waiting = $derived(!!navigating.to || busy.pending > 0);
	let slow = $state(false);
	$effect(() => {
		if (!waiting) {
			slow = false;
			return;
		}
		const timer = setTimeout(() => (slow = true), 250);
		return () => clearTimeout(timer);
	});
	const showTabs = $derived(session.status === 'signedIn' && !BARE.includes(path) && !shared);
</script>

<svelte:head>
	<title>Laamo</title>
	<!-- The app is not for search engines; the public site is. -->
	<meta name="robots" content="noindex" />
</svelte:head>

{#if slow}
	<div class="loadbar" role="status">
		<span class="visually-hidden">{t('loading')}</span>
	</div>
{/if}
<div class="app" class:tabs={showTabs}>
	{#if session.status === 'offline' && !open}
		<p class="offline">{t('error.offline')}</p>
	{/if}
	{#if session.status === 'signedIn' || open}
		{@render children()}
	{:else if session.status === 'notInvited'}
		<NotInvited />
	{:else if session.status === 'loading'}
		<p class="loading">{t('loading')}</p>
	{/if}
</div>
{#if showTabs}<TabBar needsMe={feed.needsMe} fresh={feed.unseen} />{/if}

<style>
	.app {
		max-width: var(--column-max);
		min-height: 100dvh;
		margin: 0 auto;
		background: var(--white);
	}
	/* A bar that runs along the top while something loads. */
	.loadbar {
		position: fixed;
		top: 0;
		left: 50%;
		z-index: 60;
		width: 100%;
		max-width: var(--column-max);
		height: 4px;
		overflow: hidden;
		transform: translateX(-50%);
		background: var(--gray-50);
	}
	.loadbar::before {
		content: '';
		position: absolute;
		inset: 0;
		width: 40%;
		background: var(--brick-red);
		animation: run 1s ease-in-out infinite;
	}
	@keyframes run {
		from {
			transform: translateX(-100%);
		}
		to {
			transform: translateX(250%);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.loadbar::before {
			width: 100%;
			animation: none;
		}
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
