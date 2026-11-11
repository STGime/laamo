<!-- Try before sign-up (#75): one sample helper, a real result in about 15 seconds, then keep it. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, type Schemas } from '#lib/api/client.ts';
	import { guessCity, savePending, solve, type DemoTemplate } from '#lib/demo.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { hostOf, safeURL } from '#lib/links.ts';

	const choices: { slug: DemoTemplate; avatar: string; name: Key; what: Key }[] = [
		{ slug: 'morning-briefing', avatar: '📰', name: 'landing.ex1.name', what: 'try.mb' },
		{ slug: 'weekend-scout', avatar: '🎟️', name: 'landing.ex3.name', what: 'try.ws' }
	];

	let template = $state<DemoTemplate>('morning-briefing');
	let business = $state('');
	let city = $state(guessCity());
	let busy = $state(false);
	let seconds = $state(0);
	let error = $state<Key | ''>('');
	let result = $state<Schemas['DemoResult'] | null>(null);

	const needsBusiness = $derived(template === 'morning-briefing');
	const ready = $derived(
		city.trim().length >= 2 && (!needsBusiness || business.trim().length >= 2)
	);

	async function run(e: SubmitEvent) {
		e.preventDefault();
		if (!ready || busy) return;
		busy = true;
		error = '';
		seconds = 0;
		const tick = setInterval(() => seconds++, 1000);
		try {
			const { data: ch } = await api.GET('/demo/challenge');
			if (!ch?.token) {
				error = 'try.err.off';
				return;
			}
			const nonce = await solve(ch.token, ch.bits);
			const { data, response } = await api.POST('/demo/runs', {
				body: {
					template,
					lang: i18n.lang,
					city: city.trim(),
					business: needsBusiness ? business.trim() : undefined,
					timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
					token: ch.token,
					nonce
				}
			});
			if (data) result = data;
			else
				error =
					response.status === 400
						? 'try.err.input'
						: response.status === 429
							? 'try.err.limit'
							: 'try.err.off';
		} catch {
			error = 'try.err.off';
		} finally {
			clearInterval(tick);
			busy = false;
		}
	}

	function keep() {
		savePending({
			template,
			city: city.trim(),
			business: needsBusiness ? business.trim() : undefined
		});
		goto('/start');
	}

	const links = $derived(
		(result?.links ?? []).filter((l) => safeURL(l.url)).slice(0, 5) as {
			title: string;
			url: string;
		}[]
	);
	const cards = $derived((result?.cards ?? []).filter((c) => c.title));
</script>

<div class="wrap">
	<a class="logo display" href="/">Laamo</a>

	{#if result}
		<article class="card">
			<p class="who">
				<span class="avatar" aria-hidden="true">{result.avatar}</span>
				<span class="tag">{t('try.test')}</span>
			</p>
			<h2 class="display">{cards.length > 1 ? result.title : result.name}</h2>
			{#if cards.length > 1}
				<ul class="items">
					{#each cards as c, i (i)}
						<li>
							{#if safeURL(c.url)}
								<a href={c.url} target="_blank" rel="noopener noreferrer nofollow"
									><strong>{c.title}</strong> <span class="host">{hostOf(c.url)} ↗</span></a
								>
							{:else}<strong>{c.title}</strong>{/if}
							{#if c.summary}<p>{c.summary}</p>{/if}
						</li>
					{/each}
				</ul>
			{:else if result.body}
				<p class="body">{result.body}</p>
			{/if}
			{#if links.length && cards.length <= 1}
				<ul class="links" aria-label={t('feed.sources')}>
					{#each links as l (l.url)}
						<li>
							<a href={l.url} target="_blank" rel="noopener noreferrer nofollow">
								<span class="ltitle">{l.title || hostOf(l.url)}</span>
								<span class="host">{hostOf(l.url)} ↗</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</article>
		<div class="actions">
			<button class="btn primary wide" onclick={keep}>{t('try.keep')}</button>
			<p class="hint">{t('try.keepHint')}</p>
			<button class="link" onclick={() => (result = null)}>{t('try.again')}</button>
		</div>
	{:else}
		<h1>{t('try.title')}</h1>
		<p class="lead">{t('try.body')}</p>
		<form onsubmit={run}>
			<div class="choices" role="radiogroup" aria-label={t('try.title')}>
				{#each choices as c (c.slug)}
					<button
						type="button"
						role="radio"
						aria-checked={template === c.slug}
						class="choice"
						disabled={busy}
						onclick={() => (template = c.slug)}
					>
						<span class="avatar" aria-hidden="true">{c.avatar}</span>
						<span>
							<strong>{t(c.name)}</strong>
							<span class="what">{t(c.what)}</span>
						</span>
					</button>
				{/each}
			</div>
			{#if needsBusiness}
				<label>
					<span>{t('try.business')}</span>
					<input
						bind:value={business}
						maxlength="60"
						placeholder={t('try.businessHint')}
						disabled={busy}
					/>
				</label>
			{/if}
			<label>
				<span>{t('try.city')}</span>
				<input
					bind:value={city}
					maxlength="60"
					autocomplete="address-level2"
					placeholder="Leipzig"
					disabled={busy}
				/>
			</label>
			{#if error}<p class="error" role="alert">{t(error)}</p>{/if}
			<button class="btn primary wide" disabled={!ready || busy}>
				{#if busy}<span class="pulse" aria-hidden="true"></span>{t('try.working', {
						n: seconds
					})}{:else}{t('try.go')}{/if}
			</button>
		</form>
		<a class="signup" href="/start">{t('try.skip')}</a>
	{/if}
</div>

<style>
	.wrap {
		padding: var(--space-6) var(--gutter) var(--space-7);
	}
	.logo {
		display: inline-block;
		margin-bottom: var(--space-6);
		color: var(--accent);
		font-size: var(--text-h2);
		text-decoration: none;
	}
	.lead {
		margin: var(--space-3) 0 var(--space-5);
		color: var(--muted);
		font-size: var(--text-lg);
	}
	form,
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.choices {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.choice {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: 64px;
		padding: var(--space-3);
		border: 2px solid var(--gray-200);
		border-radius: var(--radius-control);
		background: var(--white);
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.choice[aria-checked='true'] {
		border-color: var(--ink);
	}
	.choice .avatar {
		font-size: 28px;
	}
	.what {
		display: block;
		color: var(--muted);
		font-size: var(--text-sm);
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
		font-size: var(--text-lg);
		font-weight: 400;
	}
	.pulse {
		display: inline-block;
		width: 10px;
		height: 10px;
		margin-right: var(--space-2);
		border-radius: 2px;
		background: var(--brick-blue);
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-weight: 600;
	}
	.signup,
	.link {
		display: inline-block;
		min-height: var(--tap-min);
		margin-top: var(--space-4);
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		font-weight: 700;
		cursor: pointer;
	}
	.card {
		margin-bottom: var(--space-5);
		padding: var(--space-4);
		border: var(--border);
		border-radius: var(--radius-card, 12px);
		background: var(--white);
	}
	.who {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0 0 var(--space-2);
		font-size: var(--text-sm);
		font-weight: 700;
	}
	.who .avatar {
		font-size: 20px;
	}
	.tag {
		color: var(--muted);
		font-weight: 600;
	}
	.body {
		margin: var(--space-2) 0 0;
		white-space: pre-line;
	}
	.items,
	.links {
		list-style: none;
		margin: var(--space-3) 0 0;
		padding: 0;
	}
	.items li {
		padding: var(--space-2) 0;
		border-top: var(--border);
	}
	.items p {
		margin: var(--space-1) 0 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.items a,
	.links a {
		color: inherit;
		text-decoration: none;
	}
	.links li {
		border-top: var(--border);
	}
	.links a {
		display: flex;
		flex-direction: column;
		min-height: 44px;
		justify-content: center;
		padding: var(--space-2) 0;
	}
	.ltitle {
		font-weight: 600;
	}
	.host {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
		text-align: center;
	}
</style>
