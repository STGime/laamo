<!--
  A time as hour + minute, 24h or 12h (AM/PM) as the workspace likes it.
  The value is always "HH:MM". Native time inputs follow the browser's
  locale instead, so they can't honour the setting.
-->
<script lang="ts">
	import { join, split } from '#lib/clock.ts';
	import { i18n, t } from '#lib/i18n/index.svelte.ts';

	let { id, value, onchange }: { id: string; value: string; onchange: (v: string) => void } =
		$props();

	const cur = $derived(split(value));
	const pm = $derived(cur.h >= 12);
	// Minutes in 5s, plus the current one if it's in between.
	const minutes = $derived(
		[...new Set([...Array.from({ length: 12 }, (_, i) => i * 5), cur.m])].toSorted((a, b) => a - b)
	);
	const hours = $derived(
		i18n.twelve ? [12, ...Array.from({ length: 11 }, (_, i) => i + 1)] : [...Array(24).keys()]
	);
	const shownHour = $derived(i18n.twelve ? cur.h % 12 || 12 : cur.h);

	function setHour(h: number) {
		onchange(join(i18n.twelve ? (h % 12) + (pm ? 12 : 0) : h, cur.m));
	}
	function setHalf(toPm: boolean) {
		if (toPm !== pm) onchange(join((cur.h + 12) % 24, cur.m));
	}
	const two = (n: number) => String(n).padStart(2, '0');
</script>

<div class="time">
	<select
		{id}
		aria-label={t('time.hours')}
		value={shownHour}
		onchange={(e) => setHour(Number(e.currentTarget.value))}
	>
		{#each hours as h (h)}<option value={h}>{i18n.twelve ? h : two(h)}</option>{/each}
	</select>
	<span aria-hidden="true">:</span>
	<select
		aria-label={t('time.minutes')}
		value={cur.m}
		onchange={(e) => onchange(join(cur.h, Number(e.currentTarget.value)))}
	>
		{#each minutes as m (m)}<option value={m}>{two(m)}</option>{/each}
	</select>
	{#if i18n.twelve}
		<div class="half" role="group" aria-label="AM/PM">
			<button type="button" aria-pressed={!pm} onclick={() => setHalf(false)}>AM</button>
			<button type="button" aria-pressed={pm} onclick={() => setHalf(true)}>PM</button>
		</div>
	{/if}
</div>

<style>
	.time {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	select {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font: inherit;
		font-size: var(--text-lg);
		font-variant-numeric: tabular-nums;
	}
	span {
		font-weight: 800;
		font-size: var(--text-lg);
	}
	.half {
		display: flex;
		gap: var(--space-1);
		margin-left: var(--space-2);
	}
	.half button {
		min-height: 56px;
		min-width: 56px;
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font-weight: 700;
		cursor: pointer;
	}
	.half [aria-pressed='true'] {
		background: var(--ink);
		color: var(--white);
	}
</style>
