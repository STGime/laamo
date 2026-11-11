<!-- S1 Landing: what Laamo does, three real helpers, one action. Signed-in users go straight to the Feed. -->
<script lang="ts">
	import type { Group } from '#lib/components/Brick.svelte';
	import { t, type Key } from '#lib/i18n/index.svelte.ts';

	const examples: { avatar: string; name: Key; promise: Key; bricks: Group[] }[] = [
		{
			avatar: '📰',
			name: 'landing.ex1.name',
			promise: 'landing.ex1.promise',
			bricks: ['when', 'look', 'think', 'do']
		},
		{
			avatar: '🧲',
			name: 'landing.ex2.name',
			promise: 'landing.ex2.promise',
			bricks: ['when', 'look', 'think', 'ask', 'do']
		},
		{
			avatar: '🎟️',
			name: 'landing.ex3.name',
			promise: 'landing.ex3.promise',
			bricks: ['when', 'look', 'ask', 'do']
		}
	];
</script>

<div class="page">
	<header>
		<p class="logo display">Laamo</p>
		<h1>{t('landing.title')}</h1>
		<p class="sub">{t('landing.sub')}</p>
	</header>

	<ul class="examples">
		{#each examples as ex (ex.name)}
			<li>
				<div class="head">
					<span class="avatar" aria-hidden="true">{ex.avatar}</span>
					<h2>{t(ex.name)}</h2>
				</div>
				<p>{t(ex.promise)}</p>
				<div class="mini" aria-hidden="true">
					{#each ex.bricks as g, i (i)}<span class={g}></span>{/each}
				</div>
			</li>
		{/each}
	</ul>

	<p class="eu"><span aria-hidden="true">🇪🇺</span> {t('landing.eu')}</p>

	<div class="cta">
		<a class="btn primary wide" href="/try">{t('landing.try')}</a>
		<a class="signin" href="/start">{t('landing.signIn')}</a>
	</div>
</div>

<style>
	.page {
		padding-bottom: 160px;
	}
	header {
		padding: var(--space-6) var(--gutter) var(--space-5);
	}
	.logo {
		margin: 0 0 var(--space-6);
		font-size: var(--text-h2);
		color: var(--accent);
	}
	h1 {
		font-size: 40px;
	}
	.sub {
		margin: var(--space-3) 0 0;
		font-size: var(--text-lg);
		color: var(--muted);
	}
	.examples {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: var(--border);
	}
	.examples li {
		padding: var(--space-4) var(--gutter);
		border-bottom: var(--border);
	}
	.head {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.avatar {
		font-size: 22px;
	}
	h2 {
		font-size: var(--text-lg);
	}
	.examples p {
		margin: var(--space-1) 0 var(--space-3);
	}
	/* The helper's bricks, as a small flat stack. */
	.mini {
		display: flex;
		gap: 3px;
	}
	.mini span {
		width: 28px;
		height: 12px;
		border-radius: 3px;
		box-shadow: var(--brick-edge);
	}
	.mini .when {
		background: var(--brick-yellow);
	}
	.mini .look {
		background: var(--brick-blue);
	}
	.mini .think {
		background: var(--brick-purple);
	}
	.mini .do {
		background: var(--brick-green);
	}
	.mini .ask {
		background: var(--brick-red);
	}
	.eu {
		margin: 0;
		padding: var(--space-5) var(--gutter);
		font-weight: 700;
	}
	.cta {
		position: fixed;
		bottom: 0;
		left: 50%;
		transform: translateX(-50%);
		width: 100%;
		max-width: var(--column-max);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-3) var(--gutter) calc(var(--space-3) + env(safe-area-inset-bottom));
		background: var(--white);
		border-top: var(--border);
	}
	.signin {
		min-height: var(--tap-min);
		display: inline-flex;
		align-items: center;
		color: var(--ink);
		font-weight: 700;
	}
</style>
