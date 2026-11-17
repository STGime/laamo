<!-- Head tags for a site page: title, description, canonical URL, the page in
     the other language (hreflang) and the preview shown when it's shared. -->
<script lang="ts">
	import type { Lang } from '#lib/i18n/index.svelte.ts';
	import { INDEXABLE, LANGS, pageURL } from './site.ts';

	let {
		title,
		description,
		lang,
		path,
		image = '/og.png',
		noindex = false
	}: {
		title: string;
		description: string;
		lang: Lang;
		path: string;
		image?: string;
		/** Pages that aren't for search results (e.g. the confirm page). */
		noindex?: boolean;
	} = $props();

	const url = $derived(pageURL(path, lang));
	const imageURL = $derived(image.startsWith('http') ? image : pageURL(image, 'en'));
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	{#if !INDEXABLE || noindex}<meta name="robots" content="noindex" />{/if}
	<link rel="canonical" href={url} />
	{#each LANGS as l (l)}
		<link rel="alternate" hreflang={l} href={pageURL(path, l)} />
	{/each}
	<link rel="alternate" hreflang="x-default" href={pageURL(path, 'en')} />

	<meta property="og:type" content="website" />
	<meta property="og:site_name" content="Laamo" />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={url} />
	<meta property="og:image" content={imageURL} />
	{#if image === '/og.png'}
		<meta property="og:image:width" content="1200" />
		<meta property="og:image:height" content="630" />
		<meta property="og:image:alt" content="Laamo: little helpers that do your busywork" />
	{/if}
	<meta property="og:locale" content={lang === 'de' ? 'de_DE' : 'en_US'} />
	<meta name="twitter:card" content="summary_large_image" />
</svelte:head>
