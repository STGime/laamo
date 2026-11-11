<!-- S11 Helper page: on/off, rename, run now, its bricks, recent runs, delete. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import BrickStack, { type StackItem } from '#lib/components/BrickStack.svelte';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api } from '#lib/session.svelte.ts';
	import { ago, when } from '#lib/time.ts';

	let detail = $state<Schemas['HelperDetail'] | null>(null);
	let missing = $state(false);
	let renaming = $state(false);
	let name = $state('');
	let confirmDelete = $state(false);
	let busy = $state(false);
	let started = $state(false);

	const id = $derived(page.params.id ?? '');

	async function load() {
		const { data, response } = await api.GET('/helpers/{helperId}', {
			params: { path: { helperId: id } }
		});
		if (response.status === 404) missing = true;
		if (data) detail = data;
	}
	$effect(() => {
		if (id) load();
	});

	const helper = $derived(detail?.helper);
	const on = $derived(helper?.status === 'active');
	const bricks = $derived<StackItem[]>(
		(detail?.bricks ?? []).map((b) => ({ id: b.id, group: b.group, title: b.sentence }))
	);

	async function patch(body: { name?: string; status?: 'active' | 'paused' }) {
		busy = true;
		const { data } = await api.PATCH('/helpers/{helperId}', {
			params: { path: { helperId: id } },
			body
		});
		busy = false;
		if (data && detail) detail = { ...detail, helper: data };
	}

	async function runNow() {
		busy = true;
		const { data } = await api.POST('/helpers/{helperId}/runs', {
			params: { path: { helperId: id } },
			body: {}
		});
		busy = false;
		if (data) {
			started = true;
			load();
		}
	}

	async function remove() {
		busy = true;
		const { response } = await api.DELETE('/helpers/{helperId}', {
			params: { path: { helperId: id } }
		});
		busy = false;
		if (response.ok) goto('/helpers', { replaceState: true });
	}
</script>

{#if missing}
	<p class="note">{t('feed.empty.title')}</p>
{:else if helper}
	<header>
		<span class="avatar" aria-hidden="true">{helper.avatar ?? '🧱'}</span>
		{#if renaming}
			<form
				class="rename"
				onsubmit={(e) => {
					e.preventDefault();
					if (name.trim()) patch({ name: name.trim() }).then(() => (renaming = false));
				}}
			>
				<input bind:value={name} maxlength="80" aria-label={t('helper.rename')} />
				<button class="btn primary" disabled={busy || !name.trim()}>{t('helper.save')}</button>
			</form>
		{:else}
			<button
				class="title"
				onclick={() => ((name = helper.name), (renaming = true))}
				aria-label={t('helper.rename')}
			>
				<h1>{helper.name}</h1>
				<span class="pen" aria-hidden="true">✎</span>
			</button>
		{/if}

		<div class="row">
			<button
				class="switch"
				role="switch"
				aria-checked={on}
				disabled={busy || helper.status === 'draft'}
				onclick={() => patch({ status: on ? 'paused' : 'active' })}
			>
				<span class="knob" aria-hidden="true"></span>
				{on ? t('helper.on') : t('helper.off')}
			</button>
			<p class="next">
				{#if !on}{t('helper.paused')}{:else if helper.next_run_at}{t('helper.next', {
						when: when(helper.next_run_at, i18n.lang)
					})}{:else}{t('helper.noSchedule')}{/if}
			</p>
		</div>
		<button class="btn primary wide" disabled={busy} onclick={runNow}>{t('helpers.runNow')}</button>
		{#if started}<p class="started">
				{t('helpers.started')} <a href="/feed">{t('tab.feed')} →</a>
			</p>{/if}
	</header>

	<section>
		<h2>{t('helper.bricks')}</h2>
		<BrickStack {bricks} />
		<p class="hint">{t('helper.editSoon')}</p>
	</section>

	<section>
		<h2>{t('helper.runs')}</h2>
		{#if detail?.runs.length}
			<ul class="runs">
				{#each detail.runs as r (r.id)}
					<li>
						<span class="dot {r.status}" aria-hidden="true"></span>
						<span class="what"
							>{t(`run.status.${r.status}` as Key)}{r.test ? ` · ${t('run.test')}` : ''}</span
						>
						<span class="meta">{ago(r.created_at)} · €{r.cost_eur.toFixed(4)}</span>
					</li>
				{/each}
			</ul>
		{:else}
			<p class="hint">{t('helper.noRuns')}</p>
		{/if}
	</section>

	<section>
		<button class="btn wide danger" onclick={() => (confirmDelete = true)}
			>{t('helper.delete')}</button
		>
	</section>

	<BottomSheet bind:open={confirmDelete} title={t('helper.deleteTitle', { name: helper.name })}>
		<p>{t('helper.deleteBody')}</p>
		{#snippet footer()}
			<div class="confirm">
				<button class="btn" onclick={() => (confirmDelete = false)}>{t('helper.cancel')}</button>
				<button class="btn danger-solid" disabled={busy} onclick={remove}
					>{t('helper.deleteYes')}</button
				>
			</div>
		{/snippet}
	</BottomSheet>
{:else}
	<p class="note">{t('loading')}</p>
{/if}

<style>
	header {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-6) var(--gutter) var(--space-4);
	}
	.avatar {
		font-size: 44px;
		line-height: 1;
	}
	.title {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: 0;
		border: 0;
		background: none;
		text-align: left;
		cursor: pointer;
	}
	.pen {
		color: var(--muted);
		font-size: var(--text-lg);
	}
	.rename {
		display: flex;
		gap: var(--space-2);
	}
	.rename input {
		flex: 1;
		min-width: 0;
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.switch {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		padding: 0 var(--space-3) 0 var(--space-1);
		border: 2px solid var(--ink);
		border-radius: 24px;
		background: var(--white);
		font-weight: 800;
		cursor: pointer;
	}
	.knob {
		width: 36px;
		height: 36px;
		border-radius: 18px;
		background: var(--gray-200);
		transition: background var(--motion-fast) var(--ease);
	}
	.switch[aria-checked='true'] .knob {
		background: var(--brick-green);
	}
	.next {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.started {
		margin: 0;
		font-weight: 600;
	}
	section {
		padding: var(--space-4) var(--gutter);
		border-top: var(--border);
	}
	h2 {
		margin-bottom: var(--space-3);
		font-size: var(--text-lg);
	}
	.hint {
		margin: var(--space-3) 0 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.runs {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.runs li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: 44px;
		border-bottom: var(--border);
	}
	.dot {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		background: var(--gray-200);
	}
	.dot.done {
		background: var(--brick-green);
	}
	.dot.failed,
	.dot.stopped {
		background: var(--brick-red);
	}
	.dot.running,
	.dot.queued {
		background: var(--brick-blue);
	}
	.dot.waiting {
		background: var(--brick-yellow);
	}
	.what {
		flex: 1;
		font-weight: 600;
	}
	.meta {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.danger {
		color: var(--danger);
	}
	.confirm {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
	.danger-solid {
		background: var(--danger);
		color: var(--white);
	}
	.note {
		padding: var(--space-6) var(--gutter);
		color: var(--muted);
	}
</style>
