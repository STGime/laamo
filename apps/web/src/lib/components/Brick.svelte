<!--
  A brick: the one chunky element. Solid brick color, 2px dark bottom edge,
  one sentence in the display face. `state` lights it up during a run.
-->
<script lang="ts" module>
	export type Group = 'when' | 'look' | 'think' | 'do' | 'ask';
	export type BrickState = 'idle' | 'waiting' | 'running' | 'done' | 'failed';
</script>

<script lang="ts">
	let {
		group,
		title,
		sub = '',
		state = 'idle',
		onclick
	}: {
		group: Group;
		title: string;
		sub?: string;
		state?: BrickState;
		onclick?: () => void;
	} = $props();
</script>

{#if onclick}
	<button type="button" class="brick {group}" data-state={state} {onclick}>
		<span class="title">{title}</span>
		{#if sub}<span class="sub">{sub}</span>{/if}
	</button>
{:else}
	<div class="brick {group}" data-state={state}>
		<span class="title">{title}</span>
		{#if sub}<span class="sub">{sub}</span>{/if}
	</div>
{/if}

<style>
	.brick {
		display: flex;
		flex-direction: column;
		justify-content: center;
		width: 100%;
		min-height: 56px;
		padding: var(--space-3) var(--space-4);
		border: 0;
		border-radius: var(--radius-control);
		box-shadow: var(--brick-edge);
		text-align: left;
		transition:
			transform var(--motion-fast) var(--ease),
			opacity var(--motion-base) var(--ease);
	}
	button.brick {
		cursor: pointer;
	}
	button.brick:active {
		transform: scale(var(--press-scale));
	}
	.title {
		font-family: var(--font-display);
		font-weight: 800;
		font-size: var(--text-lg);
		line-height: 1.2;
	}
	.sub {
		font-size: var(--text-sm);
		font-weight: 600;
		opacity: 0.85;
	}
	.when {
		background: var(--brick-yellow);
		color: var(--on-yellow);
	}
	.look {
		background: var(--brick-blue);
		color: var(--on-blue);
	}
	.think {
		background: var(--brick-purple);
		color: var(--on-purple);
	}
	.do {
		background: var(--brick-green);
		color: var(--on-green);
	}
	.ask {
		background: var(--brick-red);
		color: var(--on-red);
	}
	/* During a run the working brick steps out of the stack. */
	[data-state='running'] {
		transform: translateX(var(--space-2));
	}
	[data-state='failed'] {
		outline: 3px solid var(--ink);
		outline-offset: -3px;
	}
</style>
