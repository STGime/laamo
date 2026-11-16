<!--
  S9 Brick builder: the helper's bricks, dragged into order. A color in the
  tray shows its bricks; a brick opens its sheet. Save makes a new version.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { dndzone, type DndEvent } from 'svelte-dnd-action';
	import { brick, catalog, type GroupId, type Stack, type StackBrick } from '@laamo/catalog';
	import { sentence } from '@laamo/catalog/describe';
	import { available, validate, type Issue } from '@laamo/catalog/validate';
	import type { Schemas } from '#lib/api/client.ts';
	import Brick, { type BrickState } from '#lib/components/Brick.svelte';
	import { listen } from '#lib/feed.svelte.ts';
	import { brickProgress, retrying } from '#lib/progress.ts';
	import { settingFix, starved } from '#lib/builder.ts';
	import RunTicker from '#lib/components/RunTicker.svelte';
	import BrickSheet, { type Pill } from '#lib/components/BrickSheet.svelte';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import {
		add,
		applyFix,
		fieldPills,
		fillProfile,
		groups,
		ready,
		startOnTop,
		tray
	} from '#lib/builder.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	const id = $derived(page.params.id ?? '');
	const lang = $derived(i18n.lang);

	let name = $state('');
	// A draft (just made from "Use in a helper" or "Build your own"): Save
	// makes it a helper, Cancel deletes it.
	let isDraft = $state(false);
	// The helper's name, editable here (a new one is named after its list).
	let newName = $state('');
	const renamed = $derived(!!newName.trim() && newName.trim() !== name);
	let version = $state(0);
	let base = $state<Stack | null>(null); // as loaded, for "anything changed?"
	let bricks = $state<StackBrick[]>([]);
	let lastOut = $state<Record<string, number>>({});
	let emailAddress = $state<string | null>(null);
	let places = $state<Schemas['Place'][]>([]);
	let lists = $state<Schemas['ListSummary'][]>([]);
	let about = $state<Record<string, unknown>>({});
	let missing = $state(false);
	let saving = $state(false);
	let error = $state('');
	let serverIssues = $state<Schemas['StackIssue'][]>([]);

	let editing = $state<string | null>(null); // brick id whose sheet is open
	let sheetOpen = $state(false);
	let trayGroup = $state<GroupId | null>(null);
	let trayOpen = $state(false);
	function openTray(g: GroupId) {
		trayGroup = g;
		trayOpen = true;
	}

	// An unsaved edit survives a trip to "Connect a chat".
	const draftKey = $derived(`laamo.edit.${id}`);

	$effect(() => {
		if (!id) return;
		api.GET('/helpers/{helperId}', { params: { path: { helperId: id } } }).then(({ data }) => {
			if (data) {
				name = newName = data.helper.name;
				isDraft = data.helper.status === 'draft';
			}
		});
		api
			.GET('/helpers/{helperId}/stack', { params: { path: { helperId: id } } })
			.then(({ data, response }) => {
				if (response.status === 404) missing = true;
				if (!data) return;
				version = data.version;
				base = data.stack as Stack;
				lastOut = data.last_out;
				emailAddress = data.email_address ?? null;
				bricks = structuredClone(data.stack.bricks) as StackBrick[];
				// An edit that wasn't saved comes back. After Connect a chat
				// that's expected; otherwise it's marked, with Discard.
				try {
					const d = JSON.parse(sessionStorage.getItem(draftKey) ?? 'null');
					if (d?.version === version) {
						bricks = d.bricks;
						restored = d.via !== 'chat';
					} else sessionStorage.removeItem(draftKey);
				} catch {
					/* no draft */
				}
			});
	});
	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/places', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => (places = data?.places ?? []));
		api
			.GET('/workspaces/{workspaceId}/lists', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => (lists = data ?? []));
		api
			.GET('/workspaces/{workspaceId}', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => (about = data?.profile ?? {}));
	});

	const stack = $derived<Stack>({ ...(base ?? { schema: 1 }), schema: 1, bricks });
	const dirty = $derived(!!base && JSON.stringify(base.bricks) !== JSON.stringify(bricks));
	// Unsaved edits are kept until Save or Cancel.
	let restored = $state(false);
	function draft(via = '') {
		try {
			if (dirty) sessionStorage.setItem(draftKey, JSON.stringify({ via, version, bricks }));
			else sessionStorage.removeItem(draftKey);
		} catch {
			/* private mode */
		}
	}
	$effect(() => {
		if (base) draft();
	});
	const keepDraft = () => draft('chat');
	function dropDraft() {
		try {
			sessionStorage.removeItem(draftKey);
		} catch {
			/* private mode */
		}
	}
	// Cancel: a draft goes away (nothing was saved), else back to the helper.
	async function cancel(e: MouseEvent) {
		dropDraft();
		if (!isDraft) return; // the link goes to the helper page
		e.preventDefault();
		await api.DELETE('/helpers/{helperId}', { params: { path: { helperId: id } } });
		if (history.length > 1) history.back();
		else goto('/helpers', { replaceState: true });
	}
	function discard() {
		if (base) bricks = structuredClone($state.snapshot(base.bricks)) as StackBrick[];
		restored = false;
	}

	// Live checks, the same as the server's, with one-tap fixes (#41).
	const issues = $derived(validate(stack));
	// About me facts a brick uses but that aren't set yet: the run would
	// stop there, so it's an error here (the sheet can set them).
	const unset = $derived(
		bricks.flatMap((b, index) =>
			[...JSON.stringify(b.config).matchAll(/\{my ([a-z_]+)\}/g)]
				.map((m) => m[1])
				.filter((k) => catalog.profile.some((f) => f.key === k) && !String(profile[k] ?? '').trim())
				.slice(0, 1)
				.map((k): Issue => ({
					brick: b.id,
					index,
					level: 'error',
					code: 'unset_fact',
					field: k,
					message: ''
				}))
		)
	);
	// A brick that works on cards with nothing before it to make any.
	const noCards = $derived(
		bricks.flatMap((b, index): Issue[] =>
			starved(bricks, index)
				? [{ brick: b.id, index, level: 'hint', code: 'no_cards', message: '' }]
				: []
		)
	);
	const errors = $derived([...issues.filter((i) => i.level === 'error'), ...unset]);
	function issueFor(b: StackBrick): Issue | Schemas['StackIssue'] | undefined {
		return (
			issues.find((i) => i.brick === b.id && i.level === 'error') ??
			unset.find((i) => i.brick === b.id) ??
			noCards.find((i) => i.brick === b.id) ??
			serverIssues.find((i) => i.brick === b.id) ??
			issues.find((i) => i.brick === b.id) ??
			(ready.has(b.type)
				? undefined
				: { brick: b.id, index: -1, level: 'hint', code: 'not_available', message: '' })
		);
	}
	function issueText(i: Issue | Schemas['StackIssue']): string {
		switch (i.code) {
			case 'missing_setting': {
				// Name what's missing ("Tap to set: Which list").
				const sbType = bricks.find((x) => x.id === i.brick)?.type;
				const label = brick(sbType ?? '')?.settings.find((x) => x.key === i.setting)?.label[lang];
				return label ? t('builder.issue.missing', { what: label }) : t('builder.issue.setting');
			}
			case 'bad_value':
			case 'bad_pill':
				return t('builder.issue.setting');
			case 'unknown_profile':
				return t('builder.issue.profile');
			case 'unset_fact':
				return t('builder.issue.unset', { fact: t(`pill.${i.field}` as Key) });
			case 'start_first':
			case 'start_only_first':
				return t('builder.issue.start');
			case 'missing_field':
				return t('builder.issue.field', {
					field: catalog.fields.find((f) => f.key === i.field)?.label[lang] ?? i.field ?? ''
				});
			case 'check_required':
				return t('builder.issue.check');
			case 'check_suggested':
				return t('builder.issue.checkMaybe');
			case 'not_available':
				return t('builder.issue.later');
			case 'no_cards':
				return t('builder.issue.noCards');
		}
		return i.message;
	}

	// A fix we can offer: the brick it adds has to run today.
	function fixOf(i: Issue | Schemas['StackIssue']) {
		const f = 'fix' in i ? i.fix : undefined;
		return f && ready.has(f.insert) ? f : undefined;
	}
	function fix(i: Issue) {
		const f = fixOf(i);
		if (!f) return;
		const r = applyFix(bricks, f);
		bricks = r.bricks;
		// A brick that still needs a setting opens right away.
		const added = bricks.find((x) => x.id === r.id);
		if (
			added &&
			validate({ ...stack, bricks }).some((x) => x.brick === r.id && x.level === 'error')
		)
			open(added);
	}
	const settingCodes = [
		'missing_setting',
		'bad_value',
		'bad_pill',
		'unknown_profile',
		'unset_fact'
	];

	// Sentences with known About me facts and chat names filled in.
	const profile = $derived({ ...about, ...(base?.profile ?? {}) });
	function title(b: StackBrick): string {
		let s = fillProfile(sentence(b, lang), profile);
		for (const p of places) s = s.replaceAll(p.id, p.label);
		for (const l of lists) s = s.replaceAll(l.id, l.name);
		return s;
	}

	const flip = 150;
	function consider(e: CustomEvent<DndEvent<StackBrick>>) {
		bricks = e.detail.items;
	}
	function finalize(e: CustomEvent<DndEvent<StackBrick>>) {
		bricks = startOnTop(e.detail.items); // a When brick stays on top
	}

	function open(b: StackBrick) {
		editing = b.id;
		sheetOpen = true;
	}
	function pick(type: string) {
		const r = add(bricks, type);
		bricks = r.bricks;
		trayOpen = false;
		open(bricks.find((b) => b.id === r.id)!);
	}
	function remove() {
		bricks = bricks.filter((b) => b.id !== editing);
		sheetOpen = false;
	}
	const editIndex = $derived(bricks.findIndex((b) => b.id === editing));
	// Pills for the open sheet: known About me facts, then what the bricks
	// above give (card fields, {answer}).
	const pills = $derived.by((): Pill[] => {
		if (editIndex < 0) return [];
		const here = available(stack, editIndex);
		const owner = session.workspace?.role === 'owner';
		const mine: Pill[] = catalog.profile.flatMap((f) => {
			const v = profile[f.key];
			if (typeof v === 'string' && v.trim()) return [{ raw: `my ${f.key}`, label: v }];
			return owner ? [{ raw: `my ${f.key}`, label: t(`pill.${f.key}` as Key), unset: f.key }] : [];
		});
		const field = (k: string) => catalog.fields.find((f) => f.key === k)?.label[lang] ?? k;
		return [
			...mine.filter((p) => !p.unset),
			...here.context
				.filter((k) => k === 'answer')
				.map((k) => ({ raw: k, label: t('builder.answer') })),
			...here.fields
				.filter((f) => fieldPills.includes(f))
				.map((f) => ({ raw: f, label: field(f) })),
			...mine.filter((p) => p.unset)
		];
	});

	// "+ your offer" in a sheet: remembered in About me, for every helper.
	async function rememberFact(key: string, value: string): Promise<boolean> {
		const ws = session.workspace;
		if (!ws) return false;
		const { data } = await api.PATCH('/workspaces/{workspaceId}', {
			params: { path: { workspaceId: ws.id } },
			body: { profile: { [key]: value.trim() } }
		});
		if (data) about = data.profile ?? {};
		return !!data;
	}

	// Test (#42): runs this edit once, max 3 cards, without saving it.
	let testRun = $state<string | null>(null);
	let detail = $state<Schemas['HelperDetail'] | null>(null);
	let asking = $state(false);
	let answer = $state('');
	const question = $derived(
		bricks[0]?.type === 'when.tap' && typeof bricks[0].config.question === 'string'
			? bricks[0].config.question.trim()
			: ''
	);
	const run = $derived(
		testRun && detail?.helper.run?.id === testRun ? detail.helper.run : undefined
	);
	const finished = $derived(
		testRun ? detail?.runs.find((r) => r.id === testRun && !run) : undefined
	);
	const testing = $derived(!!testRun && !finished && run?.status !== 'waiting');
	// While it runs, the bricks light up like on the helper page.
	const live = $derived(
		testRun && detail
			? brickProgress(
					bricks.map((b) => ({ id: b.id, group: brick(b.type)?.group ?? 'do', title: '' })),
					run,
					detail.steps
				)
			: null
	);
	function stateOf(i: number): BrickState {
		if (!live) return 'idle';
		const step = detail?.steps?.find((s) => s.brick_id === bricks[i].id);
		if (run) return i < live.current ? 'done' : i === live.current ? 'running' : 'idle';
		return step?.status === 'failed' ? 'failed' : step ? 'done' : 'idle';
	}
	// Where the result ends up: always the Feed; a last Do brick also says
	// what it does with it ("Posted to “Team”").
	const ending = $derived.by(() => {
		const last = bricks[bricks.length - 1];
		if (!last || brick(last.type)?.group === 'when') return '';
		const place = places.find((p) => p.id === last.config.place)?.label;
		const list = lists.find((l) => l.id === last.config.list)?.name;
		if (last.type === 'do.chat' && place) return t('builder.endsChat', { name: place });
		if (last.type === 'do.list' && list) return t('builder.endsList', { name: list });
		return t('builder.endsFeed');
	});
	// Only When and Look bricks: it fetches things and does nothing with them.
	const onlyReads = $derived(
		bricks.some((b) => brick(b.type)?.group === 'look') &&
			bricks.every((b) => ['when', 'look'].includes(brick(b.type)?.group ?? ''))
	);

	// A step that failed and waits for its retry (so it doesn't look stuck).
	const retryingNow = $derived(
		retrying(
			run,
			detail?.steps,
			bricks.map((b) => b.id)
		)
	);
	async function loadTest() {
		const { data } = await api.GET('/helpers/{helperId}', { params: { path: { helperId: id } } });
		if (data) detail = data;
	}
	$effect(() => {
		if (!testRun) return;
		loadTest();
		return listen(loadTest, () => session.fresh(), ['run', 'step', 'reload']);
	});
	function test() {
		if (question && !asking) {
			asking = true;
			return;
		}
		startTest();
	}
	async function startTest() {
		asking = false;
		error = '';
		const {
			data,
			error: err,
			response
		} = await api.POST('/helpers/{helperId}/stack/test', {
			params: { path: { helperId: id } },
			body: { stack: stack as Schemas['Stack'], ...(question ? { answer } : {}) }
		});
		if (data) {
			testRun = data.run_id;
			detail = null;
			return;
		}
		if (response.status === 422) {
			serverIssues = (err as Schemas['StackProblems']).issues;
			error = t('builder.problems');
		} else if (response.status === 409) error = t('builder.busy');
		else error = t('builder.failed');
	}

	async function save() {
		saving = true;
		error = '';
		// The name first: a new email address is made from it on save.
		if (renamed) {
			const { response: r } = await api.PATCH('/helpers/{helperId}', {
				params: { path: { helperId: id } },
				body: { name: newName.trim() }
			});
			if (r.ok) name = newName.trim();
		}
		const {
			data,
			error: err,
			response
		} = await api.PUT('/helpers/{helperId}/stack', {
			params: { path: { helperId: id } },
			body: { stack: stack as Schemas['Stack'], version }
		});
		saving = false;
		if (data) {
			dropDraft();
			goto(`/helpers/${id}`, { replaceState: true });
			return;
		}
		if (response.status === 422) {
			serverIssues = (err as Schemas['StackProblems']).issues;
			error = t('builder.problems');
		} else if (response.status === 409) error = t('builder.stale');
		else error = t('builder.failed');
	}
