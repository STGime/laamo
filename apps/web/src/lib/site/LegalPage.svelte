<!-- One legal document (imprint, privacy, terms) from lib/site/legal.ts. -->
<script lang="ts">
	import type { Lang } from '#lib/i18n/index.svelte.ts';
	import { LEGAL, type LegalSlug } from './legal.ts';
	import Seo from './Seo.svelte';

	let { lang, slug }: { lang: Lang; slug: LegalSlug } = $props();
	const doc = $derived(LEGAL[lang][slug]);
</script>

<Seo title="{doc.title} · Laamo" description={doc.description} {lang} path="/{slug}" />

<article>
	<h1>{doc.title}</h1>
	<p class="updated">{doc.updated}</p>
	{#each doc.sections as s, i (i)}
		{#if s.h}<h2>{s.h}</h2>{/if}
		{#each s.blocks as b, j (j)}
			{#if typeof b === 'string'}
				<p>{b}</p>
			{:else}
				<ul>
					{#each b.list as item, k (k)}<li>{item}</li>{/each}
				</ul>
			{/if}
		{/each}
	{/each}
</article>

<style>
	article {
		padding: var(--space-5) var(--gutter) var(--space-7);
	}
	h1 {
		font-size: 36px;
	}
	.updated {
		margin: var(--space-2) 0 var(--space-5);
		color: var(--muted);
		font-size: var(--text-sm);
	}
	h2 {
		margin: var(--space-6) 0 var(--space-2);
		font-size: var(--text-lg);
	}
	p,
	ul {
		margin: 0 0 var(--space-3);
	}
	li {
		margin-bottom: var(--space-1);
	}
</style>
