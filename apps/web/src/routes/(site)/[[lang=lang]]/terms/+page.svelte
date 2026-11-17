<script lang="ts">
	import LegalPage from '#lib/site/LegalPage.svelte';

	let { data } = $props();
</script>

<LegalPage lang={data.lang} slug="terms" />
