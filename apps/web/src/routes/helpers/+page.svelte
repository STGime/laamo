<script lang="ts">
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
</script>

<PageHeader title={t('helpers.title')} />
<p class="note">{t('helpers.empty')}</p>
<div class="cta"><a class="btn primary wide" href="/new">{t('feed.empty.cta')}</a></div>

<style>
	.note {
		margin: 0;
		padding: 0 var(--gutter);
		color: var(--muted);
	}
	.cta {
		padding: var(--space-5) var(--gutter);
	}
</style>
