<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	let helpers = $state<Schemas['Helper'][] | null>(null);
	let started = $state<string | null>(null);
	let busy = $state<string | null>(null);

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/helpers', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				helpers = data?.helpers ?? [];
			});
	});

	async function runNow(id: string) {
		busy = id;
		const { data } = await api.POST('/helpers/{helperId}/runs', {
			params: { path: { helperId: id } },
			body: {}
		});
		busy = null;
		if (data) started = id;
	}
</script>

<PageHeader title={t('helpers.title')} />

{#if helpers && helpers.length === 0}
	<p class="note">{t('helpers.empty')}</p>
	<div class="cta"><a class="btn primary wide" href="/new">{t('feed.empty.cta')}</a></div>
{:else if helpers}
	<ul class="list">
		{#each helpers as h (h.id)}
			<li>
				<span class="avatar" aria-hidden="true">{h.avatar ?? '🧱'}</span>
				<span class="text">
					<span class="name display">{h.name}</span>
					<span class="status">{t(`helpers.${h.status}` as Key)}</span>
					{#if started === h.id}<span class="started">{t('helpers.started')}</span>{/if}
				</span>
				<button
					class="btn"
					disabled={busy === h.id || h.status === 'draft'}
					onclick={() => runNow(h.id)}>{t('helpers.runNow')}</button
				>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.note {
		margin: 0;
		padding: 0 var(--gutter);
		color: var(--muted);
	}
	.cta {
		padding: var(--space-5) var(--gutter);
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.list li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-4) var(--gutter);
		border-bottom: var(--border);
	}
	.avatar {
		font-size: 28px;
	}
	.text {
		flex: 1;
		display: flex;
		flex-direction: column;
	}
	.name {
		font-size: var(--text-lg);
	}
	.status,
	.started {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.started {
		color: var(--ink);
	}
</style>
