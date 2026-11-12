<!--
  "Add to list" from the Feed (#47): saves some or all cards of a Feed card
  to one of your lists, or a new one. The server copies the cards from the
  run; duplicates are skipped.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	let {
		feedCardId,
		cardIds = [],
		links,
		quiet = false,
		ondone
	}: {
		feedCardId: string;
		/** The chosen cards; empty means all of them. */
		cardIds?: string[];
		/** A result's source links instead of cards: the chosen ones, or all. */
		links?: { chosen: string[]; all: string[] };
		/** A small text button instead of a big one (under Feed links). */
		quiet?: boolean;
		ondone?: () => void;
	} = $props();
	const chosenCount = $derived(links ? links.chosen.length : cardIds.length);

	let picking = $state(false);
	let lists = $state<Schemas['ListSummary'][]>([]);
	let name = $state('');
	let busy = $state(false);
	let done = $state<{ list: Schemas['ListSummary']; added: number; duplicates: number } | null>(
		null
	);
	let error = $state('');

	async function start() {
		const ws = session.workspace;
		if (!ws) return;
		picking = true;
		done = null;
		error = '';
		const { data } = await api.GET('/workspaces/{workspaceId}/lists', {
			params: { path: { workspaceId: ws.id } }
		});
		lists = data ?? [];
	}

	async function add(listId: string) {
		busy = true;
		error = '';
		const { data } = await api.POST('/lists/{listId}/rows', {
			params: { path: { listId } },
			body: links
				? { feed_card_id: feedCardId, links: links.chosen.length ? links.chosen : links.all }
				: { feed_card_id: feedCardId, card_ids: cardIds.length ? cardIds : undefined }
		});
		busy = false;
		if (!data) return (error = t('lists.failed'));
		done = data;
		picking = false;
		ondone?.();
	}

	async function createAndAdd(e: SubmitEvent) {
		e.preventDefault();
		const ws = session.workspace;
		if (!ws || !name.trim()) return;
		busy = true;
		error = '';
		const { data, response } = await api.POST('/workspaces/{workspaceId}/lists', {
			params: { path: { workspaceId: ws.id } },
			body: { name: name.trim() }
		});
		busy = false;
		if (!data) return (error = response.status === 409 ? t('lists.taken') : t('lists.failed'));
		name = '';
		await add(data.id);
	}
</script>

<div class="add">
	{#if done}
		<p class="done" role="status">
			{t('lists.added', { n: done.added, name: done.list.name })}{done.duplicates
				? ` ${t('lists.alreadyThere', { n: done.duplicates })}`
				: ''}
			<a href="/lists/{done.list.id}">{t('lists.open')} →</a>
		</p>
	{/if}
	{#if picking}
		<p class="label">{t('lists.pick')}</p>
		<div class="choices">
			{#each lists as l (l.id)}
				<button type="button" disabled={busy} onclick={() => add(l.id)}>{l.name}</button>
			{/each}
		</div>
		<form class="new" onsubmit={createAndAdd}>
			<input
				bind:value={name}
				maxlength="80"
				placeholder={t('lists.newName')}
				aria-label={t('lists.newName')}
			/>
			<button class="btn" disabled={busy || !name.trim()}>{t('lists.create')}</button>
		</form>
		<button class="btn wide" onclick={() => (picking = false)}>{t('helper.cancel')}</button>
	{:else}
		<button class={quiet ? 'quiet' : 'btn primary wide'} onclick={start}
			>{chosenCount ? t('lists.addSome', { n: chosenCount }) : t('lists.addAll')}{quiet
				? ' →'
				: ''}</button
		>
	{/if}
	{#if error}<p class="error" role="alert">{error}</p>{/if}
</div>

<style>
	.add {
		display: grid;
		gap: var(--space-2);
	}
	.quiet {
		justify-self: start;
		padding: var(--space-2) 0;
		background: none;
		border: 0;
		color: var(--ink);
		font: inherit;
		font-weight: 700;
		cursor: pointer;
	}
	.label,
	.done {
		margin: 0;
		font-size: var(--text-sm);
	}
	.label {
		color: var(--muted);
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
		font: inherit;
		font-weight: 600;
		cursor: pointer;
	}
	.new {
		display: flex;
		gap: var(--space-2);
	}
	.new input {
		flex: 1;
		min-width: 0;
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-size: var(--text-sm);
	}
</style>
