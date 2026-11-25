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
	// What the server said, shown small under a failure: it names the cause.
	let detail = $state('');
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
		const { data, error, response } = await api.PUT('/workspaces/{workspaceId}/social', {
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
		detail = (error as { error?: string } | undefined)?.error ?? `HTTP ${response.status}`;
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

	// Channels: the social accounts helpers can post to.
	type Channels = Schemas['SocialChannels'];
	type Platform = Schemas['SocialPlatform'];
	let channels = $state<Channels | null>(null);
	let channelsFailed = $state(false);
	let picking = $state(false);
	let form = $state<Platform | null>(null); // Bluesky: handle and app password
	let handle = $state('');
	let appPassword = $state('');
	let connectProblem = $state<Key | null>(null);
	let removing = $state<Schemas['SocialChannel'] | null>(null);
	let removeOpen = $state(false);

	function loadChannels() {
		const ws = session.workspace;
		if (!ws || social?.state !== 'on') return;
		api
			.GET('/workspaces/{workspaceId}/social/channels', {
				params: { path: { workspaceId: ws.id } }
			})
			.then(({ data }) => {
				channels = data ?? null;
				channelsFailed = !data;
			});
	}
	$effect(loadChannels);
	const platformName = (id: string) =>
		channels?.platforms.find((p) => p.id === id)?.name ?? id.charAt(0).toUpperCase() + id.slice(1);

	const connectErrors: Record<string, Key> = {
		posta_trial_expired: 'social.err.trial',
		posta_plan_inactive: 'social.err.paused',
		posta_channel_limit: 'social.err.limit',
		posta_login_failed: 'social.err.login'
	};
	async function connect(p: Platform, creds?: { identifier: string; app_password: string }) {
		const ws = session.workspace;
		if (!ws || busy) return;
		if (p.form && !creds) {
			form = p;
			connectProblem = null;
			return;
		}
		busy = true;
		connectProblem = null;
		const { data, error } = await api.POST('/workspaces/{workspaceId}/social/channels', {
			params: { path: { workspaceId: ws.id } },
			body: { platform: p.id, ...creds }
		});
		busy = false;
		if (!data) {
			const code = (error as { code?: string } | undefined)?.code ?? '';
			connectProblem = connectErrors[code] ?? 'social.err.down';
			return;
		}
		if (data.authorization_url) {
			// Off to the network's own consent page; it sends the browser back
			// to /places/social/connected.
			location.href = data.authorization_url;
			return;
		}
		picking = false;
		form = null;
		handle = appPassword = '';
		load();
		loadChannels();
	}
	async function disconnect() {
		const ws = session.workspace;
		if (!ws || !removing || busy) return;
		busy = true;
		const { response } = await api.DELETE('/workspaces/{workspaceId}/social/channels/{channelId}', {
			params: { path: { workspaceId: ws.id, channelId: removing.id } }
		});
		busy = false;
		removeOpen = false;
		if (response.ok) {
			load();
			loadChannels();
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
	{#if channels}
		<h2 class="sub">{t('social.accounts')}</h2>
		{#if channels.channels.length}
			<ul class="list">
				{#each channels.channels as c (c.id)}
					<li>
						<span class="text">
							<strong>{c.name}</strong>
							<span>{platformName(c.platform)}</span>
							{#if !c.ok}<span class="bad">{t('social.reconnect')}</span>{/if}
						</span>
						{#if owner && channels.manage}
							<span class="acts">
								{#if !c.ok}
									{@const p = channels.platforms.find((x) => x.id === c.platform)}
									{#if p}<button class="btn" disabled={busy} onclick={() => connect(p)}
											>{t('social.connectAgain')}</button
										>{/if}
								{/if}
								<button
									class="remove"
									aria-label={t('social.disconnect', { name: c.name })}
									onclick={() => ((removing = c), (removeOpen = true))}>✕</button
								>
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="note">{t(channels.manage ? 'social.noneYet' : 'social.noneOwn')}</p>
		{/if}
		{#if owner && channels.manage && channels.platforms.length}
			<div class="cta">
				<button
					type="button"
					class="btn primary wide"
					onclick={() => ((picking = true), (form = null), (connectProblem = null))}
					>{t('social.connect')}</button
				>
			</div>
		{/if}
	{:else if channelsFailed}
		<p class="note">{t('social.channelsFailed')}</p>
	{/if}
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
		{#if problem}<p class="bad" role="alert">{t(problem)}</p>
			{#if problem === 'social.err.down' && detail}<p class="small">{detail}</p>{/if}{/if}
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
		{#if problem}<p class="bad" role="alert">{t(problem)}</p>
			{#if problem === 'social.err.down' && detail}<p class="small">{detail}</p>{/if}{/if}
		<button class="btn primary wide" disabled={busy || !accepted}>{t('social.turnOn')}</button>
		<button type="button" class="link" onclick={() => ((own = true), (problem = null))}
			>{t('social.haveOwn')}</button
		>
	</form>
{/if}

<BottomSheet
	bind:open={picking}
	title={form ? t('social.form.title', { name: form.name }) : t('social.connect')}
>
	{#if form}
		{@const p = form}
		<form
			class="form inset"
			onsubmit={(e) => {
				e.preventDefault();
				connect(p, { identifier: handle, app_password: appPassword });
			}}
		>
			<ol class="steps">
				<li>{t('social.form.1', { name: p.name })}</li>
				<li>{t('social.form.2')}</li>
			</ol>
			<label for="sc-handle">{t('social.form.handle')}</label>
			<input
				id="sc-handle"
				bind:value={handle}
				autocomplete="off"
				autocapitalize="none"
				spellcheck="false"
				placeholder="anna.bsky.social"
				maxlength="200"
			/>
			<label for="sc-pass">{t('social.form.password')}</label>
			<input
				id="sc-pass"
				type="password"
				bind:value={appPassword}
				autocomplete="off"
				placeholder="xxxx-xxxx-xxxx-xxxx"
				maxlength="200"
			/>
			{#if connectProblem}<p class="bad" role="alert">{t(connectProblem)}</p>{/if}
			<button class="btn primary wide" disabled={busy || !handle.trim() || !appPassword.trim()}
				>{t('social.form.connect')}</button
			>
		</form>
	{:else}
		<p>{t('social.pick')}</p>
		<ul class="pick">
			{#each channels?.platforms ?? [] as p (p.id)}
				<li>
					<button type="button" class="btn wide" disabled={busy} onclick={() => connect(p)}
						>{p.name}{p.images === false ? ` · ${t('social.videoOnly')}` : ''}</button
					>
				</li>
			{/each}
		</ul>
		{#if connectProblem}<p class="bad" role="alert">{t(connectProblem)}</p>{/if}
	{/if}
</BottomSheet>

<BottomSheet bind:open={removeOpen} title={t('social.disconnect', { name: removing?.name ?? '' })}>
	<p>{t('social.disconnectBody')}</p>
	{#snippet footer()}
		<div class="confirm">
			<button class="btn" onclick={() => (removeOpen = false)}>{t('helper.cancel')}</button>
			<button class="btn danger-solid" disabled={busy} onclick={disconnect}
				>{t('social.disconnectDo')}</button
			>
		</div>
	{/snippet}
</BottomSheet>

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
	.sub {
		margin: 0;
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
	.text .bad {
		color: var(--danger);
	}
	.acts {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.remove {
		width: var(--tap-min);
		height: var(--tap-min);
		border: 0;
		background: none;
		color: var(--muted);
		font-size: var(--text-lg);
		cursor: pointer;
	}
	.pick {
		list-style: none;
		margin: var(--space-3) 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.form.inset {
		padding: 0;
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
