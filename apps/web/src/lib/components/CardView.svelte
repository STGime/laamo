<!-- One card from a pile (a search result, a business, a draft, a photo of a receipt). With `edits`, text fields become editable. -->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { mailto, safeURL } from '#lib/links.ts';
	import { columnLabel, morePictures, osmSource } from '#lib/lists.ts';
	import MapThumb from './MapThumb.svelte';
	import Picture from './Picture.svelte';

	let {
		card,
		edits = $bindable(),
		editing = false
	}: { card: Schemas['Card']; edits?: Record<string, string>; editing?: boolean } = $props();

	const text = (k: string) => {
		const v = card.fields[k];
		return typeof v === 'string' ? v.trim() : '';
	};
	// A photo the helper read (Read the photo), e.g. a receipt: the details
	// pulled out of it. All of them can be corrected or filled in, also the
	// ones that weren't found.
	const RECEIPT = [
		'vendor',
		'date',
		'total',
		'net',
		'vat_rate',
		'vat_amount',
		'currency',
		'invoice_number'
	];
	const photo = $derived(card.kind === 'photo');
	const details = $derived(photo ? RECEIPT.filter((k) => k in card.fields) : []);
	const heading = $derived(
		text('title') ||
			text('name') ||
			text('subject') ||
			text('page_title') ||
			(photo ? text('vendor') || text('image_alt') : '')
	);
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
	const editable = $derived([
		...['email', 'subject', 'title', 'body'].filter((k) => text(k) !== ''),
		...details
	]);
	const label = (k: string) =>
		RECEIPT.includes(k) ? columnLabel(k, i18n.lang) : t(`field.${k}` as Key);
	const untrusted = (k: string) => card.trust[k] === 'untrusted';
	// An email the helper received: who sent it, and whether Laamo could
	// check the sender (anyone can write any From).
	const sender = $derived(
		untrusted('from') && text('from')
			? [text('from_name'), text('from_name') ? `(${text('from')})` : text('from')]
					.filter(Boolean)
					.join(' ')
			: ''
	);
	const senderChecked = $derived(
		text('from_signed') === 'yes' && card.trust.from_signed === 'user_entered'
	);
	// Map data must name its source (OpenStreetMap's licence).
	const fromMap = $derived(osmSource(card.source_url));
	// A business: where it is, how far, when it's open, how to call.
	const business = $derived(card.kind === 'business');
	const location = $derived(text('location'));
	const km = $derived(typeof card.fields.distance_km === 'number' ? card.fields.distance_km : null);
	const from = $derived(text('distance_from'));
	const away = $derived(
		km == null
			? ''
			: km < 1
				? t(from ? 'osm.metersFrom' : 'osm.metersAway', { n: Math.round(km * 1000), place: from })
				: t(from ? 'osm.kmFrom' : 'osm.kmAway', { n: km, place: from })
	);
	const phone = $derived(text('phone'));
	// A draft to someone: open it in the user's own mail app, or copy it.
	const draft = $derived(
		text('body') && email
			? mailto(
					edits?.email ?? email,
					edits?.subject ?? text('subject'),
					edits?.body ?? text('body')
				)
			: null
	);
	let copied = $state(false);
	async function copyDraft() {
		const subject = edits?.subject ?? text('subject');
		await navigator.clipboard?.writeText(
			(subject ? subject + '\n\n' : '') + (edits?.body ?? text('body'))
		);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

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
				<span>{label(k)}</span>
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
		{#if heading}<h3 class="display">
				{edits?.title ?? edits?.subject ?? (photo ? edits?.vendor : undefined) ?? heading}
			</h3>{/if}
		{#if details.length}
			<dl class="details">
				{#each details as k (k)}
					<div>
						<dt>{label(k)}</dt>
						<dd>{value(k) || '–'}</dd>
					</div>
				{/each}
			</dl>
		{/if}
		{#if sender}
			<p class="host">
				✉️ {sender} · {t(senderChecked ? 'feed.senderChecked' : 'feed.senderNotChecked')}
			</p>
		{/if}
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
		{#if draft}
			<div class="send">
				<a class="btn" href={draft}>{t('draft.open')}</a>
				<button type="button" class="btn" onclick={copyDraft}
					>{copied ? t('lists.copied') : t('draft.copy')}</button
				>
			</div>
		{/if}
		{#if text('image')}<Picture path={text('image')} alt={text('image_alt')} />{/if}
		{#each morePictures(card.fields) as more (more)}<Picture path={more} />{/each}
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
	.details {
		display: grid;
		gap: var(--space-1);
		margin: var(--space-1) 0 0;
	}
	.details div {
		display: flex;
		justify-content: space-between;
		gap: var(--space-3);
	}
	.details dt {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.details dd {
		margin: 0;
		font-weight: 600;
		text-align: right;
		overflow-wrap: anywhere;
	}
	.facts {
		color: var(--muted);
	}
	.send {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
</style>
