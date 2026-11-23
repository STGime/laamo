<!-- S11 Helper page: on/off, rename, run now (or with a photo), its bricks, recent runs, delete. -->
<script lang="ts">
	import HelperIcon from '#lib/components/HelperIcon.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import BrickStack, { type StackItem } from '#lib/components/BrickStack.svelte';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { listen } from '#lib/feed.svelte.ts';
	import { brickProgress, busy as running, retrying } from '#lib/progress.ts';
	import RunTicker from '#lib/components/RunTicker.svelte';
	import HelperAddress from '#lib/components/HelperAddress.svelte';
	import { api, session } from '#lib/session.svelte.ts';
	import { shrink } from '#lib/photo.ts';
	import { ago, when } from '#lib/time.ts';

	let detail = $state<Schemas['HelperDetail'] | null>(null);
	let missing = $state(false);
	let renaming = $state(false);
	let name = $state('');
	let confirmDelete = $state(false);
	let busy = $state(false);
	// A run started from this page: first "starting" (until the page has
	// loaded it), then we follow it and say how it ended.
	let after = $state<'none' | 'starting' | 'following'>('none');
	let startedRun = $state('');
	let forgetting = $state(false);
	let forgot = $state(false);

	const id = $derived(page.params.id ?? '');
	// Changes made in the builder but not saved (kept in this tab).
	let unsaved = $state(false);
	$effect(() => {
		try {
			unsaved = !!sessionStorage.getItem(`laamo.edit.${id}`);
		} catch {
			unsaved = false;
		}
	});

	async function load() {
		const { data, response } = await api.GET('/helpers/{helperId}', {
			params: { path: { helperId: id } }
		});
		if (response.status === 404) missing = true;
		if (data) detail = data;
	}
	// The run view is live: run and step changes reload the page's data.
	$effect(() => {
		if (!id) return;
		load();
		return listen(load, () => session.fresh(), ['run', 'step', 'reload']);
	});

	const helper = $derived(detail?.helper);
	const on = $derived(helper?.status === 'active');
	const run = $derived(helper?.run);
	// While a run is going, bricks light up (see brickProgress).
	const view = $derived(
		brickProgress(
			(detail?.bricks ?? []).map((b) => ({ id: b.id, group: b.group, title: b.sentence })),
			run,
			detail?.steps
		)
	);
	const bricks = $derived(view.items as StackItem[]);
	// How the run started here ended ('' while it's going or unknown).
	const ended = $derived.by(() => {
		if (after !== 'following' || running(run)) return '';
		if (run?.id === startedRun) return 'waiting'; // not running, so it waits for the user
		const status = detail?.runs.find((r) => r.id === startedRun)?.status;
		return status === 'done' || status === 'failed' || status === 'stopped' ? status : '';
	});
	const current = $derived(view.current);

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
		if (data) follow(data.run_id);
	}

	async function follow(runId: string) {
		startedRun = runId;
		after = 'starting';
		await load();
		after = 'following';
	}

	// The camera button (helpers with Read the photo): a photo is shrunk
	// here and uploaded, and the run starts with it. A long receipt can be
	// taken in several photos (parts), which are read as one.
	const MAX_PARTS = 5;
	let photoState = $state<'idle' | 'sending' | 'failed'>('idle');
	let several = $state(false);
	let parts = $state<{ path: string; url: string }[]>([]);

	async function upload(file: Blob): Promise<{ path: string; url: string }> {
		const jpeg = await shrink(file);
		const up = await api.POST('/helpers/{helperId}/photos', {
			params: { path: { helperId: id } },
			body: jpeg as unknown as string,
			bodySerializer: (b) => b,
			headers: { 'Content-Type': 'image/jpeg' }
		});
		if (!up.data) throw new Error('upload');
		return { path: up.data.path, url: URL.createObjectURL(jpeg) };
	}

	async function startWith(paths: string[]) {
		const { data } = await api.POST('/helpers/{helperId}/runs', {
			params: { path: { helperId: id } },
			body: { photos: paths, photos_one: paths.length > 1 }
		});
		if (!data) throw new Error('start');
		follow(data.run_id);
	}

	function dropParts() {
		for (const p of parts) URL.revokeObjectURL(p.url);
		parts = [];
		several = false;
	}

	// One photo: taken, sent, and the run starts. In "several" mode the
	// photos pile up as parts until Read it.
	async function photosPicked(e: Event & { currentTarget: HTMLInputElement }) {
		const input = e.currentTarget;
		const files = [...(input.files ?? [])].slice(0, MAX_PARTS - parts.length);
		input.value = ''; // the same picture can be picked again
		if (!files.length) return;
		photoState = 'sending';
		busy = true;
		try {
			for (const file of files) {
				const part = await upload(file);
				if (several) parts = [...parts, part];
				else {
					URL.revokeObjectURL(part.url);
					await startWith([part.path]);
				}
			}
			photoState = 'idle';
		} catch {
			photoState = 'failed';
		} finally {
			busy = false;
		}
	}

	async function readParts() {
		photoState = 'sending';
		busy = true;
		try {
			await startWith(parts.map((p) => p.path));
			dropParts();
			photoState = 'idle';
		} catch {
			photoState = 'failed';
		} finally {
			busy = false;
		}
	}

	// Learning from swipes (#76): what Let me pick taught Keep the good ones.
	async function forget() {
		if (!forgetting) {
			forgetting = true;
			return;
		}
		busy = true;
		const { response } = await api.DELETE('/helpers/{helperId}/learned', {
			params: { path: { helperId: id } }
		});
		busy = false;
		forgetting = false;
		if (response.ok && detail) {
			detail = { ...detail, learned: 0 };
			forgot = true;
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
		<span class="avatar" aria-hidden="true"><HelperIcon avatar={helper.avatar} size={56} /></span>
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
					})}{:else if helper.email_address}{t('helper.onEmail')}{:else}{t(
						'helper.noSchedule'
					)}{/if}
			</p>
		</div>
		{#if helper.email_address}<HelperAddress
				address={helper.email_address}
				forward={helper.forward_address}
				helperId={helper.id}
			/>{/if}
		{#if detail?.takes_photos}
			{@const off = busy || running(run)}
			{#snippet camera()}
				<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"
					><path
						fill="currentColor"
						fill-rule="evenodd"
						d="M8.5 4h7l1.5 2.5H21V20H3V6.5h4zM12 9a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 11a2 2 0 1 1 0 4 2 2 0 0 1 0-4z"
					/></svg
				>
			{/snippet}
			{#if run && running(run)}
				<!-- A photo is this helper's input: the camera comes first. -->
				<div class="btn primary wide off">
					<RunTicker
						{run}
						retrying={retrying(
							run,
							detail?.steps,
							(detail?.bricks ?? []).map((b) => b.id)
						)}
					/>
				</div>
			{:else if photoState === 'sending' || after === 'starting'}
				<div class="btn primary wide off">
					<RunTicker waiting={photoState === 'sending' ? t('photo.sending') : ''} />
				</div>
			{:else if several}
				<!-- A long receipt, part by part. -->
				<p class="hint">{t('photo.partsHow', { n: MAX_PARTS })}</p>
				{#if parts.length}
					<ol class="parts">
						{#each parts as p, i (p.path)}
							<li><img src={p.url} alt={t('photo.part', { n: i + 1 })} /></li>
						{/each}
					</ol>
				{/if}
				{#if parts.length < MAX_PARTS}
					<label class="btn wide photo" class:primary={!parts.length}>
						<input
							type="file"
							accept="image/*"
							capture="environment"
							class="visually-hidden"
							onchange={photosPicked}
						/>
						{@render camera()}
						{t('photo.takePart', { n: parts.length + 1 })}
					</label>
					<label class="choose">
						<input
							type="file"
							accept="image/*"
							multiple
							class="visually-hidden"
							onchange={photosPicked}
						/>
						{t('photo.chooseParts')}
					</label>
				{/if}
				{#if parts.length}
					<button class="btn primary wide" onclick={readParts}
						>{t(parts.length > 1 ? 'photo.readParts' : 'photo.readOne', {
							n: parts.length
						})}</button
					>
				{/if}
				<button class="choose plain" onclick={dropParts}>{t('helper.cancel')}</button>
			{:else}
				<label class="btn primary wide photo" class:off>
					<input
						type="file"
						accept="image/*"
						capture="environment"
						class="visually-hidden"
						disabled={off}
						onchange={photosPicked}
					/>
					{@render camera()}
					{t('photo.take')}
				</label>
				<label class="choose" class:off>
					<input
						type="file"
						accept="image/*"
						class="visually-hidden"
						disabled={off}
						onchange={photosPicked}
					/>
					{t('photo.choose')}
				</label>
				<!-- Small print reads best close up: a long receipt in parts. -->
				<p class="hint">
					{t('photo.longHint')}
					<button class="link" disabled={off} onclick={() => (several = true)}
						>{t('photo.several')}</button
					>
				</p>
			{/if}
			{#if photoState === 'failed'}<p class="started" role="alert">{t('photo.failed')}</p>{/if}
		{:else}
			<button class="btn primary wide" disabled={busy || running(run)} onclick={runNow}
				>{#if run}<RunTicker
						{run}
						retrying={retrying(
							run,
							detail?.steps,
							(detail?.bricks ?? []).map((b) => b.id)
						)}
					/>{:else if after === 'starting'}<RunTicker />{:else}{t('helpers.runNow')}{/if}</button
			>
		{/if}
		{#if ended === 'waiting'}
			<p class="started">{t('helpers.waiting')}. <a href="/feed">{t('tab.feed')} →</a></p>
		{:else if ended === 'done'}
			<p class="started">{t('helpers.finished')} <a href="/feed">{t('tab.feed')} →</a></p>
		{:else if ended}
			<p class="started failed" role="alert">
				{t('helpers.runFailed')} <a href="/feed">{t('tab.feed')} →</a>
			</p>
		{/if}
	</header>

	<section>
		<h2>{t('helper.bricks')}</h2>
		{#if unsaved}
			<p class="unsaved">
				{t('helper.unsaved')} <a href="/helpers/{id}/edit">{t('helper.unsavedOpen')} →</a>
			</p>
		{/if}
		<BrickStack {bricks} {current} />
		<a class="btn wide edit" href="/helpers/{id}/edit">{t('builder.edit')}</a>
		{#if detail?.learned}
			<p class="learned">
				{t('helper.learned', { n: detail.learned })}
				<button class="link" class:sure={forgetting} disabled={busy} onclick={forget}
					>{forgetting ? t('helper.forgetSure') : t('helper.forget')}</button
				>
			</p>
		{:else if forgot}
			<p class="learned">{t('helper.forgotten')}</p>
		{/if}
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
						<span class="meta"
							>{r.cards_out != null ? `${t('helper.cards', { n: r.cards_out })} · ` : ''}{ago(
								r.created_at
							)} · €{r.cost_eur.toFixed(4)}</span
						>
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
	/* The camera button and "choose a picture" are labels around a file input. */
	.photo,
	.choose {
		cursor: pointer;
	}
	.photo:focus-within,
	.choose:focus-within {
		outline: 3px solid var(--focus);
		outline-offset: 2px;
	}
	.choose {
		align-self: center;
		display: inline-flex;
		align-items: center;
		min-height: var(--tap-min);
		font-weight: 600;
		text-decoration: underline;
	}
	.off {
		opacity: 0.5;
		cursor: default;
		pointer-events: none;
	}
	.choose.plain {
		border: 0;
		background: none;
		font: inherit;
		font-weight: 600;
		color: inherit;
	}
	.parts {
		display: flex;
		gap: var(--space-2);
		margin: 0;
		padding: 0;
		list-style: none;
		overflow-x: auto;
	}
	.parts img {
		display: block;
		width: 72px;
		height: 96px;
		object-fit: cover;
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
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
	.failed {
		color: var(--danger);
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
	.learned {
		margin-top: var(--space-2);
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.link {
		background: none;
		border: 0;
		padding: 0;
		color: var(--ink);
		font: inherit;
		text-decoration: underline;
		cursor: pointer;
	}
	.link.sure {
		color: var(--danger);
	}
	.note {
		padding: var(--space-6) var(--gutter);
		color: var(--muted);
	}
	.unsaved {
		margin: 0 0 var(--space-3);
		padding: var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font-size: var(--text-sm);
		font-weight: 600;
	}
</style>
