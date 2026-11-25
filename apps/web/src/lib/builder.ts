// The brick builder (#39): what the tray offers, where a new brick goes,
// and the rules that keep a stack sensible while it's dragged around.
import {
	brick,
	catalog,
	withDefaults,
	type Brick,
	type GroupId,
	type StackBrick,
	type Text
} from '@laamo/catalog';
import { available, validate } from '@laamo/catalog/validate';

/** Bricks the worker can run today. The rest arrive with M4 and show as "Soon". */
export const ready = new Set([
	'when.every',
	'when.tap',
	'when.email',
	'look.search',
	'look.read_page',
	'look.find_businesses',
	'look.my_stuff',
	'look.read_photo',
	'think.summarize',
	'think.keep',
	'think.extract',
	'think.write',
	'think.draw',
	'do.list',
	'do.chat',
	'do.social',
	'ask.check',
	'ask.pick',
	'ask.question',
	'ask.stop'
]);

export const groups: GroupId[] = ['when', 'look', 'think', 'do', 'ask'];

/** The tray for one color: every brick of it, ready ones first. */
export function tray(group: GroupId): Brick[] {
	return catalog.bricks
		.filter((b) => b.group === group && b.status === 'mvp')
		.toSorted((a, b) => Number(ready.has(b.id)) - Number(ready.has(a.id)));
}

const isStart = (sb: StackBrick) => brick(sb.type)?.shape === 'start';

/** A When brick only sits on top: after a drag, it goes back up. */
export function startOnTop(bricks: StackBrick[]): StackBrick[] {
	return [...bricks.filter(isStart), ...bricks.filter((b) => !isStart(b))];
}

/** A fresh id that isn't in the stack yet ("b7"). */
export function nextId(bricks: StackBrick[]): string {
	let n = bricks.length + 1;
	while (bricks.some((b) => b.id === `b${n}`)) n++;
	return `b${n}`;
}

/**
 * Adds a brick from the tray: a When brick replaces the old one on top
 * (a helper starts one way), anything else goes at the end. Email this
 * helper starts as "only from me" (ownEmail): its address is public, so
 * strangers get in only when the user clears the list on purpose.
 */
export function add(
	bricks: StackBrick[],
	type: string,
	ownEmail?: string
): { bricks: StackBrick[]; id: string } {
	const b = brick(type);
	if (!b) return { bricks, id: '' };
	const sb: StackBrick = { id: nextId(bricks), type, config: withDefaults(b, {}) };
	if (type === 'when.email' && ownEmail) sb.config = { ...sb.config, senders: [ownEmail] };
	if (b.shape === 'start') return { bricks: [sb, ...bricks.filter((x) => !isStart(x))], id: sb.id };
	// Write it or Draw a picture with no cards before it: one in total.
	if (alone.has(type) && !cardsBefore(bricks, bricks.length))
		sb.config = { ...sb.config, mode: 'all' };
	return { bricks: fitSettings([...bricks, sb], sb.id), id: sb.id };
}

/** Card fields worth a pill (the rest are internal, like page_text). */
export const fieldPills = catalog.fields.filter((f) => f.pick).map((f) => f.key);

/** "Opening days" → "opening_days": a detail of the user's own for Pull out details. */
export function detailKey(text: string): string {
	const key = text
		.toLowerCase()
		.replace(/ä/g, 'ae')
		.replace(/ö/g, 'oe')
		.replace(/ü/g, 'ue')
		.replace(/ß/g, 'ss')
		.normalize('NFKD')
		.replace(/[^a-z0-9]+/g, '_')
		.replace(/^_+|_+$/g, '')
		.slice(0, 40)
		.replace(/_+$/, '');
	if (!key) return '';
	return /^[a-z]/.test(key) ? key : `x_${key}`;
}

/** {my city} → Leipzig where the fact is known (as on the Shelf). */
export function fillProfile(text: string, profile: Record<string, unknown>): string {
	return text.replace(/\{my ([a-z_]+)\}/g, (m, key: string) => {
		const v = profile[key];
		return typeof v === 'string' && v.trim() ? v : m;
	});
}

