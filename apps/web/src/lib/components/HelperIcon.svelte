<!--
  A helper's picture: our own flat icon on a brick-colored tile, the same on
  every device. Helpers store an emoji as their avatar; the ones we have an
  icon for (the Shelf and the site's helpers) are drawn, any other emoji is
  shown as it is.
-->
<script lang="ts" module>
	type Icon = { color: 'red' | 'blue' | 'purple' | 'yellow' | 'green' | 'gray'; d: string };

	// 24×24 glyphs, solid fills; holes via evenodd.
	export const ICONS: Record<string, Icon> = {
		// Customer Finder: a magnet
		'🧲': {
			color: 'red',
			d: 'M5 8h4v3.5a3 3 0 0 0 6 0V8h4v3.5a7 7 0 0 1-14 0zM5 3.5h4V7H5zM15 3.5h4V7h-4z'
		},
		// Competitor research: an eye
		'👀': {
			color: 'purple',
			d: 'M1.5 12S5.5 5 12 5s10.5 7 10.5 7-4 7-10.5 7S1.5 12 1.5 12zM12 7.8a4.2 4.2 0 1 0 0 8.4 4.2 4.2 0 0 0 0-8.4zM12 10a2 2 0 1 1 0 4 2 2 0 0 1 0-4z'
		},
		// Morning Briefing: a newspaper
		'📰': {
			color: 'yellow',
			d: 'M3 4.5h14v14a1.5 1.5 0 0 0 1.5 1.5H5a2 2 0 0 1-2-2zM18 8h3v10.5a1.5 1.5 0 0 1-3 0zM6 7.5h8V11H6zM6 13h8v1.5H6zM6 16h8v1.5H6z'
		},
		// Inbox Helper: a tray with an arrow
		'📥': {
			color: 'blue',
			d: 'M11 2.5h2v5.5h3l-4 4.5L8 8h3zM3 14h5l1.5 2.5h5L16 14h5v6.5H3z'
		},
		// Social Poster: a megaphone
		'📣': {
			color: 'green',
			d: 'M3 9.5h4l10-5v15l-10-5H3zM7.5 15.5h3.2l1.3 5H9zM19 10h2.5v4H19z'
		},
		// Receipt Keeper: a till receipt
		'🧾': {
			color: 'green',
			d: 'M5 2.5h14v19l-2.3-1.7-2.4 1.7-2.3-1.7-2.3 1.7-2.4-1.7L5 21.5zM8 6.5h8v1.8H8zM8 10h8v1.8H8zM8 13.5h5v1.8H8z'
		},
		// Weekend Scout: a ticket
		'🎟': {
			color: 'red',
			d: 'M2.5 6.5h19V10a2 2 0 0 0 0 4v3.5h-19V14a2 2 0 0 0 0-4zM15 8h1.5v2H15zM15 11h1.5v2H15zM15 14h1.5v2H15z'
		},
		// Any helper without a picture: three bricks
		'🧱': {
			color: 'gray',
			d: 'M3 14h8.5v5.5H3zM12.5 14H21v5.5h-8.5zM7.5 7.5H16V13H7.5z'
		}
	};

	/** The icon for an avatar emoji, if we draw that one ("🎟️" = "🎟"). */
	export function iconFor(avatar: string | null | undefined): Icon | undefined {
		return ICONS[(avatar ?? '🧱').replace(/️/g, '')];
	}
</script>

<script lang="ts">
	let { avatar, size = 40 }: { avatar?: string | null; size?: number } = $props();
	const icon = $derived(iconFor(avatar));
</script>

{#if icon}
	<span
		class="tile {icon.color}"
		style:width="{size}px"
		style:height="{size}px"
		style:border-radius="{Math.max(4, Math.round(size / 5))}px"
		aria-hidden="true"
	>
		<svg viewBox="0 0 24 24" width={Math.round(size * 0.62)} height={Math.round(size * 0.62)}>
			<path d={icon.d} fill="currentColor" fill-rule="evenodd" />
		</svg>
	</span>
{:else}
	<span class="emoji" style:font-size="{Math.round(size * 0.8)}px" aria-hidden="true">{avatar}</span
	>
{/if}

<style>
	.tile {
		display: inline-grid;
		flex: none;
		place-items: center;
		vertical-align: middle;
	}
	.red {
		background: var(--brick-red);
		color: var(--on-red);
	}
	.blue {
		background: var(--brick-blue);
		color: var(--on-blue);
	}
	.purple {
		background: var(--brick-purple);
		color: var(--on-purple);
	}
	.yellow {
		background: var(--brick-yellow);
		color: var(--on-yellow);
	}
	.green {
		background: var(--brick-green);
		color: var(--on-green);
	}
	.gray {
		background: var(--gray-200);
		color: var(--ink);
	}
	.emoji {
		line-height: 1;
		vertical-align: middle;
	}
</style>
