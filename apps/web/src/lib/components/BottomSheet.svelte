<!--
  Bottom sheet: slides up inside the app column. Escape, the scrim or the
  close button close it; focus moves in on open and back on close.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '#lib/i18n/index.svelte.ts';

	let {
		open = $bindable(false),
		title,
		children,
		footer
	}: { open?: boolean; title: string; children: Snippet; footer?: Snippet } = $props();

	let panel = $state<HTMLElement>();
	let opener: Element | null = null;

	$effect(() => {
		if (open) {
			opener = document.activeElement;
			panel?.focus();
			document.body.style.overflow = 'hidden';
			return () => {
				document.body.style.overflow = '';
				(opener as HTMLElement | null)?.focus?.();
			};
		}
	});

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') open = false;
	}
</script>

{#if open}
	<div class="layer">
		<button
			class="scrim"
			type="button"
			tabindex="-1"
			aria-label={t('sheet.close')}
			onclick={() => (open = false)}
		></button>
		<div
			class="sheet"
			role="dialog"
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			bind:this={panel}
			{onkeydown}
		>
			<header>
				<h2>{title}</h2>
				<button
					class="close"
					type="button"
					aria-label={t('sheet.close')}
					onclick={() => (open = false)}
				>
					<svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true"
						><path
							d="M6 6l12 12M18 6L6 18"
							stroke="currentColor"
							stroke-width="2.5"
							stroke-linecap="round"
						/></svg
					>
				</button>
			</header>
			<div class="body">{@render children()}</div>
			{#if footer}<footer>{@render footer()}</footer>{/if}
		</div>
	</div>
{/if}

<style>
	.layer {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		justify-content: center;
		align-items: flex-end;
	}
	.scrim {
		position: absolute;
		inset: 0;
		border: 0;
		background: var(--scrim);
		animation: fade var(--motion-base) var(--ease);
	}
	.sheet {
		position: relative;
		width: 100%;
		max-width: var(--column-max);
		max-height: 88dvh;
		display: flex;
		flex-direction: column;
		background: var(--white);
		border-radius: var(--radius-sheet) var(--radius-sheet) 0 0;
		animation: up var(--motion-base) var(--ease);
		outline: none;
	}
	header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-4) var(--gutter) var(--space-2);
	}
	.close {
		width: var(--tap-min);
		height: var(--tap-min);
		margin-right: calc(var(--space-3) * -1);
		border: 0;
		background: none;
		cursor: pointer;
	}
	.body {
		overflow-y: auto;
		padding: 0 var(--gutter) var(--space-5);
	}
	footer {
		padding: var(--space-3) var(--gutter) calc(var(--space-3) + env(safe-area-inset-bottom));
		border-top: var(--border);
	}
	@keyframes up {
		from {
			transform: translateY(100%);
		}
	}
	@keyframes fade {
		from {
			opacity: 0;
		}
	}
</style>
