<!--
  S9 Brick builder: the helper's bricks, dragged into order. A color in the
  tray shows its bricks; a brick opens its sheet. Save makes a new version.
-->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { dndzone, type DndEvent } from 'svelte-dnd-action';
	import { brick, type GroupId, type Stack, type StackBrick } from '@laamo/catalog';
	import { sentence } from '@laamo/catalog/describe';
	import { available, validate, type Issue } from '@laamo/catalog/validate';
	import type { Schemas } from '#lib/api/client.ts';
	import Brick from '#lib/components/Brick.svelte';
	import BrickSheet from '#lib/components/BrickSheet.svelte';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { add, fieldPills, fillProfile, groups, ready, startOnTop, tray } from '#lib/builder.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	const id = $derived(page.params.id ?? '');
	const lang = $derived(i18n.lang);

	let name = $state('');
	let version = $state(0);
	let base = $state<Stack | null>(null); // as loaded, for "anything changed?"
	let bricks = $state<StackBrick[]>([]);
	let lastOut = $state<Record<string, number>>({});
	let places = $state<Schemas['Place'][]>([]);
	let about = $state<Record<string, unknown>>({});
	let missing = $state(false);
	let saving = $state(false);
	let error = $state('');
	let serverIssues = $state<Schemas['StackIssue'][]>([]);

	let editing = $state<string | null>(null); // brick id whose sheet is open
	let sheetOpen = $state(false);
	let trayGroup = $state<GroupId | null>(null);
	let trayOpen = $state(false);

	// An unsaved edit survives a trip to "Connect a chat".
	const draftKey = $derived(`laamo.edit.${id}`);

	$effect(() => {
		if (!id) return;
		api.GET('/helpers/{helperId}', { params: { path: { helperId: id } } }).then(({ data }) => {
			if (data) name = data.helper.name;
		});
		api
			.GET('/helpers/{helperId}/stack', { params: { path: { helperId: id } } })
			.then(({ data, response }) => {
				if (response.status === 404) missing = true;
				if (!data) return;
				version = data.version;
				base = data.stack as Stack;
				lastOut = data.last_out;
				bricks = structuredClone(data.stack.bricks) as StackBrick[];
				try {
					const d = JSON.parse(sessionStorage.getItem(draftKey) ?? 'null');
					if (d?.version === version) bricks = d.bricks;
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
			.GET('/workspaces/{workspaceId}', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => (about = data?.profile ?? {}));
	});

	const stack = $derived<Stack>({ ...(base ?? { schema: 1 }), schema: 1, bricks });
	const dirty = $derived(!!base && JSON.stringify(base.bricks) !== JSON.stringify(bricks));
	$effect(() => {
		if (!base) return;
		try {
			if (dirty) sessionStorage.setItem(draftKey, JSON.stringify({ version, bricks }));
			else sessionStorage.removeItem(draftKey);
		} catch {
			/* private mode */
		}
	});

	// Live checks, the same as the server's (#41 adds one-tap fixes).
	const issues = $derived(validate(stack));
	const errors = $derived(issues.filter((i) => i.level === 'error'));
	function issueFor(b: StackBrick): Issue | Schemas['StackIssue'] | undefined {
		return (
			issues.find((i) => i.brick === b.id && i.level === 'error') ??
			serverIssues.find((i) => i.brick === b.id) ??
			issues.find((i) => i.brick === b.id)
		);
	}
	function issueText(i: Issue | Schemas['StackIssue']): string {
		switch (i.code) {
			case 'missing_setting':
			case 'bad_value':
			case 'bad_pill':
				return t('builder.issue.setting');
			case 'unknown_profile':
				return t('builder.issue.profile');
			case 'start_first':
			case 'start_only_first':
				return t('builder.issue.start');
			case 'missing_field':
				return t('builder.issue.field', { field: i.field ?? '' });
			case 'check_required':
			case 'check_suggested':
				return t('builder.issue.check');
		}
		return i.message;
	}

	// Sentences with known About me facts and chat names filled in.
	const profile = $derived({ ...about, ...(base?.profile ?? {}) });
	function title(b: StackBrick): string {
		let s = fillProfile(sentence(b, lang), profile);
		for (const p of places) s = s.replaceAll(p.id, p.label);
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
	const pills = $derived.by(() => {
		if (editIndex < 0) return [];
		const here = available(stack, editIndex);
		const mine = Object.entries(profile)
			.filter(([, v]) => typeof v === 'string' && v.trim())
			.map(([k]) => `my ${k}`);
		return [...mine, ...here.context.filter((k) => k === 'answer'), ...here.fields.filter((f) => fieldPills.includes(f))];
	});

	async function save() {
		saving = true;
		error = '';
		const { data, error: err, response } = await api.PUT('/helpers/{helperId}/stack', {
			params: { path: { helperId: id } },
			body: { stack: stack as Schemas['Stack'], version }
		});
		saving = false;
		if (data) {
			try {
				sessionStorage.removeItem(draftKey);
			} catch {
				/* private mode */
			}
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
	<PageHeader title={t('builder.title')}>
		{#snippet right()}<a class="cancel" href="/helpers/{id}">{t('helper.cancel')}</a>{/snippet}
	</PageHeader>
	{#if name}<p class="lead">{name}</p>{/if}

	{#if base}
		<ol
			class="stack"
			aria-label={t('helper.bricks')}
			use:dndzone={{ items: bricks, flipDurationMs: flip, dropTargetStyle: {} }}
			onconsider={consider}
			onfinalize={finalize}
		>
			{#each bricks as b (b.id)}
				{@const issue = issueFor(b)}
				{@const out = lastOut[b.id]}
				<li>
					<Brick
						group={brick(b.type)?.group ?? 'do'}
						title={title(b)}
						sub={out !== undefined && brick(b.type)?.shape !== 'start'
							? t('helper.cards', { n: out })
							: ''}
						onclick={() => open(b)}
					/>
					{#if issue}
						<p class="issue" class:hint={issue.level === 'hint'}>{issueText(issue)}</p>
					{/if}
				</li>
			{/each}
		</ol>
		{#if !bricks.length}<p class="empty">{t('builder.empty')}</p>{/if}
		{#if !errors.length && issues.length === 0 && !bricks.some((b) => !ready.has(b.type))}
			<p class="ok">{t('builder.ok')}</p>
		{/if}
	{:else}
		<p class="empty">{t('loading')}</p>
	{/if}

	<div class="dock">
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
		{#if error}<p class="error" role="alert">{error}</p>{/if}
		<button
			type="button"
			class="btn primary wide"
			disabled={!dirty || errors.length > 0 || saving}
			onclick={save}>{saving ? t('builder.saving') : t('builder.save')}</button
		>
	</div>

	{#if editing && editIndex >= 0}
		<BrickSheet
			bind:open={sheetOpen}
			bind:sb={bricks[editIndex]}
			{places}
			lastOut={lastOut[editing]}
			{pills}
			back="/helpers/{id}/edit"
			onremove={remove}
		/>
	{/if}

	<BottomSheet bind:open={trayOpen} title={trayGroup ? t(`group.${trayGroup}` as Key) : ''}>
		<ul class="pick">
			{#each trayGroup ? tray(trayGroup) : [] as b (b.id)}
				<li>
					{#if ready.has(b.id)}
						<Brick group={b.group} title={b.title[lang]} sub={b.does[lang]} onclick={() => pick(b.id)} />
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
	.cancel {
		color: inherit;
		font-weight: 700;
		text-decoration: none;
	}
	.stack {
		list-style: none;
		margin: 0;
		padding: 0 var(--gutter) 260px;
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
	.ok,
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
</style>
