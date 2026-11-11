<script lang="ts">
	import Brick from '#lib/components/Brick.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
</script>

<PageHeader title={t('new.title')} />
<div class="choices">
	<Brick group="when" title={t('new.shelf')} sub={t('new.soon')} />
	<Brick group="look" title={t('new.build')} sub={t('new.soon')} />
</div>

<style>
	.choices {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-2) var(--gutter);
	}
</style>
