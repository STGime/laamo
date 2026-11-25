<!-- Where the browser lands after connecting a social account at the
     network: Posta sends it back with success=true|false in the address. -->
<script lang="ts">
	import { page } from '$app/state';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { connectResult } from '#lib/social.ts';

	const r = $derived(connectResult(page.url.searchParams));
	const name = $derived(r.name || t('social.done.account'));
</script>

<PageHeader title={t('social.done.title')} />

<div class="box" class:failed={!r.ok} role="status">
	{#if r.ok}
		<p><strong>{t('social.done.ok', { name })}</strong></p>
		{#if r.connected > 0}
			<p>
				{t(r.connected === 1 ? 'social.done.pages.one' : 'social.done.pages', { n: r.connected })}
				{#if r.skipped > 0}{t('social.done.skipped', { n: r.skipped })}{/if}
				{#if r.failed > 0}{t('social.done.failedPages', { n: r.failed })}{/if}
			</p>
		{/if}
		<p class="hint">{t('social.done.okHint')}</p>
	{:else}
		<p><strong>{t('social.done.failed', { name })}</strong></p>
		<p class="hint">{t('social.done.failedHint')}</p>
	{/if}
</div>
<div class="cta">
	<a class="btn primary wide" href="/places/social">{t('social.done.back')}</a>
</div>

<style>
	.box {
		margin: var(--space-3) var(--gutter);
		padding: var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
	}
	.box.failed {
		border-color: var(--danger);
	}
	.box p {
		margin: 0;
	}
	.hint {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.cta {
		padding: var(--space-3) var(--gutter);
	}
</style>
