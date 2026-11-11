<!-- Big, tight, left-aligned screen title with an optional right side. -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	let { title, right }: { title: string; right?: Snippet } = $props();
</script>

<header>
	<h1>{title}</h1>
	{#if right}<div class="right">{@render right()}</div>{/if}
</header>

<style>
	header {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--space-3);
		padding: var(--space-6) var(--gutter) var(--space-3);
	}
	.right {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
</style>
