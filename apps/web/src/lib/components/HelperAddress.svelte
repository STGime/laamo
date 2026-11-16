<!--
  A helper's own email address (Email this helper), with Copy. Before the
  helper is saved there's none yet: we say when it comes.
-->
<script lang="ts">
	import { t } from '#lib/i18n/index.svelte.ts';

	let { address }: { address?: string | null } = $props();

	let copied = $state(false);
	async function copy() {
		if (!address) return;
		await navigator.clipboard?.writeText(address);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}
</script>

<div class="address">
	<p class="label">{t('mail.address')}</p>
	{#if address}
		<div class="row">
			<code>{address}</code>
			<button type="button" class="btn" onclick={copy}
				>{copied ? t('mail.copied') : t('mail.copy')}</button
			>
		</div>
		<p class="hint">{t('mail.hint')}</p>
	{:else}
		<p class="hint">{t('mail.afterSave')}</p>
	{/if}
</div>

<style>
	.address {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		padding: var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
	}
	.label {
		margin: 0;
		font-weight: 700;
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex-wrap: wrap;
	}
	code {
		flex: 1 1 12rem;
		min-width: 0;
		overflow-wrap: anywhere;
		font-size: var(--text-body);
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
