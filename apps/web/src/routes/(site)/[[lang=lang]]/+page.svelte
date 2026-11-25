<!-- The site's home page (prerendered): the problem, how Laamo works, what it
     can do, and the waiting list (top and bottom). Words in lib/site/copy.ts. -->
<script lang="ts">
	import HelperIcon from '#lib/components/HelperIcon.svelte';
	import Brick from '#lib/components/Brick.svelte';
	import { copy, type Stack } from '#lib/site/copy.ts';
	import { INDUSTRIES, INDUSTRY_UI } from '#lib/site/industries.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { jsonLD, localPath, organizationLD, pageURL, SITE_URL } from '#lib/site/site.ts';
	import { UI, USE_CASES } from '#lib/site/usecases.ts';
	import WaitlistForm from '#lib/site/WaitlistForm.svelte';

	let { data } = $props();
	const c = $derived(copy[data.lang]);

	// Structured data: what Laamo is (an app for small businesses), who makes
	// it, and the FAQ, so search engines and AI assistants can say it right.
	const ld = $derived(
		jsonLD({
			'@context': 'https://schema.org',
			'@graph': [
				{
					'@type': 'WebSite',
					'@id': `${SITE_URL}/#website`,
					name: 'Laamo',
					alternateName: `Laamo: ${c.category}`,
					url: pageURL('/', data.lang),
					inLanguage: data.lang,
					publisher: { '@id': `${SITE_URL}/#organization` }
				},
				{
					'@type': 'SoftwareApplication',
					'@id': `${SITE_URL}/#app`,
					name: 'Laamo',
					alternateName: `Laamo: ${c.category}`,
					description: c.meta.description,
					url: pageURL('/', data.lang),
					applicationCategory: 'BusinessApplication',
					operatingSystem: 'Web',
					inLanguage: ['en', 'de'],
					audience: { '@type': 'BusinessAudience', name: 'Small businesses' },
					featureList: USE_CASES.filter((u) => !u.soon).map((u) => u.text[data.lang].name),
					publisher: { '@id': `${SITE_URL}/#organization` }
				},
				organizationLD(),
				{
					'@type': 'FAQPage',
					mainEntity: c.faq.items.map((i) => ({
						'@type': 'Question',
						name: i.q,
						acceptedAnswer: { '@type': 'Answer', text: i.a }
					}))
				}
			]
		})
	);
</script>

<Seo title={c.meta.title} description={c.meta.description} lang={data.lang} path="/" />
<svelte:head>
	{@html `<script type="application/ld+json">${ld}</script>`}
</svelte:head>

