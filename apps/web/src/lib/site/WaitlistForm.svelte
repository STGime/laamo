<!--
  The waiting-list form: email, optional interests, one button. Solves the
  server's small puzzle first (no captcha), then the API emails a link to
  confirm (double opt-in). A share code from ?ref= rides along.
-->
<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import { solve } from '#lib/demo.ts';
	import type { Lang } from '#lib/i18n/index.svelte.ts';
	import { copy } from './copy.ts';
	import { localPath } from './site.ts';

	type Interest = NonNullable<Schemas['WaitlistJoin']['interests']>[number];

	let { lang, source, id }: { lang: Lang; source: string; id: string } = $props();
	const c = $derived(copy[lang].form);

	let email = $state('');
	let website = $state(''); // honeypot: people never see it
	let picked = $state<Interest[]>([]);
	let wish = $state('');
	let busy = $state(false);
	let sent = $state('');
	let error = $state('');

	const interests: Interest[] = [
		'customers',
		'competitors',
		'social',
		'news',
		'emails',
		'events',
		'other'
	];

	function toggle(i: Interest) {
		picked = picked.includes(i) ? picked.filter((x) => x !== i) : [...picked, i];
	}

	/** The share code someone came with (?ref=…), kept for this visit. */
	function ref(): string | undefined {
		try {
			const r = new URLSearchParams(location.search).get('ref')?.toLowerCase() ?? '';
			if (/^[a-z0-9]{6,12}$/.test(r)) sessionStorage.setItem('laamo.ref', r);
			return sessionStorage.getItem('laamo.ref') ?? undefined;
		} catch {
			return undefined;
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (busy) return;
		busy = true;
		error = '';
		try {
			const ch = await fetch('/api/waitlist/challenge');
			if (!ch.ok) throw new Error(String(ch.status));
			const { token, bits } = (await ch.json()) as Schemas['DemoChallenge'];
			const nonce = await solve(token, bits);
			const body: Schemas['WaitlistJoin'] = {
				email: email.trim(),
				lang,
				interests: picked,
				wish: picked.includes('other') ? wish.trim() || undefined : undefined,
				source,
				ref: ref(),
				website,
				token,
				nonce
			};
			const res = await fetch('/api/waitlist', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			if (res.status === 202) sent = email.trim();
			else if (res.status === 400) error = c.badEmail;
			else if (res.status === 429 || res.status === 503) error = c.slow;
			else error = c.failed;
		} catch {
			error = c.failed;
		} finally {
			busy = false;
		}
	}

	function reset() {
		sent = '';
		email = '';
	}
</script>

{#if sent}
	<div class="sent" role="status">
		<p class="big">{c.sent(sent)}</p>
		<p>{c.sentHint}</p>
		<button type="button" class="link" onclick={reset}>{c.again}</button>
	</div>
{:else}
	<form onsubmit={submit} novalidate>
		<label for="{id}-email">{c.label}</label>
		<input
			id="{id}-email"
			type="email"
			autocomplete="email"
			inputmode="email"
			placeholder={c.placeholder}
			required
			maxlength="254"
			bind:value={email}
		/>
		<!-- Left empty by people; bots fill it in. -->
		<div class="trap" aria-hidden="true">
			<label for="{id}-website">Website</label>
			<input id="{id}-website" tabindex="-1" autocomplete="off" bind:value={website} />
		</div>

		<fieldset>
			<legend>{c.interests}</legend>
			<div class="chips">
				{#each interests as i (i)}
					<button type="button" aria-pressed={picked.includes(i)} onclick={() => toggle(i)}
						>{c.interest[i]}</button
					>
				{/each}
			</div>
			{#if picked.includes('other')}
				<label class="wish" for="{id}-wish">{c.wish}</label>
				<input
					id="{id}-wish"
					placeholder={c.wishPlaceholder}
					maxlength="300"
					autocomplete="off"
					bind:value={wish}
				/>
			{/if}
		</fieldset>

		{#if error}<p class="error" role="alert">{error}</p>{/if}
		<button class="btn primary wide" disabled={busy || !email.trim()}
			>{busy ? c.working : c.button}</button
		>
		<p class="consent">
			{c.consent} <a href={localPath('/privacy', lang)}>{c.privacy}</a>
		</p>
		<noscript><p class="error">{c.noScript}</p></noscript>
	</form>
{/if}

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	label,
	legend {
		font-weight: 700;
	}
	input {
		min-height: 56px;
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font: inherit;
		font-size: var(--text-lg);
	}
	.trap {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}
	fieldset {
		margin: var(--space-2) 0 0;
		padding: 0;
		border: 0;
	}
	legend {
		margin-bottom: var(--space-2);
		padding: 0;
		font-size: var(--text-sm);
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.chips button {
		min-height: 40px;
		padding: 0 var(--space-3);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font-size: var(--text-sm);
		font-weight: 700;
		cursor: pointer;
		transition: transform var(--motion-fast) var(--ease);
	}
	.chips button:active {
		transform: scale(var(--press-scale));
	}
	.chips button[aria-pressed='true'] {
		background: var(--brick-yellow);
		color: var(--on-yellow);
		box-shadow: var(--brick-edge);
	}
	.wish {
		display: block;
		margin: var(--space-3) 0 var(--space-2);
		font-size: var(--text-sm);
	}
	.wish + input {
		width: 100%;
	}
	.consent a {
		color: inherit;
	}
	.consent {
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.error {
		margin: 0;
		color: var(--danger);
		font-weight: 600;
	}
	.sent {
		padding: var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--brick-green);
		color: var(--on-green);
	}
	.sent p {
		margin: 0 0 var(--space-2);
	}
	.sent .big {
		font-weight: 700;
		font-size: var(--text-lg);
	}
	.link {
		min-height: var(--tap-min);
		padding: 0;
		border: 0;
		background: none;
		font-weight: 700;
		text-decoration: underline;
		cursor: pointer;
	}
</style>
