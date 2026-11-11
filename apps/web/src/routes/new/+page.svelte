<!-- S7 Shelf: templates sorted by the needs picked in onboarding. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	let templates = $state<Schemas['ShelfTemplate'][]>([]);
	let failed = $state(false);

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/templates', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				if (data) templates = data.templates;
				else failed = true;
			})
			.catch(() => (failed = true));
	});

	const forYou = $derived(templates.filter((x) => x.match > 0));
	const more = $derived(templates.filter((x) => x.match === 0));
</script>

<PageHeader title={t('shelf.title')} />

{#snippet card(tpl: Schemas['ShelfTemplate'])}
	<li>
		<a href="/new/{tpl.slug}">
			<span class="avatar" aria-hidden="true">{tpl.avatar}</span>
			<span class="text">
				<span class="name display">{tpl.name}</span>
				<span class="promise">{tpl.promise}</span>
				<span class="mini" aria-hidden="true">
					{#each tpl.bricks as b (b.id)}<span class={b.group}></span>{/each}
				</span>
				<span class="uses">
					{t('shelf.uses')}:
					{#if tpl.brain}{t('shelf.brain')}{/if}{#if tpl.places.length}&nbsp;· {t(
							'shelf.chat'
						)}{/if}
				</span>
			</span>
		</a>
	</li>
{/snippet}

{#if failed}
	<p class="note">{t('error.offline')}</p>
{:else}
	{#if forYou.length}
		<h2 class="section">{t('shelf.forYou')}</h2>
		<ul class="list">
			{#each forYou as tpl (tpl.slug)}{@render card(tpl)}{/each}
		</ul>
	{/if}
	{#if more.length}
		<h2 class="section">{forYou.length ? t('shelf.more') : t('shelf.title')}</h2>
		<ul class="list">
			{#each more as tpl (tpl.slug)}{@render card(tpl)}{/each}
		</ul>
	{/if}
	<p class="note">{t('shelf.build')} · {t('new.soon')}</p>
{/if}

<style>
	.section {
		padding: var(--space-5) var(--gutter) var(--space-2);
		font-size: var(--text-lg);
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.list li {
		border-bottom: var(--border);
	}
	.list a {
		display: flex;
		gap: var(--space-3);
		padding: var(--space-4) var(--gutter);
		color: inherit;
		text-decoration: none;
		transition: transform var(--motion-fast) var(--ease);
	}
	.list a:active {
		transform: scale(var(--press-scale));
	}
	.avatar {
		font-size: 32px;
		line-height: 1;
	}
	.text {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.name {
		font-size: var(--text-lg);
	}
	.uses {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.mini {
		display: flex;
		gap: 3px;
		margin: var(--space-1) 0;
	}
	.mini span {
		width: 24px;
		height: 10px;
		border-radius: 3px;
		box-shadow: var(--brick-edge);
	}
	.mini .when {
		background: var(--brick-yellow);
	}
	.mini .look {
		background: var(--brick-blue);
	}
	.mini .think {
		background: var(--brick-purple);
	}
	.mini .do {
		background: var(--brick-green);
	}
	.mini .ask {
		background: var(--brick-red);
	}
	.note {
		margin: 0;
		padding: var(--space-5) var(--gutter);
		color: var(--muted);
	}
</style>
