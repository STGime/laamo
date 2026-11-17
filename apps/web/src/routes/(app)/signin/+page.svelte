<!-- Dev sign-in: paste an access token. Real sign-up and sign-in come with onboarding (#32). -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { session } from '#lib/session.svelte.ts';

	let token = $state('');
	let bad = $state(false);
	let busy = $state(false);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		bad = !(await session.useToken(token));
		busy = false;
		if (!bad) goto('/feed', { replaceState: true });
	}
</script>

<div class="wrap">
	<h1>{t('signin.title')}</h1>
	<p class="hint">{t('signin.devHint')}</p>
	<form onsubmit={submit}>
		<label for="token">{t('signin.token')}</label>
		<textarea id="token" rows="4" bind:value={token} autocomplete="off" spellcheck="false"
		></textarea>
		{#if bad}<p class="bad" role="alert">{t('signin.bad')}</p>{/if}
		<button class="btn primary wide" disabled={busy || !token.trim()}>{t('signin.go')}</button>
	</form>
</div>

<style>
	.wrap {
		padding: var(--space-7) var(--gutter);
	}
	.hint {
		color: var(--muted);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	label {
		font-weight: 700;
	}
	textarea {
		padding: var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-sm);
		resize: vertical;
	}
	.bad {
		margin: 0;
		color: var(--danger);
		font-weight: 600;
	}
</style>
