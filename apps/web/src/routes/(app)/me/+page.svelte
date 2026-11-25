<script lang="ts">
	import { goto } from '$app/navigation';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { i18n, t, type Lang } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';
	import { catalog } from '@laamo/catalog';
	import { cities } from '#lib/cities.ts';
	import { push } from '#lib/push.svelte.ts';

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
	async function setClock(twelve: boolean) {
		i18n.twelve = twelve;
		const ws = session.workspace;
		if (ws && ws.role === 'owner') {
			await api.PATCH('/workspaces/{workspaceId}', {
				params: { path: { workspaceId: ws.id } },
				body: { time_format: twelve ? '12h' : '24h' }
			});
			await session.refresh();
		}
	}
	// Notifications on this device.
	let tested = $state<'sent' | 'failed' | null>(null);
	async function testPush() {
		tested = null;
		tested = (await push.test()) ? 'sent' : 'failed';
	}
	async function signOut() {
		// This device stops getting the notifications of who signs out.
		if (push.state === 'on') await push.disable();
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
	<div>
		<dt>{t('me.timeFormat')}</dt>
		<dd class="seg" role="group" aria-label={t('me.timeFormat')}>
			<button type="button" aria-pressed={!i18n.twelve} onclick={() => setClock(false)}
				>14:30</button
			>
			<button type="button" aria-pressed={i18n.twelve} onclick={() => setClock(true)}
				>2:30 PM</button
			>
		</dd>
	</div>
</dl>
<a class="row-link" href="/places"><span>{t('me.places')}</span><span aria-hidden="true">›</span></a
>
<a class="row-link" href="/places/social"
	><span>{t('social.entry')}</span><span aria-hidden="true">›</span></a
>

{#if push.state !== 'unknown' && push.state !== 'unavailable'}
	<section class="about">
		<h2>{t('push.title')}</h2>
		<p class="hint">{t('push.hint')}</p>
	</section>
	<div class="push">
		{#if push.state === 'on'}
			<p class="pstate"><strong>{t('push.on')}</strong></p>
			<div class="pbtns">
				<button type="button" class="btn" onclick={testPush}>{t('push.test')}</button>
				<button type="button" class="btn" disabled={push.busy} onclick={() => push.disable()}
					>{t('push.turnOff')}</button
				>
			</div>
			{#if tested}<p class="hint" role="status">
					{t(tested === 'sent' ? 'push.testSent' : 'push.testFailed')}
				</p>{/if}
		{:else if push.state === 'off'}
			<button
				type="button"
				class="btn primary wide"
				disabled={push.busy}
				onclick={() => push.enable()}>{t('push.turnOn')}</button
			>
		{:else if push.state === 'homeScreen'}
			<p class="pstate">{t('push.homeScreen')}</p>
			<ol class="steps">
				<li>{t('push.homeScreen.1')}</li>
				<li>{t('push.homeScreen.2')}</li>
				<li>{t('push.homeScreen.3')}</li>
			</ol>
		{:else if push.state === 'blocked'}
			<p class="pstate">{t('push.blocked')}</p>
		{:else}
			<p class="pstate">{t('push.unsupported')}</p>
		{/if}
		{#if push.failed}<p class="hint" role="alert">{t('push.failed')}</p>{/if}
	</div>
{/if}

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
						{#if f.help}
							<textarea
								id="fact-{f.key}"
								rows="3"
								bind:value={draft}
								maxlength="500"
								placeholder={f.placeholder?.[i18n.lang] ?? ''}
								aria-describedby="fh-{f.key}"></textarea>
							<p class="fact-help" id="fh-{f.key}">{f.help[i18n.lang]}</p>
							<button class="btn primary" disabled={saving}>{t('helper.save')}</button>
						{:else}
							<div class="line">
								<input
									id="fact-{f.key}"
									bind:value={draft}
									maxlength="500"
									placeholder={f.placeholder?.[i18n.lang] ?? ''}
									list={f.key === 'city' ? 'cities' : undefined}
								/>
								<button class="btn primary" disabled={saving}>{t('helper.save')}</button>
							</div>
						{/if}
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
	.push {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-2) var(--gutter) var(--space-2);
	}
	.pstate {
		margin: 0;
	}
	.pbtns {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.steps {
		margin: 0;
		padding-left: var(--space-5);
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
	.edit textarea {
		width: 100%;
		padding: var(--space-2) var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		resize: vertical;
	}
	.fact-help {
		margin: var(--space-1) 0 var(--space-2);
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
