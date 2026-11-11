<script lang="ts">
	import FeedItem from '#lib/components/FeedItem.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { feed, listen, type Filter } from '#lib/feed.svelte.ts';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { session } from '#lib/session.svelte.ts';

	const filters: { id: Filter; key: Key }[] = [
		{ id: 'all', key: 'feed.filter.all' },
		{ id: 'needs_me', key: 'feed.filter.needsMe' },
		{ id: 'results', key: 'feed.filter.results' }
	];

	$effect(() => {
		if (!session.workspace) return;
		feed.load();
		return listen(
			() => feed.load(),
			() => session.token
		);
	});

	const pct = $derived(
		feed.brain && feed.brain.allowance_credits > 0
			? Math.round((feed.brain.credits_left / feed.brain.allowance_credits) * 100)
			: null
	);
</script>

<PageHeader title={t('feed.title')}>
	{#snippet right()}
		{#if feed.brain && pct !== null && feed.brain.mode === 'included'}
			<span
				class="brain"
				title={t('feed.brainTitle', {
					left: Math.floor(feed.brain.credits_left),
					total: Math.round(feed.brain.allowance_credits)
				})}
			>
				<span class="bar" aria-hidden="true"
					><span style:width="{pct}%" class:low={pct < 15}></span></span
				>
				{t('feed.brain', { pct })}
			</span>
		{/if}
	{/snippet}
</PageHeader>

<div class="pills" role="tablist">
	{#each filters as f (f.id)}
		<button
			type="button"
			role="tab"
			aria-selected={feed.filter === f.id}
			onclick={() => feed.load(f.id)}
		>
			{t(f.key)}
			{#if f.id === 'needs_me' && feed.needsMe > 0}<span class="count">{feed.needsMe}</span>{/if}
		</button>
	{/each}
</div>

{#if feed.cards.length}
	{#each feed.cards as card (card.id)}
		<FeedItem {card} />
	{/each}
	{#if feed.hasMore}
		<div class="more">
			<button class="btn wide" disabled={feed.loading} onclick={() => feed.more()}
				>{t('feed.more')}</button
			>
		</div>
	{/if}
{:else if feed.failed}
	<p class="note">
		{feed.failed < 0 ? t('error.offline') : t('feed.loadFailed', { status: feed.failed })}
	</p>
{:else if !feed.loading && feed.filter === 'all'}
	<!-- Empty state: an example result, not an illustration. -->
	<section class="empty">
		<h2>{t('feed.empty.title')}</h2>
		<p>{t('feed.empty.body')}</p>
	</section>
	<article class="example" aria-label="Example">
		<p class="meta"><span aria-hidden="true">📰</span> {t('feed.empty.exampleHelper')} · 07:30</p>
		<h3 class="display">{t('feed.empty.exampleTitle')}</h3>
		<p>{t('feed.empty.exampleBody')}</p>
	</article>
	<div class="cta">
		<a class="btn primary wide" href="/new">{t('feed.empty.cta')}</a>
	</div>
{:else if !feed.loading}
	<p class="note">{t('feed.empty.title')}</p>
{/if}

<style>
	.brain {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		white-space: nowrap;
	}
	.bar {
		width: 48px;
		height: 8px;
		border-radius: 4px;
		background: var(--gray-200);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--brick-purple);
	}
	.bar span.low {
		background: var(--brick-red);
	}
	.pills {
		display: flex;
		gap: var(--space-2);
		padding: 0 var(--gutter) var(--space-3);
		border-bottom: var(--border);
		overflow-x: auto;
	}
	.pills button {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: 40px;
		padding: 0 var(--space-4);
		border: 0;
		border-radius: 20px;
		background: var(--gray-50);
		font-weight: 700;
		white-space: nowrap;
		cursor: pointer;
	}
	.pills button[aria-selected='true'] {
		background: var(--ink);
		color: var(--white);
	}
	.count {
		min-width: 20px;
		height: 20px;
		padding: 0 6px;
		border-radius: 10px;
		background: var(--accent);
		color: var(--on-red);
		font-size: 12px;
		line-height: 20px;
		text-align: center;
	}
	.note {
		padding: var(--space-5) var(--gutter);
		margin: 0;
		color: var(--muted);
	}
	.more {
		padding: var(--space-4) var(--gutter);
	}
	.empty {
		padding: var(--space-5) var(--gutter) var(--space-3);
	}
	.empty p {
		margin: var(--space-2) 0 0;
		color: var(--muted);
	}
	.example {
		padding: var(--space-4) var(--gutter);
		border-top: var(--border);
		border-bottom: var(--border);
		opacity: 0.75;
	}
	.example h3 {
		margin: var(--space-1) 0;
		font-size: var(--text-lg);
	}
	.example p {
		margin: 0;
	}
	.meta {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
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
