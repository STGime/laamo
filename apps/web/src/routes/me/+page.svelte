<script lang="ts">
	import { goto } from '$app/navigation';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { i18n, t, type Lang } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { catalog } from '@laamo/catalog';
	import { cities } from '#lib/cities.ts';

	// About me: the facts helpers use as {my city} and so on (S15).
	let profile = $state<Record<string, string> | null>(null);
	let editing = $state<string | null>(null);
	let draft = $state('');
	let saving = $state(false);
	const owner = $derived(session.workspace?.role === 'owner');

	$effect(() => {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				if (data) profile = data.profile ?? {};
			});
	});

	async function saveFact(key: string, value: string) {
		const ws = session.workspace;
		if (!ws) return;
		saving = true;
		const { data } = await api.PATCH('/workspaces/{workspaceId}', {
			params: { path: { workspaceId: ws.id } },
			body: { profile: { [key]: value.trim() } }
		});
		saving = false;
		if (data) {
			profile = data.profile ?? {};
			editing = null;
		}
	}

	async function setLang(lang: Lang) {
		i18n.lang = lang;
		const ws = session.workspace;
		if (ws && ws.role === 'owner') {
			await api.PATCH('/workspaces/{workspaceId}', {
				params: { path: { workspaceId: ws.id } },
				body: { locale: lang }
			});
			await session.refresh();
		}
	}
	function signOut() {
		session.signOut();
		goto('/signin');
	}
</script>

<PageHeader title={t('me.title')} />

<dl class="rows">
	<div>
		<dt>{t('me.workspace')}</dt>
		<dd>{session.workspace?.name ?? '–'}</dd>
	</div>
	<div>
		<dt>{t('me.language')}</dt>
		<dd class="seg" role="group" aria-label={t('me.language')}>
			<button type="button" aria-pressed={i18n.lang === 'de'} onclick={() => setLang('de')}
				>Deutsch</button
			>
			<button type="button" aria-pressed={i18n.lang === 'en'} onclick={() => setLang('en')}
				>English</button
			>
		</dd>
	</div>
</dl>
<a class="row-link" href="/places"><span>{t('me.places')}</span><span aria-hidden="true">›</span></a
>

<section class="about">
	<h2>{t('me.about')}</h2>
	<p class="hint">{t('me.aboutHint')}</p>
</section>
{#if profile}
	<ul class="facts">
		{#each catalog.profile as f (f.key)}
			<li>
				{#if editing === f.key}
					<form
						class="edit"
						onsubmit={(e) => {
							e.preventDefault();
							saveFact(f.key, draft);
						}}
					>
						<label for="fact-{f.key}">{f.label[i18n.lang]}</label>
						<div class="line">
							<input
								id="fact-{f.key}"
								bind:value={draft}
								maxlength="200"
								list={f.key === 'city' ? 'cities' : undefined}
							/>
							<button class="btn primary" disabled={saving}>{t('helper.save')}</button>
						</div>
						<button type="button" class="link" onclick={() => (editing = null)}
							>{t('helper.cancel')}</button
						>
					</form>
				{:else}
					<button
						class="fact"
						disabled={!owner}
						onclick={() => ((draft = profile?.[f.key] ?? ''), (editing = f.key))}
					>
						<span class="label">{f.label[i18n.lang]}</span>
						<span class="value" class:empty={!profile[f.key]}
							>{profile[f.key] || t('me.notSet')}</span
						>
					</button>
					{#if profile[f.key] && owner}
						<button class="forget" disabled={saving} onclick={() => saveFact(f.key, '')}
							>{t('me.forget')}</button
						>
					{/if}
				{/if}
			</li>
		{/each}
	</ul>
	<datalist id="cities">
		{#each cities as c (c)}<option value={c}></option>{/each}
	</datalist>
{/if}

<p class="who">{t('me.signedInAs', { email: session.me?.user.email ?? '' })}</p>
<div class="actions">
	<button type="button" class="btn wide" onclick={signOut}>{t('me.signOut')}</button>
	{#if import.meta.env.DEV}<a class="kit" href="/kit">Component kit</a>{/if}
</div>

<style>
	.rows {
		margin: 0;
	}
	.rows div {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-height: 56px;
		padding: 0 var(--gutter);
		border-bottom: var(--border);
	}
	dt {
		font-weight: 700;
	}
	dd {
		margin: 0;
		color: var(--muted);
	}
	.seg {
		display: flex;
		gap: var(--space-1);
	}
	.seg button {
		min-height: 40px;
		padding: 0 var(--space-3);
		border: 0;
		border-radius: var(--radius-small);
		background: var(--gray-50);
		font-weight: 700;
		cursor: pointer;
	}
	.seg button[aria-pressed='true'] {
		background: var(--ink);
		color: var(--white);
	}
	.row-link {
		display: flex;
		align-items: center;
		justify-content: space-between;
		min-height: 56px;
		padding: 0 var(--gutter);
		border-bottom: var(--border);
		color: inherit;
		font-weight: 700;
		text-decoration: none;
	}
	.about {
		padding: var(--space-5) var(--gutter) var(--space-2);
	}
	.about h2 {
		font-size: var(--text-lg);
	}
	.hint {
		margin: var(--space-1) 0 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.facts {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.facts li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: 56px;
		padding: 0 var(--gutter);
		border-bottom: var(--border);
	}
	.fact {
		flex: 1;
		min-width: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-height: 56px;
		padding: 0;
		border: 0;
		background: none;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.label {
		flex-shrink: 0;
		font-weight: 700;
		white-space: nowrap;
	}
	.value {
		min-width: 0;
		overflow: hidden;
		color: var(--ink);
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.value.empty {
		color: var(--muted);
	}
	.forget,
	.link {
		min-height: 40px;
		padding: 0 var(--space-2);
		border: 0;
		background: none;
		color: var(--muted);
		font: inherit;
		font-size: var(--text-sm);
		font-weight: 700;
		cursor: pointer;
	}
	.edit {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-3) 0;
	}
	.edit label {
		font-weight: 700;
	}
	.line {
		display: flex;
		gap: var(--space-2);
	}
	.line input {
		flex: 1;
		min-width: 0;
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
	}
	.edit .link {
		align-self: flex-start;
		padding: 0;
	}
	.who {
		padding: var(--space-4) var(--gutter) 0;
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-4) var(--gutter);
	}
	.kit {
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
