import { describe, expect, it } from 'vitest';
import { connectResult } from './social.ts';

const q = (s: string) => new URLSearchParams(s);

describe('connectResult', () => {
	it('reads a success', () => {
		expect(connectResult(q('success=true&platform=instagram&accountId=123'))).toEqual({
			ok: true,
			name: 'Instagram',
			connected: 0,
			skipped: 0,
			failed: 0
		});
	});
	it('counts Facebook Pages', () => {
		const r = connectResult(q('success=true&platform=facebook&connected=2&skipped=1&failed=0'));
		expect([r.name, r.connected, r.skipped, r.failed]).toEqual(['Facebook', 2, 1, 0]);
	});
	it('reads a failure, and anything but success=true is one', () => {
		expect(connectResult(q('success=false&platform=tiktok&error=access_denied')).ok).toBe(false);
		expect(connectResult(q('platform=tiktok')).ok).toBe(false);
		expect(connectResult(q('success=1')).ok).toBe(false);
	});
	it('shows nothing a stranger could put in the address', () => {
		const r = connectResult(
			q('success=true&platform=<b>Your bank</b>&connected=1e9&skipped=-3&failed=abc')
		);
		expect(r).toEqual({ ok: true, name: '', connected: 0, skipped: 0, failed: 0 });
	});
});
