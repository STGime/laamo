<script lang="ts">
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
</script>

<PageHeader title={t('lists.title')} />
<p class="note">{t('lists.empty')}</p>

<style>
	.note {
		margin: 0;
		padding: 0 var(--gutter);
		color: var(--muted);
	}
</style>
