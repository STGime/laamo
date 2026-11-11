<!--
  A helper's bricks, stacked and touching like real bricks. During a run,
  `current` is the brick working now: earlier ones are done, later ones dim.
-->
<script lang="ts">
	import Brick, { type BrickState, type Group } from './Brick.svelte';

	export interface StackItem {
		id: string;
		group: Group;
		title: string;
		sub?: string;
		state?: BrickState;
	}

	let {
		bricks,
		current = -1,
		onselect
	}: {
		bricks: StackItem[];
		current?: number;
		onselect?: (b: StackItem, i: number) => void;
	} = $props();

	function stateOf(b: StackItem, i: number): BrickState {
		if (b.state) return b.state;
		if (current < 0) return 'idle';
		return i < current ? 'done' : i === current ? 'running' : 'idle';
	}
</script>

<ol class="stack">
	{#each bricks as b, i (b.id)}
		<li class:dim={current >= 0 && i > current}>
			<Brick
				group={b.group}
				title={b.title}
				sub={b.sub}
				state={stateOf(b, i)}
				onclick={onselect ? () => onselect(b, i) : undefined}
			/>
		</li>
	{/each}
</ol>

<style>
	.stack {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	li {
		transition: opacity var(--motion-base) var(--ease);
	}
	.dim {
		opacity: 0.4;
	}
</style>
