<!-- The link from the confirmation email lands here. Confirming takes a tap,
     so mail apps that open links to scan them can't confirm on their own. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { copy } from '#lib/site/copy.ts';
	import Seo from '#lib/site/Seo.svelte';
	import { localPath, SITE_URL } from '#lib/site/site.ts';

	let { data } = $props();
	const c = $derived(copy[data.lang].confirm);

	let token = $state('');
	let ready = $state(false);
	let busy = $state(false);
	let refCode = $state('');
	let bad = $state(false);
	let copied = $state(false);
	let canShare = $state(false);

	$effect(() => {
		token = new URLSearchParams(location.search).get('t') ?? '';
		canShare = typeof navigator.share === 'function';
		ready = true;
	});

	const shareURL = $derived(
		refCode ? `${SITE_URL}${localPath('/', data.lang)}?ref=${refCode}` : ''
	);

	async function confirm() {
		busy = true;
		try {
			const res = await fetch('/api/waitlist/confirm', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ token })
			});
			if (res.ok) {
				refCode = ((await res.json()) as Schemas['WaitlistConfirmed']).ref_code;
				// The link is used up: don't keep the token in the address bar.
				history.replaceState(null, '', location.pathname);
			} else bad = true;
		} catch {
			bad = true;
		} finally {
			busy = false;
		}
	}

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(shareURL);
			copied = true;
		} catch {
			/* clipboard blocked: the link is on screen */
		}
	}

	function share() {
		navigator.share?.({ title: 'Laamo', url: shareURL }).catch(() => {});
	}
</script>

<Seo title={c.metaTitle} description={c.body} lang={data.lang} path="/waitlist/confirm" noindex />

<section>
	{#if refCode}
		<h1>{c.doneTitle}</h1>
		<p class="lead">{c.doneBody}</p>
		<div class="share">
			<h2>{c.shareTitle}</h2>
			<p>{c.shareBody}</p>
			<p class="url">{shareURL}</p>
			<div class="actions">
				<button type="button" class="btn primary wide" onclick={copyLink}
					>{copied ? c.copied : c.copy}</button
				>
				{#if canShare}<button type="button" class="btn wide" onclick={share}>{c.share}</button>{/if}
			</div>
		</div>
	{:else if bad}
		<h1>{c.badTitle}</h1>
		<p class="lead">{c.badBody}</p>
		<a class="btn primary wide" href={localPath('/', data.lang)}>{c.home}</a>
	{:else}
		<h1>{c.title}</h1>
		<p class="lead">{c.body}</p>
		{#if ready && !token}
			<p class="lead">{c.noLink}</p>
			<a class="btn wide" href={localPath('/', data.lang)}>{c.home}</a>
		{:else}
			<button type="button" class="btn primary wide" disabled={!ready || busy} onclick={confirm}
				>{busy ? c.working : c.button}</button
			>
		{/if}
	{/if}
</section>

<style>
	section {
		padding: var(--space-6) var(--gutter) var(--space-7);
	}
	h1 {
		font-size: 40px;
	}
	.lead {
		margin: var(--space-4) 0 var(--space-5);
		font-size: var(--text-lg);
		color: var(--muted);
	}
	.share {
		margin-top: var(--space-6);
		padding-top: var(--space-5);
		border-top: var(--border);
	}
	.share p {
		margin: var(--space-2) 0;
	}
	.url {
		padding: var(--space-3);
		border-radius: var(--radius-control);
		background: var(--gray-50);
		font-weight: 600;
		word-break: break-all;
	}
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-top: var(--space-4);
	}
	.actions .btn:not(.primary) {
		border: 2px solid var(--ink);
		background: var(--white);
	}
</style>
