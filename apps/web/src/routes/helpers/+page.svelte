<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { listen } from '#lib/feed.svelte.ts';
	import { busy as running, progressText } from '#lib/progress.ts';
	import { api, session } from '#lib/session.svelte.ts';

	let helpers = $state<Schemas['Helper'][] | null>(null);
	let starting = $state<string | null>(null);

	function load() {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/helpers', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				helpers = data?.helpers ?? [];
			});
	}

	// Progress comes live: run and step changes reload the list.
	$effect(() => {
		if (!session.workspace) return;
		load();
		return listen(load, () => session.fresh(), ['run', 'step', 'reload']);
	});

	async function runNow(id: string) {
		starting = id;
		await api.POST('/helpers/{helperId}/runs', { params: { path: { helperId: id } }, body: {} });
		load();
		starting = null;
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
				<a class="open" href="/helpers/{h.id}">
					<span class="avatar" aria-hidden="true">{h.avatar ?? '🧱'}</span>
					<span class="text">
						<span class="name display">{h.name}</span>
						{#if h.run}
							<span class="progress">
								<span class="pulse" aria-hidden="true"></span>{progressText(h.run)}
							</span>
							{#if h.run.steps > 0}
								<span class="bar" aria-hidden="true"
									><span style:width="{(Math.min(h.run.step, h.run.steps) / h.run.steps) * 100}%"
									></span></span
								>
							{/if}
						{:else}
							<span class="status">{t(`helpers.${h.status}` as Key)} ›</span>
						{/if}
					</span>
				</a>
				<button
					class="btn"
					disabled={starting === h.id || running(h.run) || h.status === 'draft'}
					onclick={() => runNow(h.id)}
					>{running(h.run) ? t('helpers.busy') : t('helpers.runNow')}</button
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
	.open {
		flex: 1;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--tap-min);
		color: inherit;
		text-decoration: none;
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
	.status {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.progress {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		font-weight: 700;
	}
	.pulse {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		background: var(--brick-blue);
	}
	.bar {
		display: block;
		width: 100%;
		max-width: 200px;
		height: 6px;
		margin-top: var(--space-1);
		border-radius: 3px;
		background: var(--gray-200);
		overflow: hidden;
	}
	.bar span {
		display: block;
		height: 100%;
		background: var(--brick-blue);
		transition: width var(--motion-base) var(--ease);
	}
</style>
