<!--
  A run that's going on: a spinner, where it is ("Running · step 3 of 9")
  and the seconds since it started here, so a slow brick doesn't look stuck.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { progressText } from '#lib/progress.ts';

	let {
		run,
		retrying = false,
		waiting = '',
		slow = false
	}: {
		run?: Schemas['RunProgress'];
		/** A step failed and waits for its retry. */
		retrying?: boolean;
		/** What to say before the run shows up (e.g. "Starting…"). */
		waiting?: string;
		/** After 30 s, say that some bricks take a while (not inside buttons). */
		slow?: boolean;
	} = $props();

	const id = $derived(run?.id ?? '');
	let start = $state(Date.now());
	let now = $state(Date.now());
	// A new run starts the count again.
	$effect(() => {
		void id;
		start = now = Date.now();
	});
	$effect(() => {
		const timer = setInterval(() => (now = Date.now()), 1000);
		return () => clearInterval(timer);
	});
	const secs = $derived(Math.max(0, Math.round((now - start) / 1000)));
</script>

<span class="ticker" role="status">
	<span class="spinner" aria-hidden="true"></span>
	{retrying ? t('builder.retrying') : run ? progressText(run) : waiting || t('helpers.queued')}
	<span class="secs">· {t('builder.seconds', { n: secs })}</span>
</span>
{#if slow && secs >= 30 && !retrying}<span class="slow">{t('builder.slow')}</span>{/if}

<style>
	.ticker {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
	.spinner {
		flex: none;
		width: 0.9em;
		height: 0.9em;
		border: 2px solid currentColor;
		border-top-color: transparent;
		border-radius: 50%;
		animation: spin 0.9s linear infinite;
	}
	@media (prefers-reduced-motion: reduce) {
		.spinner {
			animation: none;
		}
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	.secs {
		opacity: 0.75;
		font-variant-numeric: tabular-nums;
	}
	.slow {
		display: block;
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 400;
	}
</style>
