<!--
  Swipe stack (Let me pick, Check first): drag right to keep, left to skip,
  or use the buttons / arrow keys. A fast flick counts even when short.
-->
<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.ts';

	let {
		items,
		card,
		ondecide,
		keepLabel = t('swipe.keep'),
		skipLabel = t('swipe.skip')
	}: {
		items: T[];
		card: Snippet<[T]>;
		ondecide: (item: T, keep: boolean) => void;
		keepLabel?: string;
		skipLabel?: string;
	} = $props();

	let index = $state(0);
	let dx = $state(0);
	let dragging = $state(false);
	let leaving = $state<0 | 1 | -1>(0);
	let width = $state(360);
	let start = { x: 0, t: 0 };
	let lastMove = { x: 0, t: 0 };

	const top = $derived(items[index]);
	const left = $derived(items.length - index);
	const reduced =
		typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;

	function decide(keep: boolean) {
		if (!top || leaving) return;
		const item = top;
		leaving = keep ? 1 : -1;
		dx = (keep ? 1 : -1) * width * 1.4;
		setTimeout(
			() => {
				ondecide(item, keep);
				index++;
				leaving = 0;
				dx = 0;
			},
			reduced ? 0 : 200
		);
	}

	function down(e: PointerEvent) {
		if (leaving) return;
		dragging = true;
		start = { x: e.clientX, t: e.timeStamp };
		lastMove = start;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
	}
	function move(e: PointerEvent) {
		if (!dragging) return;
		dx = e.clientX - start.x;
		lastMove = { x: e.clientX, t: e.timeStamp };
	}
	function up(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		const v = (e.clientX - start.x) / Math.max(1, e.timeStamp - start.t); // px per ms
		if (Math.abs(dx) > width * 0.3 || Math.abs(v) > 0.6) decide(dx > 0);
		else dx = 0;
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'ArrowRight') decide(true);
		if (e.key === 'ArrowLeft') decide(false);
	}
</script>

<div class="swipe" bind:clientWidth={width}>
	{#if top}
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
		<div
			class="card"
			class:dragging
			role="group"
			aria-roledescription="swipe card"
			aria-label={t('swipe.left', { n: left })}
			tabindex="0"
			style:transform="translateX({dx}px) rotate({dx / 24}deg)"
			onpointerdown={down}
			onpointermove={move}
			onpointerup={up}
			onpointercancel={up}
			onkeydown={key}
		>
			<span class="stamp keep" style:opacity={Math.max(0, Math.min(1, dx / 80))}>{keepLabel}</span>
			<span class="stamp skip" style:opacity={Math.max(0, Math.min(1, -dx / 80))}>{skipLabel}</span>
			{@render card(top)}
		</div>
		<p class="count">{t('swipe.left', { n: left })}</p>
		<div class="actions">
			<button type="button" class="btn" onclick={() => decide(false)}>← {skipLabel}</button>
			<button type="button" class="btn primary" onclick={() => decide(true)}>{keepLabel} →</button>
		</div>
	{:else}
		<p class="done display">{t('swipe.done')}</p>
	{/if}
</div>

<style>
	.swipe {
		position: relative;
		overflow: hidden;
		padding: var(--space-2) 0;
	}
	.card {
		position: relative;
		min-height: 220px;
		padding: var(--space-5) var(--space-4);
		background: var(--white);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		touch-action: pan-y;
		user-select: none;
		cursor: grab;
		transition: transform var(--motion-base) var(--ease);
	}
	.card.dragging {
		transition: none;
		cursor: grabbing;
	}
	.stamp {
		position: absolute;
		top: var(--space-3);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-small);
		font-family: var(--font-display);
		font-weight: 800;
		text-transform: uppercase;
		pointer-events: none;
	}
	.stamp.keep {
		left: var(--space-3);
		background: var(--brick-green);
		color: var(--on-green);
	}
	.stamp.skip {
		right: var(--space-3);
		background: var(--ink);
		color: var(--white);
	}
	.count {
		margin: var(--space-2) 0;
		color: var(--muted);
		font-size: var(--text-sm);
		text-align: center;
	}
	.actions {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
	.actions .btn {
		padding: 0 var(--space-3);
		white-space: nowrap;
	}
	.done {
		padding: var(--space-6) 0;
		text-align: center;
		font-size: var(--text-h2);
	}
</style>
