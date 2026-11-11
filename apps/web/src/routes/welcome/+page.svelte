<!-- Onboarding after the first sign-in: a name, then up to 3 needs. Creates the workspace. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import type { Schemas } from '#lib/api/client.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	type Need = Schemas['Need'];
	const all: Need[] = [
		'find_customers',
		'stay_up_to_date',
		'sort_emails',
		'book_meetings',
		'watch_competitors',
		'plan_events',
		'post_social'
	];

	let step = $state(1);
	let name = $state('');
	let business = $state('');
	let needs = $state<Need[]>([]);
	let busy = $state(false);
	let failed = $state(false);

	function toggle(n: Need) {
		if (needs.includes(n)) needs = needs.filter((x) => x !== n);
		else if (needs.length < 3) needs = [...needs, n];
	}

	async function finish() {
		busy = true;
		failed = false;
		const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Europe/Berlin';
		const { data } = await api.POST('/workspaces', {
			body: {
				name: business.trim() || name.trim(),
				timezone,
				locale: i18n.lang,
				profile: { name: name.trim() },
				needs
			}
		});
		if (!data) {
			busy = false;
			failed = true;
			return;
		}
		await session.refresh();
		goto('/feed', { replaceState: true });
	}
</script>

<div class="wrap">
	<p class="steps" aria-hidden="true">
		<span class:on={step >= 1}></span><span class:on={step >= 2}></span>
	</p>
	{#if step === 1}
		<h1>{t('welcome.name.title')}</h1>
		<form
			onsubmit={(e) => {
				e.preventDefault();
				if (name.trim()) step = 2;
			}}
		>
			<label>
				<span>{t('welcome.name')}</span>
				<input bind:value={name} autocomplete="given-name" maxlength="80" />
			</label>
			<label>
				<span>{t('welcome.business')} <em>· {t('welcome.optional')}</em></span>
				<input
					bind:value={business}
					autocomplete="organization"
					maxlength="80"
					placeholder="Bäckerei Krause"
				/>
			</label>
			<button class="btn primary wide" disabled={!name.trim()}>{t('welcome.next')}</button>
		</form>
	{:else}
		<h1>{t('welcome.needs.title')}</h1>
		<p class="body">{t('welcome.needs.body')}</p>
		<div class="chips">
			{#each all as n (n)}
				<button
					type="button"
					aria-pressed={needs.includes(n)}
					disabled={!needs.includes(n) && needs.length >= 3}
					onclick={() => toggle(n)}>{t(`need.${n}` as Key)}</button
				>
			{/each}
		</div>
		{#if failed}<p class="error" role="alert">{t('feed.failed')}</p>{/if}
		<div class="cta">
			<button class="btn primary wide" disabled={busy || needs.length === 0} onclick={finish}
				>{t('welcome.done')}</button
			>
		</div>
	{/if}
</div>

<style>
	.wrap {
		padding: var(--space-6) var(--gutter) 120px;
	}
	.steps {
		display: flex;
		gap: var(--space-1);
		margin: 0 0 var(--space-6);
	}
	.steps span {
		width: 32px;
		height: 6px;
		border-radius: 3px;
		background: var(--gray-200);
	}
	.steps span.on {
		background: var(--ink);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		margin-top: var(--space-5);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-weight: 700;
	}
	em {
		font-style: normal;
		font-weight: 400;
		color: var(--muted);
	}
	input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
		font-weight: 400;
	}
	.body {
		margin: var(--space-3) 0 var(--space-5);
		color: var(--muted);
	}
	/* Big chips: picked ones turn into solid brick-colored blocks. */
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.chips button {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font-size: var(--text-lg);
		font-weight: 700;
		cursor: pointer;
		transition: transform var(--motion-fast) var(--ease);
	}
	.chips button:active {
		transform: scale(var(--press-scale));
	}
	.chips button[aria-pressed='true'] {
		background: var(--brick-yellow);
		color: var(--on-yellow);
		box-shadow: var(--brick-edge);
	}
	.chips button:disabled {
		opacity: 0.4;
		cursor: default;
	}
	.error {
		color: var(--danger);
		font-weight: 600;
	}
	.cta {
		position: fixed;
		bottom: 0;
		left: 50%;
		transform: translateX(-50%);
		width: 100%;
		max-width: var(--column-max);
		padding: var(--space-3) var(--gutter) calc(var(--space-3) + env(safe-area-inset-bottom));
		background: var(--white);
		border-top: var(--border);
	}
</style>
