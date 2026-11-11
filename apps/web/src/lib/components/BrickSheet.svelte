<!--
  S10 Brick sheet: a brick's settings, rendered from bricks.json. At most 3
  show; the rest wait behind "More". Text settings with pills get chips.
-->
<script lang="ts">
	import {
		brick,
		catalog,
		matches,
		withDefaults,
		type Setting,
		type StackBrick
	} from '@laamo/catalog';
	import type { Schemas } from '#lib/api/client.ts';
	import BottomSheet from './BottomSheet.svelte';
	import { i18n, t, type Key } from '#lib/i18n/index.svelte.ts';
	import { fieldPills } from '#lib/builder.ts';

	let {
		open = $bindable(false),
		sb = $bindable(),
		places,
		lastOut,
		pills = [],
		back,
		onremove
	}: {
		open?: boolean;
		sb: StackBrick;
		places: Schemas['Place'][];
		lastOut?: number;
		/** Pills that make sense here, e.g. "my city", "title", "answer". */
		pills?: string[];
		/** Where "Connect a chat" comes back to. */
		back: string;
		onremove: () => void;
	} = $props();

	const lang = $derived(i18n.lang);
	const b = $derived(brick(sb.type));
	const cfg = $derived(b ? withDefaults(b, sb.config) : sb.config);
	const shown = $derived((b?.settings ?? []).filter((s) => matches(s.show_if, cfg)));
	// Max 3 visible settings per brick; everything else lives behind More.
	const main = $derived(shown.filter((s) => !s.advanced).slice(0, 3));
	const more = $derived(shown.filter((s) => !main.includes(s)));
	let showMore = $state(false);

	function set(key: string, v: unknown) {
		sb.config = { ...sb.config, [key]: v };
	}
	const str = (v: unknown) => (typeof v === 'string' || typeof v === 'number' ? String(v) : '');
	const list = (v: unknown) => (Array.isArray(v) ? (v as string[]) : []);
	function toggle(key: string, item: string) {
		const l = list(cfg[key]);
		set(key, l.includes(item) ? l.filter((x) => x !== item) : [...l, item]);
	}
	function pill(key: string, p: string) {
		const v = str(cfg[key]);
		set(key, `${v}${v && !v.endsWith(' ') ? ' ' : ''}{${p}}`);
	}
	const fieldLabel = (k: string) => catalog.fields.find((f) => f.key === k)?.label[lang] ?? k;
	const days = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const;
	const placesFor = (s: Setting) =>
		places.filter((p) => p.verified_at && (s.place_types ?? []).includes(p.type as never));
</script>

