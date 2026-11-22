import { describe, expect, it } from 'vitest';
import { fit } from './photo.ts';

describe('fit', () => {
	it('keeps a small picture its size', () => {
		expect(fit(800, 600)).toEqual({ w: 800, h: 600 });
	});
	it('shrinks to the long side, keeping the shape', () => {
		expect(fit(4000, 3000)).toEqual({ w: 1800, h: 1350 });
		expect(fit(3000, 4000)).toEqual({ w: 1350, h: 1800 });
	});
	it('never makes a side zero', () => {
		expect(fit(9000, 2, 100)).toEqual({ w: 100, h: 1 });
	});
});
