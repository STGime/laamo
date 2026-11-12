<!-- One card from a pile (a search result, a business, a draft). With `edits`, text fields become editable. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';
	import { safeURL } from '#lib/links.ts';
	import { osmSource } from '#lib/lists.ts';
	import MapThumb from './MapThumb.svelte';

	let {
		card,
		edits = $bindable(),
		editing = false
	}: { card: Schemas['Card']; edits?: Record<string, string>; editing?: boolean } = $props();

	const text = (k: string) => {
		const v = card.fields[k];
		return typeof v === 'string' ? v.trim() : '';
	};
	const heading = $derived(text('title') || text('name') || text('subject') || text('page_title'));
	const body = $derived(text('summary') || text('body') || text('snippet') || text('page_text'));
	const email = $derived(text('email'));
	const host = $derived.by(() => {
		const u = text('url') || text('website');
		try {
			return u
				? new URL(u.includes('://') ? u : `https://${u}`).hostname.replace(/^www\./, '')
				: '';
		} catch {
			return '';
		}
	});
	// Fields a person may change before something goes out.
	const href = $derived(safeURL(text('url') || text('website')));
	const editable = $derived(['email', 'subject', 'title', 'body'].filter((k) => text(k) !== ''));
	const untrusted = (k: string) => card.trust[k] === 'untrusted';
	// Map data must name its source (OpenStreetMap's licence).
	const fromMap = $derived(osmSource(card.source_url));
	// A business: where it is, how far, when it's open, how to call.
	const business = $derived(card.kind === 'business');
	const location = $derived(text('location'));
	const km = $derived(typeof card.fields.distance_km === 'number' ? card.fields.distance_km : null);
	const away = $derived(
		km == null
			? ''
			: km < 1
				? t('osm.metersAway', { n: Math.round(km * 1000) })
				: t('osm.kmAway', { n: km })
	);
	const phone = $derived(text('phone'));

	function value(k: string) {
		return edits?.[k] ?? text(k);
	}
	function set(k: string, v: string) {
		const next = { ...(edits ?? {}) };
		if (v === text(k)) delete next[k];
		else next[k] = v;
		edits = next;
	}
</script>

<div class="card">
	{#if editing}
		{#each editable as k (k)}
			<label>
				<span>{t(`field.${k}` as Key)}</span>
				{#if k === 'body'}
					<textarea rows="5" value={value(k)} oninput={(e) => set(k, e.currentTarget.value)}
					></textarea>
				{:else}
					<input
						type={k === 'email' ? 'email' : 'text'}
						value={value(k)}
						oninput={(e) => set(k, e.currentTarget.value)}
					/>
				{/if}
			</label>
		{/each}
	{:else}
		{#if heading}<h3 class="display">{edits?.title ?? edits?.subject ?? heading}</h3>{/if}
		{#if business}
			{#if text('address') || away}
				<p class="where">{[text('address'), away].filter(Boolean).join(' · ')}</p>
			{/if}
			{#if phone || text('opening_hours')}
				<p class="facts">
					{#if phone}<a href="tel:{phone.replace(/[^\d+]/g, '')}">{phone}</a>{/if}
					{#if phone && text('opening_hours')}·{/if}
					{text('opening_hours')}
				</p>
			{/if}
		{/if}
		{#if email}<p class="email">→ {edits?.email ?? email}</p>{/if}
		{#if body}<p class="body">
				{(edits?.body ?? body).slice(0, 320)}{body.length > 320 ? '…' : ''}
			</p>{/if}
		{#if host}
			<p class="host">
				{#if href}<a {href} target="_blank" rel="noopener noreferrer nofollow">{host} ↗</a
					>{:else}{host}{/if}{#if untrusted('url') || untrusted('title')}&nbsp;· {t(
						'feed.fromWeb'
					)}{/if}
			</p>
		{/if}
		{#if business && location}<MapThumb {location} href={fromMap} />{/if}
		{#if fromMap}
			<p class="host">
				<a href={fromMap} target="_blank" rel="noopener noreferrer nofollow">{t('osm.credit')}</a>
				·
				<a href="https://www.geoapify.com/" target="_blank" rel="noopener noreferrer"
					>{t('osm.poweredBy')}</a
				>
			</p>
		{/if}
	{/if}
</div>

<style>
	.card {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	h3 {
		margin: 0;
		font-size: var(--text-lg);
	}
	p {
		margin: 0;
	}
	.body {
		white-space: pre-line;
	}
	.email {
		font-weight: 700;
	}
	.host {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.host a {
		color: var(--ink);
		font-weight: 600;
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-sm);
		font-weight: 700;
	}
	input,
	textarea {
		min-height: var(--tap-min);
		padding: var(--space-2) var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-weight: 400;
		text-transform: none;
	}
	.where,
	.facts {
		margin: var(--space-1) 0 0;
		font-size: var(--text-sm);
	}
	.facts {
		color: var(--muted);
	}
</style>
