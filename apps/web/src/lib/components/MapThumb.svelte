<!--
  A small map with a pin for a business card. The picture comes from our
  server (the map service's key stays there) and loads when it's shown.
-->
<script lang="ts">
	import { t } from '#lib/i18n/index.svelte.ts';
	import { api } from '#lib/session.svelte.ts';

	let { location, href }: { location: string; href?: string | null } = $props();

	let src = $state<string | null>(null);
	let failed = $state(false);

	$effect(() => {
		const [lat, lon] = location.split(',').map(Number);
		if (!Number.isFinite(lat) || !Number.isFinite(lon)) return;
		let url: string | null = null;
		let gone = false;
		api
			.GET('/map', { params: { query: { lat, lon } }, parseAs: 'blob' })
			.then(({ data }) => {
				if (gone || !data) return void (failed = !data);
				url = URL.createObjectURL(data as Blob);
				src = url;
			})
			.catch(() => (failed = true));
		return () => {
			gone = true;
			if (url) URL.revokeObjectURL(url);
		};
	});
</script>

{#if src && !failed}
	<a class="map" href={href ?? undefined} target="_blank" rel="noopener noreferrer nofollow">
		<img {src} alt={t('osm.mapAlt')} width="560" height="200" loading="lazy" />
	</a>
{/if}

<style>
	.map {
		display: block;
		margin-top: var(--space-2);
		border: var(--border);
		border-radius: var(--radius-control);
		overflow: hidden;
	}
	img {
		display: block;
		width: 100%;
		height: auto;
	}
</style>
