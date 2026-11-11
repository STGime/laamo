import { describe, expect, it } from 'vitest';
import type { StackBrick } from '@laamo/catalog';
import { add, fillProfile, nextId, ready, startOnTop, tray } from './builder.ts';

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
		const look = tray('look');
		expect(ready.has(look[0].id)).toBe(true);
		expect(look.some((x) => !ready.has(x.id))).toBe(true);
	});
});
