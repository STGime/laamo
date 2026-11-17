<!-- All use cases: one line each, linking to its page. -->
<script lang="ts">
	import { copy } from '#lib/site/copy.ts';
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
				<span class="avatar" aria-hidden="true">{u.avatar}</span>
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
		margin: 0 0 var(--space-7);
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
	.avatar {
		font-size: 28px;
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
	.soon {
		margin-left: var(--space-2);
		padding: 2px var(--space-2);
		border-radius: 4px;
		background: var(--brick-yellow);
		color: var(--on-yellow);
		font-size: var(--text-sm);
	}
</style>
