<!-- S5 Connect a chat: pick one, follow three steps, paste the link, get a test message. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { connectable, guides, placeholders, placeIcons, type Connectable } from '#lib/places.ts';
	import { api, session } from '#lib/session.svelte.ts';

	const later = ['telegram', 'teams', 'email'];
	let type = $state<Connectable | null>(null);
	let label = $state('');
	let url = $state('');
	let busy = $state(false);
	let error = $state<Key | ''>('');

	// Back to where the chat was needed (a Shelf template), or the list.
	const back = $derived(
		page.url.searchParams.get('back')?.startsWith('/')
			? page.url.searchParams.get('back')!
			: '/places'
	);

	function pick(c: Connectable) {
		type = c;
		label = label || t(`places.type.${c}` as Key);
		error = '';
	}

	async function connect(e: SubmitEvent) {
		e.preventDefault();
		const ws = session.workspace;
		if (!ws || !type || busy) return;
		busy = true;
		error = '';
		const { data, error: err } = await api.POST('/workspaces/{workspaceId}/places', {
			params: { path: { workspaceId: ws.id } },
			body: { type, label: label.trim(), webhook_url: url.trim() }
		});
		busy = false;
		if (data) {
			goto(back.includes('?') ? `${back}&place=${data.id}` : `${back}?place=${data.id}`, {
				replaceState: true
			});
			return;
		}
		const code = (err as { code?: string } | undefined)?.code ?? 'webhook_unreachable';
		error = (
			code.startsWith('webhook_')
				? `places.err.${code}`
				: code === 'unavailable'
					? 'places.err.off'
					: 'places.err.webhook_unreachable'
		) as Key;
	}
</script>

<div class="wrap">
	<h1>{t('places.newTitle')}</h1>
	<p class="lead">{t('places.newLead')}</p>

	<div class="tiles" role="radiogroup" aria-label={t('places.newTitle')}>
		{#each connectable as c (c)}
			<button
				type="button"
				role="radio"
				aria-checked={type === c}
				class="tile"
				onclick={() => pick(c)}
			>
				<span class="icon" aria-hidden="true">{placeIcons[c]}</span>
				{t(`places.type.${c}` as Key)}
			</button>
		{/each}
		{#each later as c (c)}
			<button type="button" class="tile" disabled>
				<span class="icon" aria-hidden="true">{placeIcons[c]}</span>
				{t(`places.type.${c}` as Key)}
				<span class="soon">{t('places.soon')}</span>
			</button>
		{/each}
	</div>

	{#if type}
		<ol class="guide">
			{#each guides[type] as step, i (i)}
				<li><span class="n" aria-hidden="true">{i + 1}</span>{t(step as Key)}</li>
			{/each}
		</ol>
		<form onsubmit={connect}>
			<label>
				<span>{t('places.link')}</span>
				<input
					bind:value={url}
					type="url"
					inputmode="url"
					autocomplete="off"
					spellcheck="false"
					placeholder={placeholders[type]}
				/>
			</label>
			<label>
				<span>{t('places.name')}</span>
				<input bind:value={label} maxlength="60" />
			</label>
			{#if error}<p class="error" role="alert">{t(error)}</p>{/if}
			<button class="btn primary wide" disabled={busy || !url.trim() || !label.trim()}
				>{busy ? t('places.connecting') : t('places.connect')}</button
			>
			<p class="hint">{t('places.connectHint')}</p>
		</form>
	{/if}
</div>

<style>
	.wrap {
		padding: var(--space-6) var(--gutter) var(--space-7);
	}
	.lead {
		margin: var(--space-2) 0 var(--space-5);
		color: var(--muted);
	}
	.tiles {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-2);
	}
	.tile {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-1);
		min-height: 80px;
		padding: var(--space-3);
		border: 2px solid var(--gray-200);
		border-radius: var(--radius-control);
		background: var(--white);
		font: inherit;
		font-weight: 700;
		text-align: left;
		cursor: pointer;
	}
	.tile[aria-checked='true'] {
		border-color: var(--ink);
	}
	.tile:disabled {
		color: var(--muted);
		cursor: default;
	}
	.icon {
		font-size: 24px;
	}
	.soon {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.guide {
		list-style: none;
		margin: var(--space-5) 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.guide li {
		display: flex;
		gap: var(--space-3);
	}
	.n {
		flex-shrink: 0;
		display: inline-grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: 14px;
		background: var(--ink);
		color: var(--white);
		font-weight: 800;
		font-size: var(--text-sm);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-weight: 700;
	}
	input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-weight: 400;
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-weight: 600;
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
