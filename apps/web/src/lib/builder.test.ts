import { describe, expect, it } from 'vitest';
import type { StackBrick } from '@laamo/catalog';
import {
	add,
	applyFix,
	detailKey,
	fillProfile,
	nextId,
	ready,
	settingFix,
	starved,
	startOnTop,
	tray
} from './builder.ts';

const b = (id: string, type: string): StackBrick => ({ id, type, config: {} });

describe('builder', () => {
	it('keeps a When brick on top after a drag', () => {
		const out = startOnTop([b('b2', 'look.search'), b('b1', 'when.every'), b('b3', 'think.keep')]);
		expect(out.map((x) => x.id)).toEqual(['b1', 'b2', 'b3']);
	});

	it('adds at the end, and a When brick replaces the old one on top', () => {
		const st = [b('b1', 'when.every'), b('b2', 'look.search')];
		const keep = add(st, 'think.keep');
		expect(keep.bricks.map((x) => x.type)).toEqual(['when.every', 'look.search', 'think.keep']);
		expect(keep.bricks[2].config.max).toBe(10); // defaults filled in
		const tap = add(st, 'when.tap');
		expect(tap.bricks.map((x) => x.type)).toEqual(['when.tap', 'look.search']);
		expect(tap.id).not.toBe('b2');
	});

	it('starts Email this helper as only from me', () => {
		const st = [b('b1', 'when.every'), b('b2', 'look.search')];
		const mail = add(st, 'when.email', 'anna@example.com');
		expect(mail.bricks[0].config.senders).toEqual(['anna@example.com']);
		expect(add(st, 'when.email').bricks[0].config.senders).toBeUndefined();
		expect(add(st, 'when.tap', 'anna@example.com').bricks[0].config.senders).toBeUndefined();
	});

	it('picks a fresh id', () => {
		expect(nextId([b('b1', 'x'), b('b3', 'y')])).toBe('b4');
		expect(nextId([b('b3', 'x'), b('b4', 'y')])).toBe('b5');
	});

	it('fills known About me facts', () => {
		expect(fillProfile('Find {my business} in {my city}', { city: 'Leipzig', business: ' ' })).toBe(
			'Find {my business} in Leipzig'
		);
	});

	it('shows ready bricks first in the tray', () => {
		const doing = tray('do');
		expect(ready.has(doing[0].id)).toBe(true);
		expect(doing.some((x) => !ready.has(x.id))).toBe(true);
		expect(tray('look').every((x) => ready.has(x.id))).toBe(true);
	});

	it('puts a fix right before the brick that needs it', () => {
		const st = [b('b1', 'when.every'), b('b2', 'look.search'), b('b3', 'do.chat')];
		const r = applyFix(st, { insert: 'ask.check', before: 'b3' });
		expect(r.bricks.map((x) => x.type)).toEqual([
			'when.every',
			'look.search',
			'ask.check',
			'do.chat'
		]);
		expect(r.bricks[2].config.timeout_h).toBe(48);
		const start = applyFix([b('b1', 'look.search')], { insert: 'when.tap', before: 'b1' });
		expect(start.bricks.map((x) => x.type)).toEqual(['when.tap', 'look.search']);
	});
});

describe('detailKey', () => {
	it('turns words into a detail name', () => {
		expect(detailKey('Opening days')).toBe('opening_days');
		expect(detailKey('  Öffnungstage! ')).toBe('oeffnungstage');
		expect(detailKey('2nd phone')).toBe('x_2nd_phone');
		expect(detailKey('?!')).toBe('');
	});
});

describe('settingFix', () => {
	const search = { id: 'b2', type: 'look.search', config: { query: 'events' } };
	it('offers a duplicate rule the cards can meet', () => {
		const st = [
			{ id: 'b1', type: 'when.tap', config: {} },
			search,
			{ id: 'b3', type: 'do.list', config: { list: 'x', dedupe: 'website' } }
		];
		expect(settingFix(st, 'b3', 'website')).toMatchObject({ key: 'dedupe', value: 'url' });
	});
	it('picks a fitting rule when the brick is added', () => {
		const st = [{ id: 'b1', type: 'when.tap', config: {} }, search];
		const r = add(st, 'do.list');
		expect(r.bricks.find((x) => x.id === r.id)?.config.dedupe).toBe('url');
	});
});

describe('cards before', () => {
	const tap = { id: 'b1', type: 'when.tap', config: {} };
	it('draws one picture in total right after When', () => {
		const r = add([tap], 'think.draw');
		expect(r.bricks[1].config.mode).toBe('all');
		expect(starved(r.bricks, 1)).toBe(false);
	});
	it('writes one post in total right after When', () => {
		const r = add([tap], 'think.write');
		expect(r.bricks[1].config.mode).toBe('all');
		expect(starved(r.bricks, 1)).toBe(false);
		// The post is a card for what comes next.
		const post = add(r.bricks, 'do.social');
		expect(starved(post.bricks, 2)).toBe(false);
		const each = [tap, { id: 'b2', type: 'think.write', config: { what: 'x', mode: 'each' } }];
		expect(starved(each, 1)).toBe(true);
	});
	it('notices a brick that gets no cards', () => {
		const st = [tap, { id: 'b2', type: 'think.draw', config: { mode: 'each' } }];
		expect(starved(st, 1)).toBe(true);
		const withSearch = [tap, { id: 'b3', type: 'look.search', config: { query: 'x' } }, st[1]];
		expect(starved(withSearch, 2)).toBe(false);
		const mail = [{ id: 'b1', type: 'when.email', config: {} }, st[1]];
		expect(starved(mail, 1)).toBe(false);
	});
});
