import { describe, expect, it } from 'vitest';
import { workspaceTimezone } from './timezone.ts';

describe('workspaceTimezone', () => {
	it('keeps a place, or UTC', () => {
		for (const tz of ['Europe/Vienna', 'America/Argentina/Buenos_Aires', 'UTC', 'Etc/GMT+3'])
			expect(workspaceTimezone(tz)).toBe(tz);
	});
	it('starts in Europe/Berlin when the browser names no place', () => {
		for (const tz of ['CET', 'EST5EDT', 'UTC+3', 'GMT', '', undefined])
			expect(workspaceTimezone(tz)).toBe('Europe/Berlin');
	});
	it('reads the browser by default', () => {
		expect(workspaceTimezone()).toMatch(/^(UTC|[A-Za-z_]+\/.+)$/);
	});
});
