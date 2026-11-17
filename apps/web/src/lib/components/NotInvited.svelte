<!-- Signed in, but not invited from the waiting list yet (the app is invite-only). -->
<script lang="ts">
	import { i18n, t } from '#lib/i18n/index.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import { localPath } from '#lib/site/site.ts';
</script>

<div class="wrap">
	<h1>{t('invite.title')}</h1>
	<p class="body">{t('invite.body')}</p>
	<div class="actions">
		<a class="btn primary wide" href={localPath('/', i18n.lang)}>{t('invite.join')}</a>
		<button class="link" onclick={() => session.signOut()}>{t('invite.other')}</button>
	</div>
</div>

<style>
	.wrap {
		padding: var(--space-7) var(--gutter);
	}
	.body {
		margin: var(--space-3) 0 var(--space-5);
		color: var(--muted);
		font-size: var(--text-lg);
	}
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.link {
		min-height: var(--tap-min);
		border: 0;
		background: none;
		font-weight: 700;
		cursor: pointer;
	}
</style>
