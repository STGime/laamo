<!-- The public site's frame: logo, language switch, sign-in; footer with who runs it. -->
<script lang="ts">
	import { t } from '#lib/i18n/index.svelte.ts';
	import { copy } from '#lib/site/copy.ts';
	import { localPath } from '#lib/site/site.ts';
	import { LEGAL, LEGAL_SLUGS } from '#lib/site/legal.ts';
	import { UI } from '#lib/site/usecases.ts';

	let { data, children } = $props();
	const lang = $derived(data.lang);
	const other = $derived(lang === 'de' ? 'en' : 'de');

	// Someone already signed in gets a way back into the app instead.
	let signedIn = $state(false);
	$effect(() => {
		try {
			signedIn = !!localStorage.getItem('laamo.session');
		} catch {
			/* storage blocked */
		}
	});
</script>

<div class="site">
	<header>
		<a class="logo" href={localPath('/', lang)} aria-label={t('site.home', {}, lang)}
			><img src="/logo.svg" alt="" width="72" height="72" /></a
		>
		<nav>
			<a href={localPath(data.path, other)} hreflang={other} lang={other}
				>{t('site.otherLang', {}, lang)}</a
			>
			{#if signedIn}
				<a class="app" href="/feed">{t('site.openApp', {}, lang)}</a>
			{:else}
				<a class="app" href="/start">{t('site.signIn', {}, lang)}</a>
			{/if}
		</nav>
	</header>

	<main>
		{@render children()}
	</main>

	<footer>
		<p class="what"><strong>Laamo</strong>: {copy[lang].category}.</p>
		<p>
			<a href={localPath('/use-cases', lang)}>{UI[lang].all}</a>
			<a href={localPath('/try', lang)}>{copy[lang].tryIt.button}</a>
		</p>
		<p class="legal">
			{#each LEGAL_SLUGS as s (s)}<a href={localPath(`/${s}`, lang)}>{LEGAL[lang][s].title}</a
				>{/each}
		</p>
		<p>{t('site.madeBy', {}, lang)}</p>
		<p><span aria-hidden="true">🇪🇺</span> {t('landing.eu', {}, lang)}</p>
	</footer>
</div>

<style>
	.site {
		max-width: var(--column-max);
		min-height: 100dvh;
		margin: 0 auto;
		background: var(--white);
		display: flex;
		flex-direction: column;
	}
	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		padding: var(--space-3) var(--gutter);
	}
	/* The square logo (static/logo.svg): red wordmark on the brand teal */
	.logo {
		display: block;
		flex-shrink: 0;
	}
	.logo img {
		display: block;
		width: 72px;
		height: 72px;
	}
	nav {
		display: flex;
		align-items: center;
		gap: var(--space-4);
	}
	nav a {
		min-height: var(--tap-min);
		display: inline-flex;
		align-items: center;
		color: var(--ink);
		font-weight: 600;
	}
	nav .app {
		font-weight: 700;
	}
	main {
		flex: 1;
	}
	footer {
		padding: var(--space-5) var(--gutter) var(--space-7);
		border-top: var(--border);
		color: var(--muted);
		font-size: var(--text-sm);
	}
	footer p {
		margin: 0 0 var(--space-2);
	}
	footer a {
		color: var(--ink);
		font-weight: 700;
	}
	footer a + a {
		margin-left: var(--space-4);
	}
	.legal a + a {
		margin-left: 0;
	}
	.legal {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
	.legal a {
		font-weight: 400;
	}
</style>
