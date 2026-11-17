<!-- A helper as a stack of flat bricks, one sentence each (site pages). -->
<script lang="ts">
	import Brick from '#lib/components/Brick.svelte';
	import type { Stack } from './copy.ts';

	let { stack }: { stack: Stack } = $props();
</script>

<div class="stack">
	{#each stack as b, i (i)}<Brick group={b.group} title={b.text} />{/each}
</div>

<style>
	.stack {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin-top: var(--space-3);
	}
</style>
