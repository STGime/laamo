<!--
  A helper's own email address (Email this helper), with Copy. Before the
  helper is saved there's none yet: we say when it comes. Folded away below
  it: the forwarding address for automatic forwarding rules (auto-forwarded
  mail keeps its original sender, so the sender list would stop it), with
  New address for when it got out.
-->
<script lang="ts">
	import { t } from '#lib/i18n/index.svelte.ts';
	import { api } from '#lib/session.svelte.ts';

	let {
		address,
		forward,
		helperId
	}: { address?: string | null; forward?: string | null; helperId?: string } = $props();

	// A new forwarding address replaces the one we were given.
	let renewed = $state<string | null>(null);
	const fwd = $derived(renewed ?? forward);

	let copied = $state<string | null>(null);
	async function copy(a: string | null | undefined) {
		if (!a) return;
		await navigator.clipboard?.writeText(a);
		copied = a;
		setTimeout(() => (copied = null), 2000);
	}

	let busy = $state(false);
	let failed = $state(false);
	async function renew() {
		if (!helperId || busy || !confirm(t('mail.forwardNewConfirm'))) return;
		busy = true;
		failed = false;
		try {
			const { data } = await api.POST('/helpers/{helperId}/forward-address', {
				params: { path: { helperId } }
			});
			if (data?.forward_address) renewed = data.forward_address;
			else failed = true;
		} catch {
			failed = true;
		} finally {
			busy = false;
		}
	}
</script>

<div class="address">
	<p class="label">{t('mail.address')}</p>
	{#if address}
		<div class="row">
			<code>{address}</code>
			<button type="button" class="btn" onclick={() => copy(address)}
				>{copied === address ? t('mail.copied') : t('mail.copy')}</button
			>
		</div>
		<p class="hint">{t('mail.hint')}</p>
		{#if fwd}
			<details>
				<summary>{t('mail.forwardTitle')}</summary>
				<p class="hint">{t('mail.forwardHint')}</p>
				<div class="row">
					<code>{fwd}</code>
					<button type="button" class="btn" onclick={() => copy(fwd)}
						>{copied === fwd ? t('mail.copied') : t('mail.copy')}</button
					>
				</div>
				<p class="hint">{t('mail.forwardPrivate')}</p>
				{#if helperId}
					<button type="button" class="btn" disabled={busy} onclick={renew}
						>{t('mail.forwardNew')}</button
					>
				{/if}
				{#if failed}<p class="hint" role="alert">{t('mail.forwardNewFailed')}</p>{/if}
			</details>
		{/if}
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
	details {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin-top: var(--space-1);
	}
	details[open] > * + * {
		margin-top: var(--space-1);
	}
	summary {
		cursor: pointer;
		font-weight: 700;
	}
	details .btn {
		align-self: flex-start;
	}
	.hint {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
