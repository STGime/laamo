<!-- Posting to social media: switch it on (a Posta account is made for you,
     or you paste the key of your own), see the plan, switch it off. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from '#lib/components/BottomSheet.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	type Social = Schemas['SocialPosting'];
	let social = $state<Social | null>(null);
	let accepted = $state(false);
	let own = $state(false); // "I already have a Posta account"
	let key = $state('');
	let busy = $state(false);
	let problem = $state<Key | null>(null);
	let confirmOff = $state(false);
	const owner = $derived(session.workspace?.role === 'owner');

	function load() {
		const ws = session.workspace;
		if (!ws) return;
		api
			.GET('/workspaces/{workspaceId}/social', { params: { path: { workspaceId: ws.id } } })
			.then(({ data }) => {
				if (data) social = data;
			});
	}
	$effect(load);

	async function enable(body: { accept_terms?: boolean; api_key?: string }) {
		const ws = session.workspace;
		if (!ws || busy) return;
		busy = true;
		problem = null;
		const { data, error } = await api.PUT('/workspaces/{workspaceId}/social', {
			params: { path: { workspaceId: ws.id } },
			body
		});
		busy = false;
		if (data) {
			social = data;
			key = '';
			return;
		}
		const code = (error as { code?: string } | undefined)?.code;
		if (code === 'posta_account_exists') {
			// Their email has a Posta account: only they can hand over its key.
			own = true;
			problem = 'social.err.exists';
		} else if (code === 'posta_key_invalid') problem = 'social.err.key';
		else if (code === 'posta_elsewhere') load();
		else problem = 'social.err.down';
	}

	async function disable() {
		const ws = session.workspace;
		if (!ws || busy) return;
		busy = true;
		problem = null;
		const { response } = await api.DELETE('/workspaces/{workspaceId}/social', {
			params: { path: { workspaceId: ws.id } }
		});
		busy = false;
		confirmOff = false;
		if (response.ok) {
			accepted = false;
			own = false;
			load();
		} else problem = 'social.err.down';
	}

	const day = (iso: string) =>
		new Date(iso).toLocaleDateString(i18n.lang, { day: 'numeric', month: 'long' });
	// What the plan line says for an account Laamo made.
	const planLine = $derived.by(() => {
		const p = social?.plan;
		if (!p) return '';
		if (!p.active) return t(p.type === 'trial' ? 'social.plan.trialOver' : 'social.plan.paused');
		if (p.type === 'trial') {
			const n = p.days_left ?? 0;
			return t(n === 1 ? 'social.plan.trial.one' : 'social.plan.trial', { n });
		}
		return p.ends_at ? t('social.plan.until', { when: day(p.ends_at) }) : t('social.plan.on');
	});
</script>

<PageHeader title={t('social.title')} />
<p class="lead">{t('social.lead')}</p>

