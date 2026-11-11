<!-- The magic link lands here: exchange its token for a session, then go to the Feed (or onboarding). -->
<script lang="ts">
	import { goto, replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { auth, session } from '#lib/session.svelte.ts';

	let failed = $state(false);
	let started = false;

	$effect(() => {
		if (started) return;
		started = true;
		const token = page.url.searchParams.get('token');
		// Drop the token from the address bar and history right away.
		replaceState('/auth/callback', {});
		if (!token) {
			failed = true;
			return;
		}
		auth
			.signIn(token)
			.then((tokens) => session.useTokens(tokens))
			.then((ok) => {
				if (!ok) throw new Error('session');
				goto(session.me?.workspaces.length ? '/feed' : '/welcome', { replaceState: true });
			})
			.catch(() => (failed = true));
	});
</script>

<div class="wrap">
	{#if failed}
		<h1>{t('callback.failed')}</h1>
		<a class="btn primary wide" href="/start">{t('callback.again')}</a>
	{:else}
		<p class="working">{t('callback.working')}</p>
	{/if}
</div>

<style>
	.wrap {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		padding: var(--space-7) var(--gutter);
	}
	h1 {
		font-size: var(--text-h2);
	}
	.working {
		color: var(--muted);
		font-size: var(--text-lg);
	}
</style>
