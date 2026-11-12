<!-- A list someone shared: read-only, no sign-in. -->
<script lang="ts">
	import { page } from '$app/state';
	import { api, type Schemas } from '#lib/api/client.ts';
	import { i18n, t } from '#lib/i18n/index.svelte.ts';
	import { cellHref, cellText, columnLabel, rowHeading } from '#lib/lists.ts';

	let list = $state<Schemas['SharedList'] | null>(null);
	let missing = $state(false);

	$effect(() => {
		const token = page.params.token ?? '';
		api.GET('/shared/lists/{token}', { params: { path: { token } } }).then(({ data }) => {
			if (data) list = data;
			else missing = true;
		});
	});
	const columns = $derived(
		(list?.columns ?? []).map((c) => c.key).filter((k) => columnLabel(k, i18n.lang))
	);
</script>

<svelte:head><meta name="robots" content="noindex" /></svelte:head>

<main>
	{#if missing}
		<h1>{t('lists.sharedGone')}</h1>
	{:else if list}
		<h1>{list.name}</h1>
		<p class="hint">{t('lists.sharedBy')}</p>
		<ul>
			{#each list.rows as r (r.id)}
				{@const heading = rowHeading(r.data)}
				<li>
					{#if heading}<strong>{heading}</strong>{/if}
					<dl>
						{#each columns as c (c)}
							{#if r.data[c] != null && r.data[c] !== '' && r.data[c] !== heading}
								{@const href = cellHref(c, r.data[c])}
								<dt>{columnLabel(c, i18n.lang)}</dt>
								<dd>
									{#if href}<a {href} target="_blank" rel="noopener noreferrer nofollow"
											>{cellText(r.data[c])}</a
										>{:else}{cellText(r.data[c])}{/if}
								</dd>
							{/if}
						{/each}
					</dl>
				</li>
			{/each}
		</ul>
	{/if}
</main>

<style>
	main {
		padding: var(--space-6) var(--gutter);
	}
	h1 {
		margin: 0;
	}
	.hint {
		color: var(--muted);
		font-size: var(--text-sm);
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	li {
		padding: var(--space-3) 0;
		border-bottom: var(--border);
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 2px var(--space-3);
		margin: var(--space-1) 0 0;
		font-size: var(--text-sm);
	}
	dt {
		color: var(--muted);
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
</style>
