<!-- Sign in: one email field, then a magic link. Only the link in an invite email (?invite) makes an account. -->
<script lang="ts">
	import { AuthError, callbackURL } from '#lib/auth.ts';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { page } from '$app/state';
	import { i18n } from '#lib/i18n/index.svelte.ts';
	import { auth } from '#lib/session.svelte.ts';
	import { localPath } from '#lib/site/site.ts';

	// The invite email links here with the address filled in.
	let email = $state(page.url.searchParams.get('email')?.trim() ?? '');
	const invited = page.url.searchParams.has('invite');
	let sent = $state('');
	let busy = $state(false);
	let error = $state('');
	let wait = $state(0);
	let timer: ReturnType<typeof setInterval> | undefined;

	const valid = $derived(/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()));

	async function send(e?: SubmitEvent) {
		e?.preventDefault();
		if (!valid || busy) return;
		busy = true;
		error = '';
		try {
			await auth.sendLink(email, callbackURL(), invited);
			sent = email.trim().toLowerCase();
			wait = 30;
			clearInterval(timer);
			timer = setInterval(() => {
				wait--;
				if (wait <= 0) clearInterval(timer);
			}, 1000);
		} catch (err) {
			error = err instanceof AuthError && err.status === 429 ? t('start.slow') : t('start.error');
		} finally {
			busy = false;
		}
	}

	$effect(() => () => clearInterval(timer));
</script>

<div class="wrap">
	{#if sent}
		<p class="big" aria-hidden="true">✉️</p>
		<h1>{t('start.sent.title')}</h1>
		<p class="body">
			{t(invited ? 'start.sent.body' : 'start.sent.ifAccount', { email: sent })}
		</p>
		<div class="actions">
			<button class="btn wide" disabled={busy || wait > 0} onclick={() => send()}>
				{wait > 0 ? t('start.sent.wait', { n: wait }) : t('start.sent.again')}
			</button>
			<button class="link" onclick={() => ((sent = ''), (wait = 0))}>{t('start.sent.other')}</button
			>
		</div>
		{#if error}<p class="error" role="alert">{error}</p>{/if}
	{:else}
		<h1>{t(invited ? 'start.invited' : 'start.title')}</h1>
		<p class="body">{t('start.body')}</p>
		<form onsubmit={send}>
			<label for="email" class="visually-hidden">{t('start.email')}</label>
			<input
				id="email"
				type="email"
				inputmode="email"
				autocomplete="email"
				placeholder="anna@baeckerei.de"
				bind:value={email}
			/>
			{#if error}<p class="error" role="alert">{error}</p>{/if}
			<button class="btn primary wide" disabled={!valid || busy}>{t('start.send')}</button>
		</form>
		{#if !invited}
			<p class="new">
				{t('start.new')} <a href={localPath('/', i18n.lang)}>{t('start.join')}</a>
			</p>
		{/if}
		{#if import.meta.env.DEV}<a class="dev" href="/signin">Dev: paste a token</a>{/if}
	{/if}
</div>

<style>
	.wrap {
		padding: var(--space-7) var(--gutter);
	}
	.big {
		margin: 0 0 var(--space-3);
		font-size: 44px;
	}
	.body {
		margin: var(--space-3) 0 var(--space-5);
		color: var(--muted);
		font-size: var(--text-lg);
	}
	form,
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
	}
	.link {
		min-height: var(--tap-min);
		border: 0;
		background: none;
		font-weight: 700;
		cursor: pointer;
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-weight: 600;
	}
	.new {
		margin: var(--space-6) 0 0;
		color: var(--muted);
	}
	.new a {
		color: var(--ink);
		font-weight: 700;
	}
	.dev {
		display: inline-block;
		margin-top: var(--space-6);
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
