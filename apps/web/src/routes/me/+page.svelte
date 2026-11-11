<script lang="ts">
	import { goto } from '$app/navigation';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import { i18n, t, type Lang } from '#lib/i18n/index.svelte.ts';
	import { api, session } from '#lib/session.svelte.ts';

	async function setLang(lang: Lang) {
		i18n.lang = lang;
		const ws = session.workspace;
		if (ws && ws.role === 'owner') {
			await api.PATCH('/workspaces/{workspaceId}', {
				params: { path: { workspaceId: ws.id } },
				body: { locale: lang }
			});
			await session.refresh();
		}
	}
	function signOut() {
		session.signOut();
		goto('/signin');
	}
</script>

<PageHeader title={t('me.title')} />

<dl class="rows">
	<div>
		<dt>{t('me.workspace')}</dt>
		<dd>{session.workspace?.name ?? '–'}</dd>
	</div>
	<div>
		<dt>{t('me.language')}</dt>
		<dd class="seg" role="group" aria-label={t('me.language')}>
			<button type="button" aria-pressed={i18n.lang === 'de'} onclick={() => setLang('de')}
				>Deutsch</button
			>
			<button type="button" aria-pressed={i18n.lang === 'en'} onclick={() => setLang('en')}
				>English</button
			>
		</dd>
	</div>
</dl>
<p class="who">{t('me.signedInAs', { email: session.me?.user.email ?? '' })}</p>
<div class="actions">
	<button type="button" class="btn wide" onclick={signOut}>{t('me.signOut')}</button>
	{#if import.meta.env.DEV}<a class="kit" href="/kit">Component kit</a>{/if}
</div>

<style>
	.rows {
		margin: 0;
	}
	.rows div {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-height: 56px;
		padding: 0 var(--gutter);
		border-bottom: var(--border);
	}
	dt {
		font-weight: 700;
	}
	dd {
		margin: 0;
		color: var(--muted);
	}
	.seg {
		display: flex;
		gap: var(--space-1);
	}
	.seg button {
		min-height: 40px;
		padding: 0 var(--space-3);
		border: 0;
		border-radius: var(--radius-small);
		background: var(--gray-50);
		font-weight: 700;
		cursor: pointer;
	}
	.seg button[aria-pressed='true'] {
		background: var(--ink);
		color: var(--white);
	}
	.who {
		padding: var(--space-4) var(--gutter) 0;
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-4) var(--gutter);
	}
	.kit {
		color: var(--muted);
		font-size: var(--text-sm);
	}
</style>
