import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// Every text/background pairing the tokens define must meet WCAG AA (4.5:1).
const css = readFileSync(new URL('./tokens.css', import.meta.url), 'utf8');
const vars = Object.fromEntries(
	[...css.matchAll(/(--[\w-]+):\s*(#[0-9a-f]{6})/gi)].map((m) => [m[1], m[2]])
);

function luminance(hex: string): number {
	const [r, g, b] = [1, 3, 5].map((i) => {
		const c = parseInt(hex.slice(i, i + 2), 16) / 255;
		return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
	});
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
const contrast = (a: string, b: string) => {
	const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
	// Rounded to 2 decimals, as contrast checkers report it: the plan's
	// blue is exactly 4.50:1 with white (4.4997 unrounded).
	return Math.round(((l1 + 0.05) / (l2 + 0.05)) * 100) / 100;
};

describe('tokens', () => {
	it.each(['yellow', 'blue', 'purple', 'green', 'red'])('text on %s bricks meets AA', (c) => {
		expect(contrast(vars[`--brick-${c}`], vars[`--on-${c}`])).toBeGreaterThanOrEqual(4.5);
	});
	it.each([
		['--ink', '--white'],
		['--gray-600', '--white'],
		['--danger', '--white'],
		['--ink', '--gray-50'],
		['--white', '--ink']
	])('%s on %s meets AA', (fg, bg) => {
		expect(contrast(vars[fg], vars[bg])).toBeGreaterThanOrEqual(4.5);
	});
	it('white on the red brick would fail AA (why --on-red is ink)', () => {
		expect(contrast(vars['--brick-red'], '#ffffff')).toBeLessThan(4.5);
	});
});
