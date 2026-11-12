<!-- One list: search, sort, delete rows, export CSV, share a read-only link. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { i18n, t } from '#lib/i18n/index.svelte.ts';
	import { cellHref, cellText, columnLabel, csvName, osmSource, rowHeading } from '#lib/lists.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { ago } from '#lib/time.ts';

	const id = $derived(page.params.id ?? '');
	const owner = $derived(session.workspace?.role === 'owner');

	let detail = $state<Schemas['ListDetail'] | null>(null);
	let missing = $state(false);
	let q = $state('');
	let sort = $state('created_at:desc');
	let confirmRow = $state<string | null>(null);
	let renaming = $state(false);
	let name = $state('');
	let confirmDelete = $state(false);
	let copied = $state(false);
	let busy = $state(false);

	async function load() {
		const [col, order] = sort.split(':') as [string, 'asc' | 'desc'];
		const { data, response } = await api.GET('/lists/{listId}', {
			params: { path: { listId: id }, query: { q: q.trim() || undefined, sort: col, order } }
		});
		if (response.status === 404) missing = true;
		if (data) detail = data;
	}
	// Search as you type, a moment after the last key.
	$effect(() => {
		void sort;
		void q;
		if (!id) return;
		const timer = setTimeout(load, 250);
		return () => clearTimeout(timer);
	});

	const list = $derived(detail?.list);
	const columns = $derived(
		(detail?.columns ?? []).map((c) => c.key).filter((k) => columnLabel(k, i18n.lang))
	);
	const shareUrl = $derived(
		list?.share_token ? `${page.url.origin}/shared/lists/${list.share_token}` : ''
	);

	async function removeRow(rowId: string) {
		const { response } = await api.DELETE('/lists/{listId}/rows/{rowId}', {
			params: { path: { listId: id, rowId } }
		});
		confirmRow = null;
		if (response.ok && detail) {
			detail = {
				...detail,
				rows: detail.rows.filter((r) => r.id !== rowId),
				matching: detail.matching - 1,
				list: { ...detail.list, rows: detail.list.rows - 1 }
			};
		}
	}

	async function rename(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		busy = true;
		const { data } = await api.PATCH('/lists/{listId}', {
			params: { path: { listId: id } },
			body: { name: name.trim() }
		});
		busy = false;
		if (data && detail) {
			detail = { ...detail, list: data };
			renaming = false;
		}
	}

	async function exportCsv() {
		busy = true;
		const { data } = await api.GET('/lists/{listId}/csv', {
			params: { path: { listId: id } },
			parseAs: 'blob'
		});
		busy = false;
		if (!data || !list) return;
		const a = document.createElement('a');
		a.href = URL.createObjectURL(data as Blob);
		a.download = csvName(list.name);
		a.click();
		URL.revokeObjectURL(a.href);
	}

	async function share(on: boolean) {
		busy = true;
		if (on) {
			const { data } = await api.PUT('/lists/{listId}/share', { params: { path: { listId: id } } });
			if (data && detail) detail = { ...detail, list: data };
		} else {
			const { response } = await api.DELETE('/lists/{listId}/share', {
				params: { path: { listId: id } }
			});
			if (response.ok && detail)
				detail = { ...detail, list: { ...detail.list, share_token: undefined } };
		}
		busy = false;
	}

	async function copy() {
		await navigator.clipboard?.writeText(shareUrl);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	async function remove() {
		busy = true;
		const { response } = await api.DELETE('/lists/{listId}', { params: { path: { listId: id } } });
		busy = false;
		if (response.ok) goto('/lists', { replaceState: true });
	}
</script>

{#if missing}
	<p class="note">{t('lists.missing')} <a href="/lists">{t('lists.title')} →</a></p>
{:else if list}
	<PageHeader title={list.name}>
		{#snippet right()}{t('lists.rows', { n: list.rows })}{/snippet}
	</PageHeader>

	{#if renaming}
		<form class="row-form" onsubmit={rename}>
			<input bind:value={name} maxlength="80" aria-label={t('helper.rename')} />
			<button class="btn primary" disabled={busy || !name.trim()}>{t('helper.save')}</button>
		</form>
	{/if}

	<div class="tools">
		<input
			type="search"
			bind:value={q}
			placeholder={t('lists.search')}
			aria-label={t('lists.search')}
		/>
		<select bind:value={sort} aria-label={t('lists.sort')}>
			<option value="created_at:desc">{t('lists.newest')}</option>
			<option value="created_at:asc">{t('lists.oldest')}</option>
			{#each columns as c (c)}
				<option value="{c}:asc">{t('lists.byColumn', { column: columnLabel(c, i18n.lang) })}</option
				>
			{/each}
		</select>
	</div>

	{#if detail?.rows.length}
		<ul class="rows">
			{#each detail.rows as r (r.id)}
				{@const heading = rowHeading(r.data)}
				<li>
					<div class="cells">
						{#if heading}<strong>{heading}</strong>{/if}
						<dl>
							{#each columns as c (c)}
								{#if r.data[c] != null && r.data[c] !== '' && r.data[c] !== heading}
									{@const href = cellHref(c, r.data[c])}
									<dt>{columnLabel(c, i18n.lang)}</dt>
									<dd>
										{#if href}<a {href} target="_blank" rel="noopener noreferrer"
												>{cellText(r.data[c])}</a
											>{:else}{cellText(r.data[c])}{/if}
									</dd>
								{/if}
							{/each}
						</dl>
						<span class="when">{t('lists.saved', { when: ago(r.created_at) })}</span>
					</div>
					{#if confirmRow === r.id}
						<span class="sure">
							<button class="btn" onclick={() => (confirmRow = null)}>{t('helper.cancel')}</button>
							<button class="btn danger-solid" onclick={() => removeRow(r.id)}
								>{t('lists.deleteRow')}</button
							>
						</span>
					{:else}
						<button
							class="remove"
							aria-label={t('lists.deleteRow')}
							onclick={() => (confirmRow = r.id)}>✕</button
						>
					{/if}
				</li>
			{/each}
		</ul>
		{#if detail.matching > detail.rows.length}
			<p class="note">{t('lists.more', { shown: detail.rows.length, n: detail.matching })}</p>
		{/if}
	{:else if detail}
		<p class="note">{q.trim() ? t('lists.noMatch') : t('lists.emptyList')}</p>
	{/if}

	{#if detail?.rows.some((r) => osmSource(r.data.url))}
		<p class="note">
			{t('osm.creditRows')}
			<a href="https://www.geoapify.com/" target="_blank" rel="noopener noreferrer"
				>{t('osm.poweredBy')}</a
			>
		</p>
	{/if}

	<section class="acts">
		<button class="btn wide" disabled={busy || !list.rows} onclick={exportCsv}
			>{t('lists.export')}</button
		>
		{#if owner}
			{#if list.share_token}
				<div class="share">
					<p>{t('lists.sharedHint')}</p>
					<input readonly value={shareUrl} aria-label={t('lists.link')} />
					<div class="two">
						<button class="btn" onclick={copy}
							>{copied ? t('lists.copied') : t('lists.copy')}</button
						>
						<button class="btn" disabled={busy} onclick={() => share(false)}
							>{t('lists.unshare')}</button
						>
					</div>
				</div>
			{:else}
				<button class="btn wide" disabled={busy} onclick={() => share(true)}
					>{t('lists.share')}</button
				>
			{/if}
		{/if}
		<button class="btn wide" onclick={() => ((renaming = !renaming), (name = list.name))}
			>{t('helper.rename')}</button
		>
		{#if owner}
			<button class="btn wide danger" onclick={() => (confirmDelete = true)}
				>{t('lists.delete')}</button
			>
		{/if}
	</section>

	<BottomSheet bind:open={confirmDelete} title={t('lists.deleteTitle', { name: list.name })}>
		<p>{t('lists.deleteBody', { n: list.rows })}</p>
		{#snippet footer()}
			<div class="two">
				<button class="btn" onclick={() => (confirmDelete = false)}>{t('helper.cancel')}</button>
				<button class="btn danger-solid" disabled={busy} onclick={remove}
					>{t('helper.deleteYes')}</button
				>
			</div>
		{/snippet}
	</BottomSheet>
{/if}

<style>
	.note {
		margin: 0;
		padding: var(--space-4) var(--gutter);
		color: var(--muted);
	}
	.row-form,
	.tools {
		display: flex;
		gap: var(--space-2);
		padding: 0 var(--gutter) var(--space-3);
	}
	.row-form input,
	.tools input {
		flex: 1;
		min-width: 0;
	}
	.tools select {
		max-width: 45%;
	}
	.rows {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.rows li {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		padding: var(--space-3) var(--gutter);
		border-bottom: var(--border);
	}
	.cells {
		flex: 1;
		min-width: 0;
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 2px var(--space-3);
		margin: var(--space-1) 0 0;
		font-size: var(--text-sm);
	}
	dt {
		color: var(--muted);
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.when {
		display: block;
		margin-top: var(--space-1);
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.remove {
		min-width: var(--tap-min);
		min-height: var(--tap-min);
		background: none;
		border: 0;
		color: var(--muted);
		font-size: 18px;
		cursor: pointer;
	}
	.sure {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.acts {
		display: grid;
		gap: var(--space-2);
		padding: var(--space-6) var(--gutter);
	}
	.share p {
		margin: 0 0 var(--space-2);
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.share input {
		width: 100%;
		margin-bottom: var(--space-2);
	}
	.two {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
	.danger {
		color: var(--danger);
	}
	.danger-solid {
		background: var(--danger);
		color: var(--white);
	}
</style>
