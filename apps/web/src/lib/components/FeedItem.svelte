<!--
  One Feed item, edge to edge. Result and Problem cards inform (hide with ✕);
  Check first, Let me pick and Ask me something wait for an answer.
  Check first also swipes: right approves all, left skips all.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { feed, type FeedCard } from '#lib/feed.svelte.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api } from '#lib/session.svelte.ts';
	import { hostOf, safeURL } from '#lib/links.ts';
	import { ago, when } from '#lib/time.ts';
	import BottomSheet from './BottomSheet.svelte';
	import CardView from './CardView.svelte';
	import SwipeStack from './SwipeStack.svelte';

	let { card }: { card: FeedCard } = $props();

	const isOpen = $derived(card.state === 'open');
	const asks = $derived(['approval', 'pick', 'question'].includes(card.type));
	const count = $derived(Number(card.payload.cards ?? 0));
	const group = $derived(
		card.type === 'problem' ? 'problem' : asks ? 'ask' : card.payload.test ? 'test' : 'result'
	);

	let busy = $state(false);
	let failed = $state(false);
	let sheet = $state(false);
	let pile = $state<Schemas['Card'][]>([]);

	// Check first: per card, skip or keep, and the user's edits.
	let skip = $state<Record<string, boolean>>({});
	let edits = $state<Record<string, Record<string, string>>>({});
	let editing = $state<string | null>(null);
	// Let me pick
	let picked = $state<string[]>([]);
	// Ask me something
	let answer = $state('');

	async function openPile() {
		busy = true;
		const { data } = await api.GET('/feed-cards/{feedCardId}', {
			params: { path: { feedCardId: card.id } }
		});
		busy = false;
		if (!data) return (failed = true);
		pile = data.cards;
		skip = {};
		edits = {};
		picked = [];
		sheet = true;
	}

	async function decide(body: Schemas['FeedDecision']) {
		busy = true;
		failed = false;
		try {
			const { data } = await api.POST('/feed-cards/{feedCardId}/decision', {
				params: { path: { feedCardId: card.id } },
				body
			});
			if (!data) throw new Error('decision');
			sheet = false;
			feed.update({ ...data, helper: card.helper });
		} catch {
			failed = true;
		} finally {
			busy = false;
		}
	}

	async function dismiss() {
		const { data } = await api.POST('/feed-cards/{feedCardId}/dismiss', {
			params: { path: { feedCardId: card.id } }
		});
		if (data) feed.update(data);
	}

	function sendChoices() {
		const skipped = pile.filter((c) => skip[c.id]).map((c) => c.id);
		const changed = Object.fromEntries(
			Object.entries(edits).filter(([id, e]) => !skip[id] && Object.keys(e).length > 0)
		);
		if (skipped.length === pile.length) return decide({ state: 'skipped' });
		const hasEdits = Object.keys(changed).length > 0;
		decide({
			state: hasEdits ? 'edited' : 'approved',
			skip: skipped,
			...(hasEdits ? { edits: changed } : {})
		});
	}

	// Swipe on the item (Check first only).
	let dx = $state(0);
	let dragging = false;
	let startX = 0;
	let startT = 0;
	const swipeable = $derived(card.type === 'approval' && isOpen && !busy);
	function down(e: PointerEvent) {
		if (!swipeable || (e.target as HTMLElement).closest('button, a, input, textarea')) return;
		dragging = true;
		startX = e.clientX;
		startT = e.timeStamp;
	}
	function move(e: PointerEvent) {
		if (dragging) dx = e.clientX - startX;
	}
	function up(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		const v = dx / Math.max(1, e.timeStamp - startT);
		const width = (e.currentTarget as HTMLElement).clientWidth;
		if (Math.abs(dx) > width * 0.35 || Math.abs(v) > 0.8) {
			decide({ state: dx > 0 ? 'approved' : 'skipped' });
		}
		dx = 0;
	}

	// Where a result came from: links people can open (http(s) only).
	const links = $derived(
		((card.payload.links as { title?: string; url?: string }[] | undefined) ?? [])
			.map((l) => ({ title: l.title ?? '', url: safeURL(l.url) }))
			.filter((l): l is { title: string; url: string } => !!l.url)
	);

	const stateKey = $derived(`feed.state.${card.state}` as Key);
</script>

