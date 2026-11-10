<script lang="ts">
	import { getHealth, type Health } from '#lib/api/health.ts';

	let health = $state<Health | null>(null);
	let error = $state('');

	$effect(() => {
		getHealth()
			.then((h) => (health = h))
			.catch((e: unknown) => (error = String(e)));
	});
</script>

<h1>Laamo</h1>
<p>Little helpers that do your busywork.</p>

<div class="bricks" aria-hidden="true">
	<span style="background: var(--brick-yellow)">When</span>
	<span style="background: var(--brick-blue); color: var(--white)">Look</span>
	<span style="background: var(--brick-purple); color: var(--white)">Think</span>
	<span style="background: var(--brick-green)">Do</span>
	<span style="background: var(--brick-red); color: var(--white)">Ask me</span>
</div>

<p class="status">
	{#if health}
		API: {health.status} · env: {health.env} · Eurobase: {health.eurobase}
	{:else if error}
		API not reachable. Run <code>task dev</code>.
	{:else}
		Checking API…
	{/if}
</p>

<style>
	h1 {
		font-size: 40px;
		line-height: 1;
		margin: 32px 0 8px;
	}
	.bricks {
		display: flex;
		flex-direction: column;
		gap: 4px;
		margin: 24px 0;
	}
	.bricks span {
		display: block;
		padding: 12px 16px;
		border-radius: var(--radius-control);
		box-shadow: var(--brick-edge);
		font-weight: 800;
	}
	.status {
		color: var(--gray-600);
	}
</style>
