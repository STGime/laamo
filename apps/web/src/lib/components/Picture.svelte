<!--
  A picture from Draw a picture. It's stored with the run; our server hands
  it out to members of the workspace only, so it loads with the session.
-->
<script lang="ts">
	import { api } from '#lib/session.svelte.ts';

	let { path, alt = '' }: { path: string; alt?: string } = $props();

	let src = $state<string | null>(null);
	$effect(() => {
		let url: string | null = null;
		let gone = false;
		api.GET('/images', { params: { query: { path } }, parseAs: 'blob' }).then(({ data }) => {
			if (gone || !data) return;
			url = URL.createObjectURL(data as Blob);
			src = url;
		});
		return () => {
			gone = true;
			if (url) URL.revokeObjectURL(url);
		};
	});
</script>

{#if src}<img class="picture" {src} {alt} loading="lazy" />{/if}

<style>
	.picture {
		display: block;
		width: 100%;
		height: auto;
		margin-top: var(--space-2);
		border-radius: var(--radius-control);
	}
</style>
