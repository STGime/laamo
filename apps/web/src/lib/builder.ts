// The brick builder (#39): what the tray offers, where a new brick goes,
// and the rules that keep a stack sensible while it's dragged around.
import {
	brick,
	catalog,
	withDefaults,
	type Brick,
	type GroupId,
	type StackBrick
} from '@laamo/catalog';

/** Bricks the worker can run today. The rest arrive with M4 and show as "Soon". */
export const ready = new Set([
	'when.every',
	'when.tap',
	'look.search',
	'look.read_page',
	'think.summarize',
	'think.keep',
	'do.chat',
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
 * (a helper starts one way), anything else goes at the end.
 */
export function add(bricks: StackBrick[], type: string): { bricks: StackBrick[]; id: string } {
	const b = brick(type);
	if (!b) return { bricks, id: '' };
	const sb: StackBrick = { id: nextId(bricks), type, config: withDefaults(b, {}) };
	if (b.shape === 'start') return { bricks: [sb, ...bricks.filter((x) => !isStart(x))], id: sb.id };
	return { bricks: [...bricks, sb], id: sb.id };
}

/** Card fields worth a pill (the rest are internal, like page_text). */
export const fieldPills = catalog.fields.filter((f) => f.pick).map((f) => f.key);

/** {my city} → Leipzig where the fact is known (as on the Shelf). */
export function fillProfile(text: string, profile: Record<string, unknown>): string {
	return text.replace(/\{my ([a-z_]+)\}/g, (m, key: string) => {
		const v = profile[key];
		return typeof v === 'string' && v.trim() ? v : m;
	});
}
