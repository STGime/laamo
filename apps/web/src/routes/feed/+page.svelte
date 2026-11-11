<script lang="ts">
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';

	type Filter = 'all' | 'needsMe' | 'results';
	let filter = $state<Filter>('all');
	const filters: { id: Filter; key: Key }[] = [
		{ id: 'all', key: 'feed.filter.all' },
		{ id: 'needsMe', key: 'feed.filter.needsMe' },
		{ id: 'results', key: 'feed.filter.results' }
	];
</script>

<PageHeader title={t('feed.title')} />

<div class="pills" role="tablist">
	{#each filters as f (f.id)}
		<button type="button" role="tab" aria-selected={filter === f.id} onclick={() => (filter = f.id)}
			>{t(f.key)}</button
		>
	{/each}
</div>

<!-- Empty state: an example result, not an illustration. The real Feed is #35. -->
<section class="empty">
	<h2>{t('feed.empty.title')}</h2>
	<p>{t('feed.empty.body')}</p>
</section>
<article class="item example" aria-label="Example">
	<p class="meta"><span class="dot"></span>{t('feed.empty.exampleHelper')} · 07:30</p>
	<h3 class="display">{t('feed.empty.exampleTitle')}</h3>
	<p>{t('feed.empty.exampleBody')}</p>
</article>

<div class="cta">
	<a class="btn primary wide" href="/new">{t('feed.empty.cta')}</a>
</div>

<style>
	.pills {
		display: flex;
		gap: var(--space-2);
		padding: 0 var(--gutter) var(--space-3);
		border-bottom: var(--border);
		overflow-x: auto;
	}
	.pills button {
		min-height: 40px;
		padding: 0 var(--space-4);
		border: 0;
		border-radius: 20px;
		background: var(--gray-50);
		font-weight: 700;
		cursor: pointer;
	}
	.pills button[aria-selected='true'] {
		background: var(--ink);
		color: var(--white);
	}
	.empty {
		padding: var(--space-5) var(--gutter) var(--space-3);
	}
	.empty p {
		margin: var(--space-2) 0 0;
		color: var(--muted);
	}
	/* Feed items are edge to edge, split by thin dividers. */
	.item {
		padding: var(--space-4) var(--gutter);
		border-top: var(--border);
		border-bottom: var(--border);
	}
	.item h3 {
		margin: var(--space-1) 0;
		font-size: var(--text-lg);
	}
	.item p {
		margin: 0;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.dot {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		background: var(--brick-yellow);
	}
	.example {
		opacity: 0.75;
	}
	/* One primary action, anchored above the tab bar. */
	.cta {
		position: fixed;
		bottom: calc(var(--tabbar-height) + env(safe-area-inset-bottom));
		left: 50%;
		transform: translateX(-50%);
		width: 100%;
		max-width: var(--column-max);
		padding: var(--space-3) var(--gutter);
		background: var(--white);
	}
</style>
