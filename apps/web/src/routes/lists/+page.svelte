<!-- S12 Lists: businesses, leads and links your helpers saved. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { ago } from '#lib/time.ts';

	let lists = $state<Schemas['ListSummary'][] | null>(null);
	let name = $state('');
	let adding = $state(false);
	let error = $state('');

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/lists', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => (lists = data ?? []));
	});

	async function create(e: SubmitEvent) {
		e.preventDefault();
		const ws = session.workspace;
		if (!ws || !name.trim()) return;
		adding = true;
		error = '';
		const { data, response } = await api.POST('/workspaces/{workspaceId}/lists', {
			params: { path: { workspaceId: ws.id } },
			body: { name: name.trim() }
		});
		adding = false;
		if (data) {
			lists = [...(lists ?? []), data];
			name = '';
		} else error = response.status === 409 ? t('lists.taken') : t('lists.failed');
	}
</script>

<PageHeader title={t('lists.title')} />

{#if lists?.length}
	<ul class="list">
		{#each lists as l (l.id)}
			<li>
				<a href="/lists/{l.id}">
					<strong>{l.name}</strong>
					<span
						>{t('lists.rows', { n: l.rows })} · {t('lists.updated', {
							when: ago(l.updated_at)
						})}{l.share_token ? ` · ${t('lists.shared')}` : ''}</span
					>
				</a>
			</li>
		{/each}
	</ul>
{:else if lists}
	<p class="note">{t('lists.empty')}</p>
{/if}

<form class="new" onsubmit={create}>
	<input
		bind:value={name}
		maxlength="80"
		placeholder={t('lists.newName')}
		aria-label={t('lists.newName')}
	/>
	<button class="btn" disabled={adding || !name.trim()}>{t('lists.new')}</button>
</form>
{#if error}<p class="error" role="alert">{error}</p>{/if}

<style>
	.note {
		margin: 0;
		padding: 0 var(--gutter) var(--space-4);
		color: var(--muted);
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.list a {
		display: flex;
		flex-direction: column;
		justify-content: center;
		min-height: 64px;
		padding: var(--space-3) var(--gutter);
		border-bottom: var(--border);
		color: inherit;
		text-decoration: none;
	}
	.list span {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.new {
		display: flex;
		gap: var(--space-2);
		padding: var(--space-4) var(--gutter);
	}
	.new input {
		flex: 1;
		min-width: 0;
	}
	.error {
		margin: 0;
		padding: 0 var(--gutter);
		color: var(--danger);
	}
</style>
