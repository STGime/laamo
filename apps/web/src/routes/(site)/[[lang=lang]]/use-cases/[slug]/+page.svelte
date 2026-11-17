<!-- One use case: the problem, how the helper solves it, its bricks, an
     example result and the waiting list. Words in lib/site/usecases.ts. -->
<script lang="ts">
	import { copy } from '#lib/site/copy.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { localPath } from '#lib/site/site.ts';
	import Stack from '#lib/site/Stack.svelte';
	import { UI, USE_CASES } from '#lib/site/usecases.ts';
	import WaitlistForm from '#lib/site/WaitlistForm.svelte';

	let { data } = $props();
	const u = $derived(data.useCase);
	const t = $derived(u.text[data.lang]);
	const ui = $derived(UI[data.lang]);
	const others = $derived(USE_CASES.filter((o) => o.slug !== u.slug));
</script>

<Seo title={t.metaTitle} description={t.description} lang={data.lang} path="/use-cases/{u.slug}" />

<section class="top">
	<p class="crumb"><a href={localPath('/use-cases', data.lang)}>{ui.all}</a></p>
	<p class="name">
		<span aria-hidden="true">{u.avatar}</span>
		{t.name}{#if u.soon}<span class="soon">{copy[data.lang].soon}</span>{/if}
	</p>
	<h1>{t.title}</h1>
	<p class="lead">{t.description}</p>
</section>

<section>
	<h2>{ui.problem}</h2>
	<p>{t.problem}</p>
	<h2 class="next">{ui.how}</h2>
	<p>{t.how}</p>
	<Stack stack={t.stack} />
</section>

<section>
	<h2>{ui.result}</h2>
	<div class="card">
		<p class="card-head"><span aria-hidden="true">{u.avatar}</span> {t.name}</p>
		<p class="card-title">{t.result.title}</p>
		{#each t.result.lines as line, i (i)}<p>{line}</p>{/each}
	</div>
	<h2 class="next">{ui.setup}</h2>
	<ol class="questions">
		{#each t.questions as q, i (i)}<li>{q}</li>{/each}
	</ol>
	{#each t.notes as n, i (i)}<p class="note">{n}</p>{/each}
	{#if u.demo}<a class="btn wide outline" href="/try">{ui.tryIt}</a>{/if}
</section>

<section>
	<h2>{ui.join}</h2>
	<p>{ui.joinBody}</p>
	<WaitlistForm lang={data.lang} source="use-case:{u.slug}" id="uc" />
</section>

<section>
	<h2>{ui.others}</h2>
	<ul class="others">
		{#each others as o (o.slug)}
			<li>
				<a href={localPath(`/use-cases/${o.slug}`, data.lang)}
					><span aria-hidden="true">{o.avatar}</span> {o.text[data.lang].name}</a
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
		margin: 0 0 var(--space-2);
		font-weight: 700;
	}
	h1 {
		font-size: 36px;
	}
	h2 {
		margin-bottom: var(--space-3);
	}
	.next {
		margin-top: var(--space-6);
	}
	p {
		margin: 0 0 var(--space-3);
	}
	.lead {
		margin: var(--space-4) 0 0;
		font-size: var(--text-lg);
		color: var(--muted);
	}
	.soon {
		margin-left: var(--space-2);
		padding: 2px var(--space-2);
		border-radius: 4px;
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-size: var(--text-sm);
	}
	/* An example result, like a Feed item: flat, edge to edge, thin rule. */
	.card {
		padding: var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
	}
	.card p {
		margin: 0 0 var(--space-2);
	}
	.card-head {
		color: var(--muted);
		font-size: var(--text-sm);
		font-weight: 600;
	}
	.card-title {
		font-family: var(--font-display);
		font-weight: 800;
		font-size: var(--text-lg);
	}
	.questions {
		margin: 0 0 var(--space-4);
		padding-left: var(--space-5);
	}
	.questions li {
		margin-bottom: var(--space-2);
	}
	.note {
		color: var(--muted);
	}
	.outline {
		margin-top: var(--space-3);
		border: 2px solid var(--ink);
		background: var(--white);
	}
	.others {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.others a {
		display: flex;
		gap: var(--space-2);
		min-height: var(--tap-min);
		align-items: center;
		border-bottom: var(--border);
		color: var(--ink);
		font-weight: 700;
		text-decoration: none;
	}
</style>