/**
 * One-tap fix (#41): puts the suggested brick right before the one that
 * needs it, e.g. Check first before Post to chat. A When brick goes on top.
 */
export function applyFix(
	bricks: StackBrick[],
	fix: { insert: string; config?: Record<string, unknown>; before: string }
): { bricks: StackBrick[]; id: string } {
	const b = brick(fix.insert);
	if (!b) return { bricks, id: '' };
	const sb: StackBrick = {
		id: nextId(bricks),
		type: fix.insert,
		config: withDefaults(b, fix.config ?? {})
	};
	const at = bricks.findIndex((x) => x.id === fix.before);
	const out = [...bricks];
	out.splice(at < 0 ? out.length : at, 0, sb);
	return { bricks: startOnTop(out), id: sb.id };
}

/**
 * A setting change that removes a "needs {field}" problem: another option
 * of the select that asked for the field, whose own needs the cards before
 * it do meet. E.g. Save to a list after a web search: "Same website" →
 * "Same link". Options that still check something come before ones that
 * don't ("Keep everything").
 */
export function settingFix(
	bricks: StackBrick[],
	brickId: string,
	field: string
): { key: string; value: unknown; label: Text } | null {
	const i = bricks.findIndex((x) => x.id === brickId);
	const sb = bricks[i];
	const b = sb && brick(sb.type);
	if (!b?.needs_if) return null;
	const have = available({ schema: 1, bricks }, i).fields;
	const cfg = withDefaults(b, sb.config);
	for (const n of b.needs_if) {
		const keys = Object.keys(n.if);
		if (!n.fields?.includes(field) || keys.length !== 1) continue;
		const key = keys[0];
		const set = b.settings.find((x) => x.key === key && x.type === 'select');
		if (!set?.options) continue;
		const needsOf = (v: unknown) =>
			b.needs_if!.filter((m) => {
				const want = m.if[key];
				return Object.keys(m.if).length === 1 && Array.isArray(want) && want.includes(v as never);
			});
		const fits = set.options.filter(
			(o) =>
				o.status !== 'later' &&
				o.value !== cfg[key] &&
				needsOf(o.value).every((m) => (m.fields ?? []).every((f) => have.includes(f)))
		);
		const best = fits.find((o) => needsOf(o.value).length > 0) ?? fits[0];
		if (best) return { key, value: best.value, label: best.label };
	}
	return null;
}

/** A brick just added gets settings that fit the cards before it (see settingFix). */
export function fitSettings(bricks: StackBrick[], id: string): StackBrick[] {
	let out = bricks;
	for (const issue of validate({ schema: 1, bricks: out })) {
		if (issue.brick !== id || issue.code !== 'missing_field' || !issue.field) continue;
		const f = settingFix(out, id, issue.field);
		if (f)
			out = out.map((x) => (x.id === id ? { ...x, config: { ...x.config, [f.key]: f.value } } : x));
	}
	return out;
}

/** Bricks that, set to one in total, make a card with none before them. */
export const alone = new Set(['think.write', 'think.draw']);
const makesOne = (b: StackBrick) =>
	alone.has(b.type) && withDefaults(brick(b.type)!, b.config).mode === 'all';

/**
 * Whether any brick before index makes cards: a Look brick, Email this
 * helper (the email), or a brick
 * that makes one on its own (Write it or Draw a picture, one in total). Without that, a
 * brick that works on cards gets none (e.g. When I tap → Draw a picture).
 */
export function cardsBefore(bricks: StackBrick[], index: number): boolean {
	return bricks.slice(0, index).some((b) => {
		const def = brick(b.type);
		return (
			def?.group === 'look' ||
			b.type === 'when.email' || // the email is a card
			makesOne(b)
		);
	});
}

/** A brick that needs cards but nothing before it makes any. */
export function starved(bricks: StackBrick[], index: number): boolean {
	const b = bricks[index];
	const def = b && brick(b.type);
	if (!def || def.group === 'when' || def.group === 'look') return false;
	if (makesOne(b)) return false;
	return !cardsBefore(bricks, index);
}
