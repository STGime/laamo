<!-- A template: its bricks in plain words, the questions still open, then a test run. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Schemas } from '#lib/api/client.ts';
	import BrickStack, { type StackItem } from '#lib/components/BrickStack.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	let tpl = $state<Schemas['ShelfTemplate'] | null>(null);
	let answers = $state<Record<string, string>>({});
	let busy = $state(false);
	let error = $state('');

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/templates', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				tpl = data?.templates.find((x) => x.slug === page.params.slug) ?? null;
				for (const q of tpl?.questions ?? []) if (q.kind === 'place') answers[q.key] = 'feed';
			});
	});

	// Bricks in plain words: About me facts fill in as they're typed, and a
	// chat step answered "just my Feed" says so.
	const toFeed = $derived(
		new Set(
			(tpl?.questions ?? [])
				.filter((q) => q.kind === 'place' && answers[q.key] === 'feed')
				.flatMap((q) => q.fills ?? [])
		)
	);
	function fill(text: string): string {
		return text.replace(
			/\{my (\w+)\}/g,
			(m, key: string) => answers[key]?.trim() || t(`pill.${key}` as Key) || m
		);
	}
	const bricks = $derived<StackItem[]>(
		(tpl?.bricks ?? []).map((b) =>
			toFeed.has(b.id)
				? { id: b.id, group: 'do', title: t('tpl.toFeed') }
				: { id: b.id, group: b.group, title: fill(b.sentence) }
		)
	);
	const ready = $derived(!!tpl && tpl.questions.every((q) => (answers[q.key] ?? '').trim() !== ''));

	async function tryIt() {
		const ws = session.workspace;
		if (!ws || !tpl) return;
		busy = true;
		error = '';
		const { data, error: err } = await api.POST('/workspaces/{workspaceId}/helpers', {
			params: { path: { workspaceId: ws.id } },
			body: { template: tpl.slug, answers, run_test: true }
		});
		busy = false;
		if (!data) {
			error = t('tpl.failed', { error: (err as { error?: string } | undefined)?.error ?? '' });
			return;
		}
		goto('/feed');
	}
</script>

{#if tpl}
	<div class="page">
		<header>
			<span class="avatar" aria-hidden="true">{tpl.avatar}</span>
			<h1>{tpl.name}</h1>
			<p class="promise">{tpl.promise}</p>
		</header>

		<section>
			<h2>{t('tpl.how')}</h2>
			<BrickStack {bricks} />
		</section>

		{#if tpl.questions.length}
			<section>
				<h2>{t('tpl.setup')}</h2>
				{#each tpl.questions as q (q.key)}
					<div class="question">
						<label for="q-{q.key}">{q.text}</label>
						{#if q.kind === 'place'}
							<div class="choices" id="q-{q.key}" role="radiogroup">
								<button
									type="button"
									role="radio"
									aria-checked={answers[q.key] === 'feed'}
									onclick={() => (answers[q.key] = 'feed')}>{t('tpl.feedOnly')}</button
								>
							</div>
						{:else}
							<input id="q-{q.key}" bind:value={answers[q.key]} placeholder={q.placeholder ?? ''} />
						{/if}
						{#if q.profile}<p class="hint">{t('tpl.remember')}</p>{/if}
					</div>
				{/each}
			</section>
		{/if}
		{#if error}<p class="error" role="alert">{error}</p>{/if}
	</div>

	<div class="cta">
		<button class="btn primary wide" disabled={!ready || busy} onclick={tryIt}
			>{t('tpl.test')}</button
		>
		<p class="hint">{t('tpl.testHint')}</p>
	</div>
{:else}
	<p class="loading">{t('loading')}</p>
{/if}

<style>
	.page {
		padding-bottom: 180px;
	}
	header {
		padding: var(--space-6) var(--gutter) var(--space-4);
	}
	.avatar {
		font-size: 44px;
	}
	h1 {
		margin-top: var(--space-2);
	}
	.promise {
		margin: var(--space-2) 0 0;
		font-size: var(--text-lg);
		color: var(--muted);
	}
	section {
		padding: var(--space-4) var(--gutter);
		border-top: var(--border);
	}
	h2 {
		margin-bottom: var(--space-3);
		font-size: var(--text-lg);
	}
	.question {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-bottom: var(--space-4);
	}
	label {
		font-weight: 700;
	}
	input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
	}
	.choices {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.choices button {
		min-height: var(--tap-min);
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font-weight: 700;
		cursor: pointer;
	}
	.choices button[aria-checked='true'] {
		background: var(--brick-green);
		color: var(--on-green);
		box-shadow: var(--brick-edge);
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.error {
		margin: 0;
		padding: 0 var(--gutter);
		color: var(--danger);
		font-weight: 600;
	}
	.loading {
		padding: var(--space-6) var(--gutter);
		color: var(--muted);
	}
	.cta {
		position: fixed;
		bottom: calc(var(--tabbar-height) + env(safe-area-inset-bottom));
		left: 50%;
		transform: translateX(-50%);
		width: 100%;
		max-width: var(--column-max);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-3) var(--gutter);
		background: var(--white);
		border-top: var(--border);
	}
</style>
