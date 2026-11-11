<!-- A template: its bricks in plain words, the questions still open, then a test run. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Schemas } from '#lib/api/client.ts';
	import BrickStack, { type StackItem } from '#lib/components/BrickStack.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { listen } from '#lib/feed.svelte.ts';
	import { cities } from '#lib/cities.ts';
	import { brickProgress, progressText } from '#lib/progress.ts';

	let tpl = $state<Schemas['ShelfTemplate'] | null>(null);
	let answers = $state<Record<string, string>>({});
	let places = $state<Schemas['Place'][]>([]);
	// Chats this template can post to (Discord, Slack, …).
	const usable = $derived(places.filter((p) => tpl?.places.includes(p.type) && p.verified_at));
	// About me facts the user wants for this helper only.
	let justHere = $state<string[]>([]);
	let busy = $state(false);
	let error = $state('');
	// After Try it now: the new helper's first run, live.
	let helperId = $state<string | null>(null);
	let detail = $state<Schemas['HelperDetail'] | null>(null);

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/templates', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				tpl = data?.templates.find((x) => x.slug === page.params.slug) ?? null;
				// Back from connecting a chat: that chat is the answer.
				const back = page.url.searchParams.get('place');
				for (const q of tpl?.questions ?? [])
					if (q.kind === 'place') answers[q.key] = back ?? 'feed';
			});
		api
			.GET('/workspaces/{workspaceId}/places', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				places = data?.places ?? [];
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
	// While the first run goes, the bricks light up; when it's done (or
	// waits for you), the Feed has its card.
	const live = $derived(detail ? brickProgress(bricks, detail.helper.run, detail.steps) : null);
	async function loadRun() {
		if (!helperId) return;
		const { data } = await api.GET('/helpers/{helperId}', {
			params: { path: { helperId } }
		});
		if (!data) return;
		detail = data;
		const run = data.helper.run;
		if (!run || run.status === 'waiting') goto('/feed', { replaceState: true });
	}
	$effect(() => {
		if (!helperId) return;
		loadRun();
		return listen(loadRun, () => session.fresh(), ['run', 'step', 'reload']);
	});

	const ready = $derived(!!tpl && tpl.questions.every((q) => (answers[q.key] ?? '').trim() !== ''));

	async function tryIt() {
		const ws = session.workspace;
		if (!ws || !tpl) return;
		busy = true;
		error = '';
		const { data, error: err } = await api.POST('/workspaces/{workspaceId}/helpers', {
			params: { path: { workspaceId: ws.id } },
			body: { template: tpl.slug, answers, run_test: true, just_this_helper: justHere }
		});
		if (!data) {
			busy = false;
			error = t('tpl.failed', { error: (err as { error?: string } | undefined)?.error ?? '' });
			return;
		}
		if (data.run_id) {
			helperId = data.helper.id;
			document.getElementById('how')?.scrollIntoView({ block: 'start' });
		} else goto('/feed');
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
			<h2 id="how">{t('tpl.how')}</h2>
			<BrickStack bricks={(live?.items ?? bricks) as StackItem[]} current={live?.current ?? -1} />
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
								{#each usable as p (p.id)}
									<button
										type="button"
										role="radio"
										aria-checked={answers[q.key] === p.id}
										onclick={() => (answers[q.key] = p.id)}>{p.label}</button
									>
								{/each}
							</div>
							{#if session.workspace?.role === 'owner'}
								<a class="add" href="/places/new?back={encodeURIComponent(`/new/${tpl.slug}`)}"
									>+ {t('tpl.addChat')}</a
								>
							{/if}
						{:else}
							<input
								id="q-{q.key}"
								bind:value={answers[q.key]}
								placeholder={q.placeholder ?? ''}
								list={q.key === 'city' ? 'cities' : undefined}
								autocomplete={q.key === 'city' ? 'address-level2' : 'off'}
							/>
						{/if}
						{#if q.profile}
							<label class="just">
								<input
									type="checkbox"
									checked={justHere.includes(q.key)}
									onchange={(e) =>
										(justHere = e.currentTarget.checked
											? [...justHere, q.key]
											: justHere.filter((k) => k !== q.key))}
								/>
								{t('tpl.justHere')}
							</label>
							<p class="hint">
								{justHere.includes(q.key) ? t('tpl.justHereHint') : t('tpl.remember')}
							</p>
						{/if}
					</div>
				{/each}
			</section>
		{/if}
		{#if error}<p class="error" role="alert">{error}</p>{/if}
		<datalist id="cities">
			{#each cities as c (c)}<option value={c}></option>{/each}
		</datalist>
	</div>

	<div class="cta">
		<button class="btn primary wide" disabled={!ready || busy} onclick={tryIt}
			>{detail?.helper.run
				? progressText(detail.helper.run)
				: busy
					? t('helpers.queued')
					: t('tpl.test')}</button
		>
		<p class="hint">{t('tpl.testHint')}</p>
	</div>
{:else}
	<p class="loading">{t('loading')}</p>
{/if}

<style>
	.add {
		display: inline-flex;
		align-items: center;
		min-height: var(--tap-min);
		color: inherit;
		font-weight: 700;
		text-decoration: none;
	}
	.just {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.just input {
		width: 20px;
		height: 20px;
		accent-color: var(--ink);
	}
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
