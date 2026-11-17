<!-- Shared by the public site (prerendered HTML) and the app (browser only). -->
<script lang="ts">
	import '#lib/styles/tokens.css';

	let { children } = $props();
</script>

{@render children()}