{#snippet stack(s: Stack)}
	<div class="stack">
		{#each s as b, i (i)}<Brick group={b.group} title={b.text} />{/each}
	</div>
{/snippet}

<section class="hero">
	<h1>{c.hero.title}</h1>
	<p class="lead">{c.hero.sub}</p>
	<WaitlistForm lang={data.lang} source="home-top" id="top" />
</section>

<section>
	<h2>{c.problem.title}</h2>
	{#each c.problem.body as p, i (i)}<p>{p}</p>{/each}
	<p class="label">{c.problem.stackTitle}</p>
	{@render stack(c.problem.stack)}
</section>

<section>
	<h2>{c.how.title}</h2>
	<ol class="steps">
		{#each c.how.steps as s, i (i)}
			<li>
				<span class="num display">{i + 1}</span>
				<div>
					<h3>{s.title}</h3>
					<p>{s.body}</p>
				</div>
			</li>
		{/each}
	</ol>
	<h3 class="sub">{c.how.bricksTitle}</h3>
	<ul class="legend">
		{#each c.how.bricks as b (b.group)}
			<li>
				<span class="swatch {b.group}" aria-hidden="true"></span>
				<b>{b.name}</b>
				<span>{b.question}</span>
			</li>
		{/each}
	</ul>
</section>

<section>
	<h2>{c.mobile.title}</h2>
	<p>{c.mobile.body}</p>
</section>

<section>
	<h2>{c.social.title}</h2>
	<p>{c.social.body}</p>
	{@render stack(c.social.stack)}
</section>

<section>
	<h2>{c.customers.title}</h2>
	{#each c.customers.examples as ex (ex.name)}
		<div class="example">
			<h3>{ex.name}</h3>
			<p>{ex.body}</p>
			{@render stack(ex.stack)}
		</div>
	{/each}
	<p class="note">{c.customers.note}</p>
</section>

<section>
	<h2>{c.fast.title}</h2>
	<ol class="timeline">
		{#each c.fast.steps as s (s.time)}
			<li><span class="time display">{s.time}</span> {s.text}</li>
		{/each}
	</ol>
	<p class="note">{c.fast.note}</p>
</section>

<section>
	<h2>{c.free.title}</h2>
	<ul class="points">
		{#each c.free.points as p (p.text)}
			<li>
				{p.text}{#if p.soon}<span class="soon">{c.soon}</span>{/if}
			</li>
		{/each}
	</ul>
</section>

<section>
	<h2>{UI[data.lang].listTitle}</h2>
	<ul class="cases">
		{#each USE_CASES as u (u.slug)}
			<li>
				<a href={localPath(`/use-cases/${u.slug}`, data.lang)}
					><HelperIcon avatar={u.avatar} size={28} />
					{u.text[data.lang].name}{#if u.soon}<span class="soon">{c.soon}</span>{/if}</a
				>
			</li>
		{/each}
	</ul>
	<h3 class="kinds">{INDUSTRY_UI[data.lang].listTitle}</h3>
	<ul class="cases">
		{#each INDUSTRIES as ind (ind.slug)}
			<li>
				<a href={localPath(`/for/${ind.slug}`, data.lang)}
					><HelperIcon avatar={ind.avatar} size={28} />
					{ind.text[data.lang].name}</a
				>
			</li>
		{/each}
	</ul>
</section>

<section class="try">
	<h2>{c.tryIt.title}</h2>
	<p>{c.tryIt.body}</p>
	<a class="btn wide" href={localPath('/try', data.lang)}>{c.tryIt.button}</a>
</section>

<section>
	<h2>{c.faq.title}</h2>
	<dl class="faq">
		{#each c.faq.items as i (i.q)}
			<dt>{i.q}</dt>
			<dd>{i.a}</dd>
		{/each}
	</dl>
</section>

<section class="end">
	<h2>{c.end.title}</h2>
	<p>{c.end.body}</p>
	<WaitlistForm lang={data.lang} source="home-bottom" id="bottom" />
</section>

<style>
	section {
		padding: var(--space-7) var(--gutter);
		border-top: var(--border);
	}
	.hero {
		padding-top: var(--space-5);
		border-top: 0;
	}
	h1 {
		font-size: 40px;
	}
	h2 {
		font-size: 30px;
		margin-bottom: var(--space-4);
	}
	h3 {
		margin: 0;
		font-size: var(--text-lg);
	}
	p {
		margin: 0 0 var(--space-4);
	}
	.lead {
		margin: var(--space-4) 0 var(--space-5);
		font-size: var(--text-lg);
		color: var(--muted);
	}
	.label {
		margin-top: var(--space-5);
		font-weight: 700;
	}
	.stack {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin-top: var(--space-3);
	}
	.steps {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.steps li {
		display: flex;
		gap: var(--space-3);
	}
	.steps p {
		margin: var(--space-1) 0 0;
	}
	.num {
		flex: none;
		width: 40px;
		height: 40px;
		display: grid;
		place-items: center;
		border-radius: var(--radius-control);
		background: var(--ink);
		color: var(--white);
		font-size: var(--text-lg);
	}
	.sub {
		margin: var(--space-6) 0 var(--space-3);
	}
	.legend {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.legend li {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-2) 0;
		border-bottom: var(--border);
	}
	.legend b {
		min-width: 80px;
	}
	.legend span:last-child {
		color: var(--muted);
	}
	.swatch {
		flex: none;
		width: 28px;
		height: 20px;
		border-radius: 4px;
		box-shadow: var(--brick-edge);
	}
	.swatch.when {
		background: var(--brick-yellow);
	}
	.swatch.look {
		background: var(--brick-blue);
	}
	.swatch.think {
		background: var(--brick-purple);
	}
	.swatch.do {
		background: var(--brick-green);
	}
	.swatch.ask {
		background: var(--brick-red);
	}
	.soon {
		display: inline-block;
		margin-left: var(--space-2);
		padding: 2px var(--space-2);
		border-radius: 4px;
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-family: var(--font-ui);
		font-size: var(--text-sm);
		font-weight: 700;
		vertical-align: middle;
		letter-spacing: 0;
	}
	.example {
		margin-bottom: var(--space-6);
	}
	.example p {
		margin: var(--space-1) 0 0;
	}
	.note {
		margin: 0;
		color: var(--muted);
	}
	.timeline {
		list-style: none;
		margin: 0 0 var(--space-4);
		padding: 0;
	}
	.timeline li {
		display: flex;
		align-items: baseline;
		gap: var(--space-3);
		padding: var(--space-3) 0;
		border-bottom: var(--border);
		font-weight: 600;
	}
	.time {
		min-width: 56px;
		color: var(--accent);
		font-size: var(--text-h2);
	}
	.points {
		margin: 0;
		padding-left: var(--space-5);
	}
	.points li {
		margin-bottom: var(--space-3);
	}
	.cases {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.cases a {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--tap-min);
		border-bottom: var(--border);
		color: var(--ink);
		font-weight: 700;
		text-decoration: none;
	}
	.try {
		background: var(--gray-50);
	}
	.try .btn {
		border: 2px solid var(--ink);
		background: var(--white);
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
	.end {
		padding-bottom: var(--space-7);
	}
	.kinds {
		margin: var(--space-5) 0 var(--space-2);
		font-size: var(--text-lg);
	}
</style>
