<!-- Bottom tab bar on every screen size: Feed · Helpers · ➕ · Lists · Me -->
<script lang="ts">
	import { page } from '$app/state';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';

	let { needsMe = 0 }: { needsMe?: number } = $props();

	const tabs: { href: string; key: Key; icon: string }[] = [
		{ href: '/feed', key: 'tab.feed', icon: 'M4 6h16M4 12h16M4 18h10' },
		{ href: '/helpers', key: 'tab.helpers', icon: 'M5 5h14v5H5zM5 14h14v5H5z' },
		{ href: '/new', key: 'tab.new', icon: 'M12 5v14M5 12h14' },
		{ href: '/lists', key: 'tab.lists', icon: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01' },
		{ href: '/me', key: 'tab.me', icon: 'M12 12a4 4 0 100-8 4 4 0 000 8zM4 21c1-4 4-6 8-6s7 2 8 6' }
	];
	const active = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(href + '/');
</script>

<nav aria-label="Laamo">
	{#each tabs as tab (tab.href)}
		<a
			href={tab.href}
			class:new={tab.href === '/new'}
			aria-current={active(tab.href) ? 'page' : undefined}
		>
			<span class="icon">
				<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
					<path
						d={tab.icon}
						fill="none"
						stroke="currentColor"
						stroke-width="2.4"
						stroke-linecap="round"
						stroke-linejoin="round"
					/>
				</svg>
				{#if tab.href === '/feed' && needsMe > 0}<span class="badge">{needsMe}</span>{/if}
			</span>
			<span class:visually-hidden={tab.href === '/new'}>{t(tab.key)}</span>
		</a>
	{/each}
</nav>

<style>
	nav {
		position: fixed;
		bottom: 0;
		left: 50%;
		transform: translateX(-50%);
		z-index: 40;
		width: 100%;
		max-width: var(--column-max);
		height: calc(var(--tabbar-height) + env(safe-area-inset-bottom));
		padding-bottom: env(safe-area-inset-bottom);
		display: grid;
		grid-template-columns: repeat(5, 1fr);
		background: var(--white);
		border-top: var(--border);
	}
	a {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		min-height: var(--tap-min);
		color: var(--muted);
		font-size: 12px;
		font-weight: 600;
		text-decoration: none;
		transition: transform var(--motion-fast) var(--ease);
	}
	a:active {
		transform: scale(var(--press-scale));
	}
	a[aria-current='page'] {
		color: var(--ink);
		font-weight: 800;
	}
	.icon {
		position: relative;
		display: flex;
	}
	.new .icon {
		width: 48px;
		height: 40px;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-control);
		background: var(--accent);
		color: var(--ink);
		box-shadow: var(--brick-edge);
	}
	.badge {
		position: absolute;
		top: -6px;
		right: -10px;
		min-width: 18px;
		height: 18px;
		padding: 0 5px;
		border-radius: 9px;
		background: var(--accent);
		color: var(--ink);
		font-size: 11px;
		font-weight: 800;
		line-height: 18px;
		text-align: center;
	}
</style>
