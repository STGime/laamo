import { createHash } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { guessCity, solve } from './demo.ts';

describe('solve', () => {
	it('finds a nonce the server accepts', async () => {
		const token = '1700000000.abc.sig';
		const nonce = await solve(token, 10);
		const sum = createHash('sha256').update(`${token}:${nonce}`).digest();
		// 10 leading zero bits: the first byte is 0, the second < 64.
		expect(sum[0]).toBe(0);
		expect(sum[1]).toBeLessThan(64);
	});
});

describe('guessCity', () => {
	it('takes European cities from the timezone', () => {
		expect(guessCity('Europe/Berlin')).toBe('Berlin');
		expect(guessCity('Europe/Isle_of_Man')).toBe('Isle of Man');
		expect(guessCity('America/New_York')).toBe('');
		expect(guessCity('UTC')).toBe('');
	});
});
