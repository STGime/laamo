import { describe, expect, it } from 'vitest';
import { retrying } from './progress.ts';

describe('retrying', () => {
	it('knows when the current brick failed and waits for a retry', () => {
		const run = { id: 'r', status: 'running' as const, step: 1, steps: 3, test: true };
		const steps = [
			{ brick_id: 'b1', status: 'done' as const, cards_in: 0, cards_out: 0 },
			{ brick_id: 'b2', status: 'failed' as const, cards_in: 0, cards_out: 0 }
		];
		expect(retrying(run, steps, ['b1', 'b2', 'b3'])).toBe(true);
		expect(retrying({ ...run, step: 0 }, steps, ['b1', 'b2', 'b3'])).toBe(false);
		expect(retrying(undefined, steps, ['b1'])).toBe(false);
	});
});