</script>

{#if missing}
	<p class="empty">{t('builder.missing')}</p>
{:else}
	<PageHeader title={isDraft ? t('builder.newTitle') : t('builder.title')}>
		{#snippet right()}
			<div class="head-actions">
				<a class="cancel" href="/helpers/{id}" onclick={cancel}>{t('helper.cancel')}</a>
				<button
					type="button"
					class="btn primary"
					disabled={(!dirty && !isDraft && !renamed) || errors.length > 0 || saving}
					onclick={save}>{saving ? t('builder.saving') : t('builder.save')}</button
				>
			</div>
		{/snippet}
	</PageHeader>
	{#if name}
		<label class="name-field">
			<span>{t('builder.name')}</span>
			<input bind:value={newName} maxlength="80" />
		</label>
	{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}
	{#if restored && dirty}
		<p class="restored">
			{t('builder.restored')}
			<button type="button" onclick={discard}>{t('builder.discard')}</button>
		</p>
	{/if}

	{#if base}
		<ol
			class="stack"
			aria-label={t('helper.bricks')}
			use:dndzone={{
				items: bricks,
				flipDurationMs: flip,
				dropTargetStyle: {},
				dragDisabled: testing
			}}
			onconsider={consider}
			onfinalize={finalize}
		>
			{#each bricks as b, i (b.id)}
				{@const issue = issueFor(b)}
				{@const out = lastOut[b.id]}
				<li class:dim={testing && live && i > live.current}>
					<Brick
						group={brick(b.type)?.group ?? 'do'}
						title={title(b)}
						state={stateOf(i)}
						sub={live
							? live.items[i].sub
							: out !== undefined && brick(b.type)?.shape !== 'start'
								? t('helper.cards', { n: out })
								: ''}
						onclick={testing ? undefined : () => open(b)}
					/>
					{#if issue}
						{@const f = fixOf(issue)}
						<div class="issue" class:hint={issue.level === 'hint'}>
							{#if settingCodes.includes(issue.code)}
								<button type="button" class="note" onclick={() => open(b)}
									>{issueText(issue)}</button
								>
							{:else}
								<span>{issueText(issue)}</span>
							{/if}
							{#if issue.code === 'missing_field' && issue.field}
								{@const sf = settingFix(bricks, b.id, issue.field)}
								{#if sf}
									<button
										type="button"
										class="fix"
										onclick={() =>
											(bricks = bricks.map((x) =>
												x.id === b.id ? { ...x, config: { ...x.config, [sf.key]: sf.value } } : x
											))}>{t('builder.useOption', { option: sf.label[lang] })}</button
									>
								{/if}
							{/if}
							{#if f}
								<button type="button" class="fix" onclick={() => fix(issue as Issue)}
									>+ {brick(f.insert)?.title[lang]}</button
								>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ol>
		{#if ending}<p class="ending">{ending}</p>{/if}
		{#if onlyReads}
			<div class="hint-next">
				<p>{t('builder.onlyReads')}</p>
				<div class="two">
					<button type="button" class="swatch think" onclick={() => openTray('think')}
						>+ {t('group.think')}</button
					>
					<button type="button" class="swatch do" onclick={() => openTray('do')}
						>+ {t('group.do')}</button
					>
				</div>
			</div>
		{/if}
		{#if !bricks.length}<p class="empty">{t('builder.empty')}</p>{/if}
	{:else}
		<p class="empty">{t('loading')}</p>
	{/if}

	<div class="dock">
		{#if testRun}
			<p class="result" role="status">
				{#if run?.status === 'waiting'}
					{t('builder.testWaits')} <a href="/feed">{t('tab.feed')} →</a>
				{:else if finished?.status === 'done'}
					{t('builder.testDone', { n: finished.cards_out ?? 0 })}
					<a href="/feed">{t('tab.feed')} →</a>
				{:else if finished}
					{t('builder.testFailed')} <a href="/feed">{t('tab.feed')} →</a>
				{:else}
					<RunTicker {run} retrying={retryingNow} slow />
				{/if}
			</p>
			<!-- A test doesn't save: say so once it's back, and offer to. -->
			{#if dirty && (finished || run?.status === 'waiting')}
				<div class="unsaved">
					<p>{t('builder.testNotSaved')}</p>
					<button
						type="button"
						class="btn primary"
						disabled={errors.length > 0 || saving}
						onclick={save}>{t('builder.save')}</button
					>
				</div>
			{/if}
		{/if}
		<button
			type="button"
			class="btn wide"
			disabled={!base || errors.length > 0 || testing}
			onclick={test}>{testing ? t('builder.testing') : t('builder.test')}</button
		>
		<div class="tray" role="group" aria-label={t('builder.tray')}>
			{#each groups as g (g)}
				<button
					type="button"
					class="swatch {g}"
					onclick={() => {
						trayGroup = g;
						trayOpen = true;
					}}>{t(`group.${g}` as Key)}</button
				>
			{/each}
		</div>
	</div>

	{#if editing && editIndex >= 0}
		<BrickSheet
			bind:open={sheetOpen}
			bind:sb={bricks[editIndex]}
			{places}
			bind:lists
			lastOut={lastOut[editing]}
			{emailAddress}
			{pills}
			onfact={rememberFact}
			onleave={keepDraft}
			back="/helpers/{id}/edit"
			onremove={remove}
		/>
	{/if}

	<BottomSheet bind:open={asking} title={t('builder.test')}>
		<form
			class="answer"
			onsubmit={(e) => {
				e.preventDefault();
				startTest();
			}}
		>
			<label for="test-answer">{question}</label>
			<input id="test-answer" bind:value={answer} maxlength="500" autocomplete="off" />
			<button type="submit" class="btn primary wide" disabled={!answer.trim()}
				>{t('builder.test')}</button
			>
		</form>
	</BottomSheet>

	<BottomSheet bind:open={trayOpen} title={trayGroup ? t(`group.${trayGroup}` as Key) : ''}>
		<ul class="pick">
			{#each trayGroup ? tray(trayGroup) : [] as b (b.id)}
				<li>
					{#if ready.has(b.id)}
						<Brick
							group={b.group}
							title={b.title[lang]}
							sub={b.does[lang]}
							onclick={() => pick(b.id)}
						/>
					{:else}
						<div class="later">
							<strong>{b.title[lang]}</strong>
							<span>{t('places.soon')}</span>
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	</BottomSheet>
{/if}

<style>
	.lead {
		margin: 0;
		padding: 0 var(--gutter) var(--space-3);
		color: var(--muted);
		font-weight: 600;
	}
	.restored {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		margin: 0 var(--gutter) var(--space-3);
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-control);
		background: var(--gray-50);
		font-weight: 600;
	}
	.restored button {
		min-height: var(--tap-min);
		padding: 0;
		border: 0;
		background: none;
		font: inherit;
		font-weight: 800;
		text-decoration: underline;
		cursor: pointer;
	}
	.dim {
		opacity: 0.45;
	}
	.result {
		margin: 0;
		font-weight: 700;
	}
	.result a {
		color: inherit;
	}
	.answer {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.answer label {
		font-weight: 700;
	}
	.answer input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
	}
	.head-actions {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.head-actions .btn {
		min-height: var(--tap-min);
		padding: 0 var(--space-4);
	}
	.cancel {
		color: inherit;
		font-weight: 700;
		text-decoration: none;
	}
	.stack {
		list-style: none;
		margin: 0;
		padding: 0 var(--gutter) 240px;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.stack:focus-visible {
		outline: 2px solid var(--ink);
	}
	.issue {
		margin: var(--space-1) 0 var(--space-2);
		color: var(--danger);
		font-size: var(--text-sm);
		font-weight: 700;
	}
	.issue.hint {
		color: var(--muted);
	}
	.issue {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}
	.note {
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		text-decoration: underline;
		cursor: pointer;
	}
	.fix {
		min-height: 36px;
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		color: var(--ink);
		font: inherit;
		font-weight: 800;
		cursor: pointer;
	}
	.empty {
		padding: 0 var(--gutter);
		color: var(--muted);
	}
	.dock {
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
	.tray {
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		gap: var(--space-1);
	}
	.swatch {
		min-height: var(--tap-min);
		padding: 0 var(--space-1);
		border: 0;
		border-radius: var(--radius-control);
		box-shadow: var(--brick-edge);
		font-weight: 800;
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.swatch:active {
		transform: scale(var(--press-scale));
	}
	.swatch.when {
		background: var(--brick-yellow);
		color: var(--on-yellow);
	}
	.swatch.look {
		background: var(--brick-blue);
		color: var(--on-blue);
	}
	.swatch.think {
		background: var(--brick-purple);
		color: var(--on-purple);
	}
	.swatch.do {
		background: var(--brick-green);
		color: var(--on-green);
	}
	.swatch.ask {
		background: var(--brick-red);
		color: var(--on-red);
	}
	.error {
		margin: 0;
		padding: 0 var(--gutter) var(--space-3);
		color: var(--danger);
		font-weight: 600;
	}
	.pick {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.later {
		display: flex;
		justify-content: space-between;
		align-items: center;
		min-height: 56px;
		padding: var(--space-3) var(--space-4);
		border: 2px solid var(--gray-200);
		border-radius: var(--radius-control);
		color: var(--muted);
	}
	.unsaved {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin: 0 0 var(--space-2);
	}
	.unsaved p {
		flex: 1;
		margin: 0;
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.name-field {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		padding: 0 var(--gutter) var(--space-3);
	}
	.name-field span {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.name-field input {
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-weight: 700;
	}
	.ending {
		margin: var(--space-2) var(--gutter) 0;
		color: var(--muted);
		font-weight: 600;
	}
	.hint-next {
		margin: var(--space-3) var(--gutter) 0;
		padding: var(--space-3);
		border: 2px dashed var(--muted);
		border-radius: var(--radius-control);
	}
	.hint-next p {
		margin: 0 0 var(--space-2);
		font-size: var(--text-sm);
	}
	.hint-next .two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
</style>