<article
	class="item {group}"
	class:open={isOpen && asks}
	style:transform={dx ? `translateX(${dx}px)` : undefined}
	onpointerdown={down}
	onpointermove={move}
	onpointerup={up}
	onpointercancel={() => ((dragging = false), (dx = 0))}
>
	<p class="meta">
		<span class="avatar" aria-hidden="true">{card.helper?.avatar ?? '🧱'}</span>
		<span class="who">{card.helper?.name ?? 'Laamo'}</span>
		<span>· {ago(card.created_at)}</span>
		{#if card.payload.test}<span class="tag">{t('feed.test')}</span>{/if}
		{#if !isOpen || !asks}
			<button class="hide" type="button" aria-label={t('feed.dismiss')} onclick={dismiss}>
				<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"
					><path
						d="M6 6l12 12M18 6L6 18"
						stroke="currentColor"
						stroke-width="2.5"
						stroke-linecap="round"
					/></svg
				>
			</button>
		{/if}
	</p>

	{#if card.type === 'approval'}
		<h3 class="display">
			{isOpen ? t('feed.check.title', { n: Number(card.payload.cards ?? 0) }) : card.title}
		</h3>
	{:else if card.type === 'pick'}
		<h3 class="display">{t('feed.pick.title')}</h3>
	{:else}
		<h3 class="display">{card.type === 'question' ? (card.body ?? card.title) : card.title}</h3>
	{/if}
	{#if card.body && card.type !== 'question' && card.type !== 'approval' && card.type !== 'pick'}
		<p class="body">{card.body}</p>
	{/if}

	{#if isOpen && asks}
		{#if card.expires_at}<p class="due">
				{t('feed.expires', { when: when(card.expires_at, i18n.lang) })}
			</p>{/if}
		{#if card.type === 'approval'}
			<div class="actions three">
				<button class="btn primary" disabled={busy} onclick={() => decide({ state: 'approved' })}
					>{t('feed.check.approveAll')}</button
				>
				<button class="btn" disabled={busy} onclick={openPile}>{t('feed.check.look')}</button>
				<button class="btn" disabled={busy} onclick={() => decide({ state: 'skipped' })}
					>{t('feed.check.skipAll')}</button
				>
			</div>
		{:else if card.type === 'pick'}
			<div class="actions">
				<button class="btn primary wide" disabled={busy} onclick={openPile}
					>{t('feed.pick.swipe', { n: Number(card.payload.cards ?? 0) })} →</button
				>
			</div>
		{:else if card.type === 'question'}
			{@const answers = (card.payload.answers as string[] | undefined) ?? []}
			{#if answers.length}
				<div class="answers">
					{#each answers as a (a)}
						<button
							class="btn"
							disabled={busy}
							onclick={() => decide({ state: 'answered', answer: a })}>{a}</button
						>
					{/each}
				</div>
			{/if}
			{#if card.payload.free_text !== false}
				<form
					class="free"
					onsubmit={(e) => {
						e.preventDefault();
						if (answer.trim()) decide({ state: 'answered', answer: answer.trim() });
					}}
				>
					<input
						bind:value={answer}
						placeholder={t('feed.question.placeholder')}
						aria-label={t('feed.question.placeholder')}
					/>
					<button class="btn primary" disabled={busy || !answer.trim()}
						>{t('feed.question.send')}</button
					>
				</form>
			{/if}
		{/if}
	{:else if asks}
		<p class="done">{t(stateKey)}</p>
	{:else if card.type === 'result' && links.length}
		<ul class="links" aria-label={t('feed.sources')}>
			{#each links as l (l.url)}
				<li>
					<a href={l.url} target="_blank" rel="noopener noreferrer nofollow">
						<span class="ltitle">{l.title || hostOf(l.url)}</span>
						<span class="lhost">{hostOf(l.url)} ↗</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
	{#if card.type === 'result' && count > 1}
		<button class="link" type="button" onclick={openPile}
			>{t('feed.showAll', { n: count })} →</button
		>
	{/if}
	{#if failed}<p class="failed" role="alert">{t('feed.failed')}</p>{/if}
</article>

{#if card.type === 'pick'}
	<BottomSheet bind:open={sheet} title={t('feed.pick.title')}>
		<SwipeStack
			items={pile}
			ondecide={(c, keep) => {
				if (keep) picked = [...picked, c.id];
				if (c === pile[pile.length - 1]) decide({ state: 'answered', picked });
			}}
		>
			{#snippet item(c)}<CardView card={c} />{/snippet}
		</SwipeStack>
	</BottomSheet>
{:else if card.type === 'approval'}
	<BottomSheet bind:open={sheet} title={t('feed.check.title', { n: pile.length })}>
		<ul class="pile">
			{#each pile as c (c.id)}
				<li class:skipped={skip[c.id]}>
					<CardView card={c} bind:edits={edits[c.id]} editing={editing === c.id} />
					<div class="row">
						<button
							class="btn"
							type="button"
							aria-pressed={!skip[c.id]}
							onclick={() => (skip = { ...skip, [c.id]: !skip[c.id] })}
							>{skip[c.id] ? t('feed.check.approve') : t('swipe.skip')}</button
						>
						{#if !skip[c.id]}
							<button
								class="btn"
								type="button"
								onclick={() => (editing = editing === c.id ? null : c.id)}
								>{editing === c.id ? t('sheet.close') : t('feed.check.edit')}</button
							>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
		{#snippet footer()}
			<button class="btn primary wide" disabled={busy} onclick={sendChoices}
				>{t('feed.check.done')}</button
			>
		{/snippet}
	</BottomSheet>
{:else}
	<BottomSheet bind:open={sheet} title={card.title}>
		<ul class="pile">
			{#each pile as c (c.id)}<li><CardView card={c} /></li>{/each}
		</ul>
	</BottomSheet>
{/if}

<style>
	.item {
		position: relative;
		padding: var(--space-4) var(--gutter);
		border-bottom: var(--border);
		background: var(--white);
		touch-action: pan-y;
		transition: transform var(--motion-base) var(--ease);
	}
	/* Waiting for you: a red Ask me bar on the left. */
	.item.open::before {
		content: '';
		position: absolute;
		left: 0;
		top: 0;
		bottom: 0;
		width: 4px;
		background: var(--brick-red);
	}
	.meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0 0 var(--space-1);
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.who {
		color: var(--ink);
	}
	.tag {
		padding: 0 var(--space-2);
		border-radius: var(--radius-small);
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-size: 12px;
	}
	.hide {
		margin: calc(var(--space-3) * -1) calc(var(--space-3) * -1) calc(var(--space-3) * -1) auto;
		width: 44px;
		height: 44px;
		border: 0;
		background: none;
		color: var(--muted);
		cursor: pointer;
	}
	h3 {
		margin: 0;
		font-size: var(--text-lg);
	}
	.problem h3 {
		color: var(--danger);
	}
	.body {
		margin: var(--space-1) 0 0;
		white-space: pre-line;
	}
	.due,
	.done {
		margin: var(--space-1) 0 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.done {
		font-weight: 700;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-3);
	}
	/* Check first: the primary action gets its own row on a phone. */
	.actions.three {
		display: grid;
		grid-template-columns: 1fr 1fr;
	}
	.actions.three .btn {
		padding: 0 var(--space-2);
		white-space: nowrap;
	}
	.actions.three .btn.primary {
		grid-column: 1 / -1;
	}
	.answers {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		margin-top: var(--space-3);
	}
	.free {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-3);
	}
	.free input {
		flex: 1;
		min-width: 0;
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
	}
	.link {
		margin-top: var(--space-2);
		padding: 0;
		min-height: 40px;
		border: 0;
		background: none;
		font-weight: 700;
		cursor: pointer;
	}
	.links {
		list-style: none;
		margin: var(--space-3) 0 0;
		padding: 0;
		border-top: var(--border);
	}
	.links a {
		display: flex;
		flex-direction: column;
		padding: var(--space-2) 0;
		min-height: var(--tap-min);
		justify-content: center;
		border-bottom: var(--border);
		color: var(--ink);
		text-decoration: none;
	}
	.ltitle {
		font-weight: 600;
		line-height: 1.3;
	}
	.lhost {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.failed {
		margin: var(--space-2) 0 0;
		color: var(--danger);
		font-weight: 600;
	}
	.pile {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.pile li {
		padding: var(--space-4) 0;
		border-bottom: var(--border);
	}
	.pile li.skipped {
		opacity: 0.45;
	}
	.row {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-3);
	}
</style>