{#if !social}
	<p class="note">{t('loading')}</p>
{:else if social.state === 'unavailable'}
	<p class="note">{t('social.unavailable')}</p>
{:else if social.state === 'elsewhere'}
	<p class="note">{t('social.elsewhere', { name: social.elsewhere ?? '' })}</p>
{:else if social.state === 'on'}
	<dl class="rows">
		<div>
			<dt>{t('social.status')}</dt>
			<dd>{t('social.on')}</dd>
		</div>
		{#if social.source === 'issued' && social.plan}
			<div>
				<dt>{t('social.plan')}</dt>
				<dd class:bad={!social.plan.active}>{planLine}</dd>
			</div>
		{/if}
		{#if social.channels !== undefined}
			<div>
				<dt>{t('social.channels')}</dt>
				<dd>
					{social.channels}{social.plan?.max_channels
						? ` ${t('social.of', { n: social.plan.max_channels })}`
						: ''}
				</dd>
			</div>
		{/if}
	</dl>
	{#if social.source === 'pasted'}
		<p class="note">{t('social.ownHint')}</p>
	{:else if social.plan && !social.plan.active}
		<p class="note">{t('social.pausedHint')}</p>
	{/if}
	{#if problem}<p class="note bad" role="alert">{t(problem)}</p>{/if}
	{#if owner}
		<div class="cta">
			<button type="button" class="btn wide" onclick={() => (confirmOff = true)}
				>{t('social.turnOff')}</button
			>
		</div>
	{/if}
{:else if !owner}
	<p class="note">{t('social.ownerOnly')}</p>
{:else if own}
	<form
		class="form"
		onsubmit={(e) => {
			e.preventDefault();
			enable({ api_key: key });
		}}
	>
		<p>{t('social.own.lead')}</p>
		<ol class="steps">
			<li>{t('social.own.1')}</li>
			<li>{t('social.own.2')}</li>
		</ol>
		<label for="posta-key">{t('social.own.key')}</label>
		<input
			id="posta-key"
			type="password"
			autocomplete="off"
			spellcheck="false"
			bind:value={key}
			placeholder="posta_…"
			maxlength="300"
		/>
		<p class="small">{t('social.own.note')}</p>
		{#if problem}<p class="bad" role="alert">{t(problem)}</p>{/if}
		<button class="btn primary wide" disabled={busy || key.trim().length < 8}
			>{t('social.own.connect')}</button
		>
		<button type="button" class="link" onclick={() => ((own = false), (problem = null))}
			>{t('social.own.back')}</button
		>
	</form>
{:else}
	<form
		class="form"
		onsubmit={(e) => {
			e.preventDefault();
			enable({ accept_terms: true });
		}}
	>
		<ul class="points">
			<li>{t('social.point.1')}</li>
			<li>{t('social.point.2')}</li>
			<li>{t('social.point.3')}</li>
		</ul>
		<!-- The consent Posta asks for before an account is made (dev guide §3). -->
		<label class="consent">
			<input type="checkbox" bind:checked={accepted} />
			<span>
				{t('social.consent.1', { email: session.me?.user.email ?? '' })}
				<a href="https://getposta.app/terms" target="_blank" rel="noopener"
					>{t('social.consent.terms')}</a
				>
				{t('social.consent.and')}
				<a href="https://getposta.app/privacy" target="_blank" rel="noopener"
					>{t('social.consent.privacy')}</a
				>{t('social.consent.2')}
			</span>
		</label>
		{#if problem}<p class="bad" role="alert">{t(problem)}</p>{/if}
		<button class="btn primary wide" disabled={busy || !accepted}>{t('social.turnOn')}</button>
		<button type="button" class="link" onclick={() => ((own = true), (problem = null))}
			>{t('social.haveOwn')}</button
		>
	</form>
{/if}

<BottomSheet bind:open={confirmOff} title={t('social.offTitle')}>
	<p>{t(social?.source === 'pasted' ? 'social.offBodyOwn' : 'social.offBody')}</p>
	{#snippet footer()}
		<div class="confirm">
			<button class="btn" onclick={() => (confirmOff = false)}>{t('helper.cancel')}</button>
			<button class="btn danger-solid" disabled={busy} onclick={disable}
				>{t('social.turnOff')}</button
			>
		</div>
	{/snippet}
</BottomSheet>

<style>
	.lead {
		margin: 0;
		padding: 0 var(--gutter) var(--space-4);
		color: var(--muted);
	}
	.note {
		margin: 0;
		padding: var(--space-3) var(--gutter);
		color: var(--muted);
	}
	.bad {
		color: var(--danger);
		font-weight: 700;
	}
	.rows {
		margin: 0;
		border-top: var(--border);
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
		text-align: right;
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: 0 var(--gutter) var(--space-5);
	}
	.form p {
		margin: 0;
	}
	.points,
	.steps {
		margin: 0;
		padding-left: var(--space-5);
	}
	.points li,
	.steps li {
		margin-bottom: var(--space-1);
	}
	.consent {
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		padding: var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font-size: var(--text-sm);
		cursor: pointer;
	}
	.consent input {
		flex-shrink: 0;
		width: 24px;
		height: 24px;
		margin: 0;
	}
	.form label:not(.consent) {
		font-weight: 700;
	}
	.form input:not([type='checkbox']) {
		min-height: var(--tap-min);
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
	}
	.small {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.link {
		align-self: flex-start;
		min-height: 40px;
		padding: 0;
		border: 0;
		background: none;
		color: var(--muted);
		font: inherit;
		font-size: var(--text-sm);
		font-weight: 700;
		text-decoration: underline;
		cursor: pointer;
	}
	.cta {
		padding: var(--space-4) var(--gutter);
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
