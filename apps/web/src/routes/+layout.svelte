<script lang="ts">
	import '#lib/styles/tokens.css';
	let { children } = $props();
</script>

<main class="column">
	{@render children()}
</main>

<style>
	.column {
		max-width: var(--column-max);
		margin: 0 auto;
		padding: 0 16px;
		min-height: 100dvh;
	}
</style>
