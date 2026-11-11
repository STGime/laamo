import { describe, expect, it } from 'vitest';
import { clock, clockText, join, split } from './clock.ts';

describe('clock', () => {
	it('keeps 24h by default and does AM/PM when asked', () => {
		expect(clock('08:55', false)).toBe('08:55');
		expect(clock('08:55', true)).toBe('8:55 AM');
		expect(clock('00:10', true)).toBe('12:10 AM');
		expect(clock('12:00', true)).toBe('12:00 PM');
		expect(clock('23:45', true)).toBe('11:45 PM');
	});
	it('rewrites times in sentences', () => {
		expect(clockText('Every Friday at 08:50', true)).toBe('Every Friday at 8:50 AM');
		expect(clockText('Jeden Freitag um 18:05', false)).toBe('Jeden Freitag um 18:05');
	});
	it('splits and joins', () => {
		expect(split('07:30')).toEqual({ h: 7, m: 30 });
		expect(join(7, 5)).toBe('07:05');
	});
});
