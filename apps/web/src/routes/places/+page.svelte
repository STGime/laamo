<!-- S13 Places: the chats your helpers post to, with a test button. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { ago } from '#lib/time.ts';

	type Place = Schemas['Place'];
	let places = $state<Place[] | null>(null);
	let testing = $state<string | null>(null);
	let result = $state<{ id: string; ok: boolean; code?: string } | null>(null);
	let removing = $state<Place | null>(null);
	let open = $state(false);
	const owner = $derived(session.workspace?.role === 'owner');

	function load() {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/places', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				places = data?.places ?? [];
			});
	}
	$effect(load);

	async function test(p: Place) {
		testing = p.id;
		result = null;
		const { data, error } = await api.POST('/places/{placeId}/test', {
			params: { path: { placeId: p.id } }
		});
		testing = null;
		result = { id: p.id, ok: !!data, code: (error as { code?: string } | undefined)?.code };
		if (data) places = places?.map((x) => (x.id === p.id ? data : x)) ?? null;
	}

	async function remove() {
		if (!removing) return;
		const id = removing.id;
		const { response } = await api.DELETE('/places/{placeId}', {
			params: { path: { placeId: id } }
		});
		if (response.ok) places = places?.filter((x) => x.id !== id) ?? null;
		open = false;
	}
</script>

<PageHeader title={t('places.title')} />
<p class="lead">{t('places.lead')}</p>

<ul class="list">
	<li class="feed">
		<span class="text"><strong>{t('places.feed')}</strong><span>{t('places.feedHint')}</span></span>
	</li>
	{#each places ?? [] as p (p.id)}
		<li>
			<span class="text">
				<strong>{p.label}</strong>
				<span
					>{t(`places.type.${p.type}` as Key)}{p.last4 ? ` · …${p.last4}` : ''}{p.verified_at
						? ` · ${t('places.verified', { when: ago(p.verified_at) })}`
						: ''}</span
				>
				{#if p.used_by?.length}<span>{t('places.usedBy', { names: p.used_by.join(', ') })}</span
					>{/if}
				{#if result?.id === p.id}
					<span class={result.ok ? 'ok' : 'bad'} role="status"
						>{result.ok
							? t('places.testOk')
							: t(`places.err.${result.code ?? 'webhook_unreachable'}` as Key)}</span
					>
				{/if}
			</span>
			<span class="acts">
				<button class="btn" disabled={testing === p.id} onclick={() => test(p)}
					>{t('places.test')}</button
				>
				{#if owner}
					<button
						class="remove"
						aria-label={t('places.remove')}
						onclick={() => ((removing = p), (open = true))}>✕</button
					>
				{/if}
			</span>
		</li>
	{/each}
</ul>

{#if owner}
	<div class="cta"><a class="btn primary wide" href="/places/new">{t('places.add')}</a></div>
{/if}

<BottomSheet bind:open title={t('places.removeTitle', { name: removing?.label ?? '' })}>
	<p>
		{removing?.used_by?.length
			? t('places.removeUsed', { names: removing.used_by.join(', ') })
			: t('places.removeBody')}
	</p>
	{#snippet footer()}
		<div class="confirm">
			<button class="btn" onclick={() => (open = false)}>{t('helper.cancel')}</button>
			<button class="btn danger-solid" onclick={remove}>{t('places.remove')}</button>
		</div>
	{/snippet}
</BottomSheet>

<style>
	.lead {
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
	.list li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: 64px;
		padding: var(--space-3) var(--gutter);
		border-bottom: var(--border);
	}
	.text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.text span {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.text .ok {
		color: var(--ink);
		font-weight: 700;
	}
	.text .bad {
		color: var(--danger);
		font-weight: 700;
	}
	.acts {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}
	.remove {
		min-width: 40px;
		min-height: 40px;
		border: 0;
		background: none;
		color: var(--muted);
		font-size: var(--text-lg);
		cursor: pointer;
	}
	.cta {
		padding: var(--space-5) var(--gutter);
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
</style>
