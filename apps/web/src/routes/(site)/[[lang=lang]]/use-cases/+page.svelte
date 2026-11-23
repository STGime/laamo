<!-- All use cases: one line each, linking to its page. -->
<script lang="ts">
	import HelperIcon from '#lib/components/HelperIcon.svelte';
	import { copy } from '#lib/site/copy.ts';
	import { INDUSTRIES, INDUSTRY_UI } from '#lib/site/industries.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { localPath } from '#lib/site/site.ts';
	import { UI, USE_CASES } from '#lib/site/usecases.ts';

	let { data } = $props();
	const ui = $derived(UI[data.lang]);
</script>

<Seo title={ui.listMeta} description={ui.listLead} lang={data.lang} path="/use-cases" />

<section>
	<h1>{ui.listTitle}</h1>
	<p class="lead">{ui.listLead}</p>
</section>
<ul class="list">
	{#each USE_CASES as u (u.slug)}
		{@const t = u.text[data.lang]}
		<li>
			<a href={localPath(`/use-cases/${u.slug}`, data.lang)}>
				<HelperIcon avatar={u.avatar} size={40} />
				<span>
					<b
						>{t.name}{#if u.soon}<span class="soon">{copy[data.lang].soon}</span>{/if}</b
					>
					<span class="desc">{t.description}</span>
				</span>
			</a>
		</li>
	{/each}
</ul>

<section class="kinds">
	<h2>{INDUSTRY_UI[data.lang].listTitle}</h2>
	<ul class="list plain">
		{#each INDUSTRIES as ind (ind.slug)}
			{@const it = ind.text[data.lang]}
			<li>
				<a href={localPath(`/for/${ind.slug}`, data.lang)}>
					<HelperIcon avatar={ind.avatar} size={40} />
					<span>
						<b>{it.name}</b>
						<span class="desc">{it.description}</span>
					</span>
				</a>
			</li>
		{/each}
	</ul>
</section>

<section class="what">
	<h2>{ui.whatTitle}</h2>
	{#each ui.what as p, i (i)}<p>{p}</p>{/each}
</section>

<style>
	section {
		padding: var(--space-5) var(--gutter) var(--space-4);
	}
	h1 {
		font-size: 40px;
	}
	.lead {
		margin: var(--space-4) 0 0;
		font-size: var(--text-lg);
		color: var(--muted);
	}
	.list {
		list-style: none;
		margin: 0 0 var(--space-5);
		padding: 0;
		border-top: var(--border);
	}
	.list a {
		display: flex;
		gap: var(--space-3);
		padding: var(--space-4) var(--gutter);
		border-bottom: var(--border);
		color: var(--ink);
		text-decoration: none;
	}
	b {
		display: block;
		font-size: var(--text-lg);
	}
	.desc {
		display: block;
		margin-top: var(--space-1);
		color: var(--muted);
	}
	.kinds {
		padding-left: 0;
		padding-right: 0;
	}
	.kinds h2 {
		margin: 0 0 var(--space-4);
		padding: 0 var(--gutter);
	}
	.list.plain {
		margin-bottom: 0;
	}
	.what {
		padding-bottom: var(--space-7);
	}
	.what h2 {
		margin: 0 0 var(--space-3);
	}
	.what p {
		margin: 0 0 var(--space-3);
	}
	.soon {
		margin-left: var(--space-2);
		padding: 2px var(--space-2);
		border-radius: 4px;
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-size: var(--text-sm);
	}
</style>
