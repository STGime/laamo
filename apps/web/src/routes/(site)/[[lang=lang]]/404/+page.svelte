<!-- The page for an address that doesn't exist. The web server sends it with
     status 404 (deploy/docker/web.nginx.conf.template). -->
<script lang="ts">
	import { copy } from '#lib/site/copy.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { localPath } from '#lib/site/site.ts';

	let { data } = $props();
	const c = $derived(copy[data.lang].notFound);
</script>

<Seo title="{c.title} · Laamo" description={c.body} lang={data.lang} path="/404" noindex />

<section>
	<h1>{c.title}</h1>
	<p class="lead">{c.body}</p>
	<a class="btn primary wide" href={localPath('/', data.lang)}>{c.home}</a>
	<a class="btn wide" href={localPath('/use-cases', data.lang)}>{c.all}</a>
</section>

<style>
	section {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-6) var(--gutter) var(--space-7);
	}
	h1 {
		font-size: 36px;
	}
	.lead {
		margin: 0 0 var(--space-3);
		color: var(--muted);
		font-size: var(--text-lg);
	}
</style>
