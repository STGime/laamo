<!-- Laamo for one kind of business: what helpers do there, each linking to
     its use-case page, a week of results, questions and the waiting list.
     Words in lib/site/industries.ts. -->
<script lang="ts">
	import HelperIcon from '#lib/components/HelperIcon.svelte';
	import { copy } from '#lib/site/copy.ts';
	import { INDUSTRIES, INDUSTRY_UI } from '#lib/site/industries.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { jsonLD, localPath, organizationLD, pageURL } from '#lib/site/site.ts';
	import { UI, useCase } from '#lib/site/usecases.ts';
	import WaitlistForm from '#lib/site/WaitlistForm.svelte';

	let { data } = $props();
	const ind = $derived(data.industry);
	const t = $derived(ind.text[data.lang]);
	const ui = $derived(INDUSTRY_UI[data.lang]);
	const others = $derived(INDUSTRIES.filter((o) => o.slug !== ind.slug));

	const ld = $derived(
		jsonLD({
			'@context': 'https://schema.org',
			'@graph': [
				{
					'@type': 'BreadcrumbList',
					itemListElement: [
						{ '@type': 'ListItem', position: 1, name: 'Laamo', item: pageURL('/', data.lang) },
						{
							'@type': 'ListItem',
							position: 2,
							name: UI[data.lang].all,
							item: pageURL('/use-cases', data.lang)
						},
						{
							'@type': 'ListItem',
							position: 3,
							name: t.name,
							item: pageURL(`/for/${ind.slug}`, data.lang)
						}
					]
				},
				{
					'@type': 'FAQPage',
					mainEntity: t.faq.map((i) => ({
						'@type': 'Question',
						name: i.q,
						acceptedAnswer: { '@type': 'Answer', text: i.a }
					}))
				},
				organizationLD()
			]
		})
	);
</script>

<Seo title={t.metaTitle} description={t.description} lang={data.lang} path="/for/{ind.slug}" />
<svelte:head>
	{@html `<script type="application/ld+json">${ld}</script>`}
</svelte:head>

<section class="top">
	<p class="crumb"><a href={localPath('/use-cases', data.lang)}>{UI[data.lang].all}</a></p>
	<p class="name"><HelperIcon avatar={ind.avatar} size={28} /> Laamo: {copy[data.lang].category}</p>
	<h1>{t.title}</h1>
	{#each t.intro as p, i (i)}<p class="lead">{p}</p>{/each}
</section>

<section>
	<h2>{ui.jobs}</h2>
	{#each t.jobs as job (job.useCase)}
		{@const u = useCase(job.useCase)}
		<div class="job">
			<h3>{job.title}</h3>
			<p>{job.body}</p>
			{#if u}
				<a href={localPath(`/use-cases/${u.slug}`, data.lang)}
					><HelperIcon avatar={u.avatar} size={20} />
					{u.text[data.lang].name}: {ui.more} →</a
				>
			{/if}
		</div>
	{/each}
</section>

<section>
	<h2>{ui.week}</h2>
	<p>{ui.weekLead}</p>
	<ul class="week">
		{#each t.week as w, i (i)}
			<li><b>{w.when}</b> {w.what}</li>
		{/each}
	</ul>
	<a class="btn wide outline" href={localPath('/try', data.lang)}>{UI[data.lang].tryIt}</a>
</section>

<section>
	<h2>{ui.faq}</h2>
	<dl class="faq">
		{#each t.faq as f (f.q)}
			<dt>{f.q}</dt>
			<dd>{f.a}</dd>
		{/each}
	</dl>
</section>

<section>
	<h2>{UI[data.lang].join}</h2>
	<p>{UI[data.lang].joinBody}</p>
	<WaitlistForm lang={data.lang} source="for:{ind.slug}" id="ind" />
</section>

<section>
	<h2>{ui.listTitle}</h2>
	<ul class="others">
		{#each others as o (o.slug)}
			<li>
				<a href={localPath(`/for/${o.slug}`, data.lang)}
					><HelperIcon avatar={o.avatar} size={28} /> {o.text[data.lang].name}</a
				>
			</li>
		{/each}
	</ul>
</section>

<style>
	section {
		padding: var(--space-6) var(--gutter);
		border-top: var(--border);
	}
	.top {
		padding-top: var(--space-4);
		border-top: 0;
	}
	.crumb {
		margin: 0 0 var(--space-4);
		font-size: var(--text-sm);
	}
	.crumb a {
		color: var(--muted);
	}
	.name {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0 0 var(--space-2);
		font-weight: 700;
	}
	h1 {
		font-size: 36px;
	}
	.lead {
		margin: var(--space-4) 0 0;
		font-size: var(--text-lg);
		color: var(--muted);
	}
	h2 {
		margin: 0 0 var(--space-3);
	}
	.job {
		margin-top: var(--space-5);
	}
	.job h3 {
		margin: 0 0 var(--space-1);
		font-size: var(--text-lg);
	}
	.job p {
		margin: 0 0 var(--space-2);
	}
	.job a {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		color: var(--ink);
		font-weight: 700;
	}
	.week {
		list-style: none;
		margin: var(--space-3) 0 var(--space-5);
		padding: 0;
	}
	.week li {
		padding: var(--space-3) 0;
		border-top: var(--border);
	}
	.week b {
		display: block;
		font-size: var(--text-sm);
		color: var(--muted);
	}
	.faq {
		margin: 0;
	}
	.faq dt {
		margin-top: var(--space-4);
		font-weight: 700;
	}
	.faq dd {
		margin: var(--space-1) 0 0;
	}
	.others {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.others a {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-height: var(--tap-min);
		color: var(--ink);
		font-weight: 700;
		text-decoration: none;
	}
</style>