{#snippet field(s: Setting)}
	{@const id = `set-${sb.id}-${s.key}`}
	<div class="setting">
		<label for={id}>{s.label[lang]}</label>
		{#if s.type === 'select'}
			<div class="choices" {id} role="radiogroup">
				{#each s.options ?? [] as o (String(o.value))}
					<button
						type="button"
						role="radio"
						aria-checked={String(cfg[s.key]) === String(o.value)}
						disabled={o.status === 'later'}
						onclick={() => set(s.key, o.value)}>{o.label[lang]}</button
					>
				{/each}
			</div>
		{:else if s.type === 'days'}
			<div class="choices" {id}>
				{#each days as d (d)}
					<button
						type="button"
						aria-pressed={list(cfg[s.key]).includes(d)}
						onclick={() => toggle(s.key, d)}>{t(`day.${d}` as Key)}</button
					>
				{/each}
			</div>
		{:else if s.type === 'fields'}
			<div class="choices" {id}>
				{#each fieldPills as f (f)}
					<button
						type="button"
						aria-pressed={list(cfg[s.key]).includes(f)}
						onclick={() => toggle(s.key, f)}>{fieldLabel(f)}</button
					>
				{/each}
			</div>
		{:else if s.type === 'place'}
			<div class="choices" {id} role="radiogroup">
				{#each placesFor(s) as p (p.id)}
					<button
						type="button"
						role="radio"
						aria-checked={cfg[s.key] === p.id}
						onclick={() => set(s.key, p.id)}>{p.label}</button
					>
				{/each}
			</div>
			<a class="add" href="/places/new?back={encodeURIComponent(back)}">+ {t('tpl.addChat')}</a>
		{:else if s.type === 'toggle'}
			<input
				{id}
				type="checkbox"
				class="toggle"
				checked={cfg[s.key] === true}
				onchange={(e) => set(s.key, e.currentTarget.checked)}
			/>
		{:else if s.type === 'number'}
			<input
				{id}
				type="number"
				inputmode="numeric"
				min={s.min}
				max={s.max}
				value={str(cfg[s.key])}
				oninput={(e) => {
					const n = e.currentTarget.valueAsNumber;
					set(s.key, Number.isNaN(n) ? undefined : n);
				}}
			/>
		{:else if s.type === 'slider'}
			<input
				{id}
				type="range"
				min={s.min ?? 0}
				max={s.max ?? 1}
				step={s.step ?? 0.1}
				value={str(cfg[s.key])}
				oninput={(e) => set(s.key, e.currentTarget.valueAsNumber)}
			/>
			{#if s.ends}<p class="ends">
					<span>{s.ends[0][lang]}</span><span>{s.ends[1][lang]}</span>
				</p>{/if}
		{:else if s.type === 'time'}
			<input
				{id}
				type="time"
				value={str(cfg[s.key])}
				oninput={(e) => set(s.key, e.currentTarget.value)}
			/>
		{:else if s.type === 'tags'}
			<input
				{id}
				value={list(cfg[s.key]).join(', ')}
				placeholder={s.placeholder?.[lang] ?? ''}
				onchange={(e) =>
					set(
						s.key,
						e.currentTarget.value
							.split(',')
							.map((x) => x.trim())
							.filter(Boolean)
					)}
			/>
		{:else if s.type === 'longtext'}
			<textarea
				{id}
				rows="4"
				maxlength={s.max_length}
				placeholder={s.placeholder?.[lang] ?? ''}
				value={str(cfg[s.key])}
				oninput={(e) => set(s.key, e.currentTarget.value)}></textarea>
		{:else}
			<input
				{id}
				type={s.type === 'url' ? 'url' : s.type === 'email' ? 'email' : 'text'}
				maxlength={s.max_length}
				placeholder={s.placeholder?.[lang] ?? ''}
				value={str(cfg[s.key])}
				oninput={(e) => set(s.key, e.currentTarget.value)}
			/>
		{/if}
		{#if s.pills && pills.length}
			<div class="pills" aria-label={t('builder.pills')}>
				{#each pills as p (p)}
					<button
						type="button"
						class="pill"
						class:mine={p.startsWith('my ')}
						onclick={() => pill(s.key, p)}>{`{${p}}`}</button
					>
				{/each}
			</div>
		{/if}
		{#if s.help}<p class="help">{s.help[lang]}</p>{/if}
	</div>
{/snippet}

<BottomSheet bind:open title={b?.title[lang] ?? sb.type}>
	{#if b}<p class="does">{b.does[lang]}</p>{/if}
	{#if lastOut !== undefined}
		<p class="preview">{t('builder.lastTime', { n: lastOut })}</p>
	{/if}
	{#each main as s (s.key)}{@render field(s)}{/each}
	{#if more.length}
		<button
			type="button"
			class="more"
			aria-expanded={showMore}
			onclick={() => (showMore = !showMore)}
			>{showMore ? t('builder.less') : t('builder.more')}</button
		>
		{#if showMore}{#each more as s (s.key)}{@render field(s)}{/each}{/if}
	{/if}
	{#snippet footer()}
		<div class="actions">
			<button type="button" class="btn" onclick={onremove}>{t('builder.remove')}</button>
			<button type="button" class="btn primary" onclick={() => (open = false)}
				>{t('builder.done')}</button
			>
		</div>
	{/snippet}
</BottomSheet>

<style>
	.does,
	.preview,
	.help {
		margin: 0 0 var(--space-3);
		color: var(--muted);
	}
	.preview {
		font-weight: 700;
		color: var(--ink);
	}
	.setting {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-bottom: var(--space-4);
	}
	label {
		font-weight: 700;
	}
	input:not([type='checkbox'], [type='range']),
	textarea {
		min-height: 56px;
		padding: var(--space-3) var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-lg);
	}
	input[type='range'] {
		accent-color: var(--ink);
	}
	.toggle {
		width: 28px;
		height: 28px;
		accent-color: var(--ink);
	}
	.ends {
		display: flex;
		justify-content: space-between;
		margin: 0;
		color: var(--muted);
		font-size: var(--text-sm);
	}
	.choices,
	.pills {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
	.choices button {
		min-height: var(--tap-min);
		padding: 0 var(--space-4);
		border: 2px solid var(--ink);
		border-radius: var(--radius-control);
		background: var(--white);
		font-weight: 700;
		cursor: pointer;
	}
	.choices button:disabled {
		border-color: var(--gray-200);
		color: var(--muted);
		cursor: default;
	}
	.choices [aria-checked='true'],
	.choices [aria-pressed='true'] {
		background: var(--ink);
		color: var(--white);
	}
	.pill {
		min-height: 36px;
		padding: 0 var(--space-3);
		border: 0;
		border-radius: var(--radius-control);
		background: var(--gray-50);
		font-size: var(--text-sm);
		font-weight: 700;
		cursor: pointer;
	}
	.pill.mine {
		background: var(--brick-purple);
		color: var(--on-purple);
	}
	.add,
	.more {
		align-self: flex-start;
		min-height: var(--tap-min);
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		font-weight: 700;
		text-decoration: none;
		cursor: pointer;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
	}
	.actions .btn {
		flex: 1;
	}
</style>
