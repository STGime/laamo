<!-- Component kit: every shell component on one screen, for review at 375px and 1440px. -->
<script lang="ts">
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import BrickStack, { type StackItem } from '#lib/components/BrickStack.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import SwipeStack from '#lib/components/SwipeStack.svelte';

	const bricks: StackItem[] = [
		{ id: 'b1', group: 'when', title: 'Every weekday at 7:30' },
		{
			id: 'b2',
			group: 'look',
			title: 'Search the web for bakery news in Leipzig',
			sub: '→ 10 results'
		},
		{ id: 'b3', group: 'think', title: 'Keep the 5 that matter most', sub: '→ max 5' },
		{ id: 'b4', group: 'ask', title: 'Check with me first' },
		{ id: 'b5', group: 'do', title: 'Post in #news' }
	];
	let current = $state(-1);
	let sheet = $state(false);
	let kept = $state<string[]>([]);
	const leads = [
		{ name: 'Bäckerei Krause', note: 'No online shop · Gohlis' },
		{ name: 'Café Blume', note: 'Breakfast until 14:00 · Südvorstadt' },
		{ name: 'Backhaus Ost', note: 'Family bakery since 1911' }
	];

	function run() {
		current = 0;
		const tick = setInterval(() => {
			current++;
			if (current >= bricks.length) {
				clearInterval(tick);
				current = -1;
			}
		}, 700);
	}
</script>

<PageHeader title="Component kit" />

<section>
	<h2>Bricks</h2>
	<BrickStack {bricks} {current} onselect={() => (sheet = true)} />
	<div class="row">
		<button class="btn primary" onclick={run}>Run now</button>
		<button class="btn" onclick={() => (sheet = true)}>Open a sheet</button>
	</div>
</section>

<section>
	<h2>Let me pick</h2>
	<SwipeStack items={leads} ondecide={(l, keep) => keep && (kept = [...kept, l.name])}>
		{#snippet card(l)}
			<h3 class="display">{l.name}</h3>
			<p>{l.note}</p>
		{/snippet}
	</SwipeStack>
	{#if kept.length}<p class="muted">Kept: {kept.join(', ')}</p>{/if}
</section>

<section>
	<h2>Type</h2>
	<p class="display number">12</p>
	<p>Found 8 bakeries in Leipzig. Body text is the system font at 16px.</p>
	<p class="muted">Muted text for times and counts.</p>
</section>

<BottomSheet bind:open={sheet} title="Search the web">
	<p>Up to 3 settings live here, with big inputs and chips for fields.</p>
	{#snippet footer()}
		<button class="btn primary wide" onclick={() => (sheet = false)}>Done</button>
	{/snippet}
</BottomSheet>

<style>
	section {
		padding: var(--space-4) var(--gutter) var(--space-5);
		border-bottom: var(--border);
	}
	h2 {
		margin-bottom: var(--space-3);
	}
	h3 {
		margin: 0 0 var(--space-2);
		font-size: var(--text-h2);
	}
	.row {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-4);
	}
	.number {
		font-size: var(--text-number);
		margin: 0;
	}
	.muted {
		color: var(--muted);
	}
</style>
