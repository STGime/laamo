import { describe, expect, it } from 'vitest';
import { parseSSE } from './feed.svelte.ts';
import { ago } from './time.ts';

describe('parseSSE', () => {
	it('splits events and keeps the unfinished rest', () => {
		const { events, rest } = parseSSE(
			': connected\n\nevent: feed\ndata: {"id":"f1"}\n\nevent: reload\ndata: {}\n\nevent: run\nda'
		);
		expect(events).toEqual([
			{ event: 'feed', data: '{"id":"f1"}' },
			{ event: 'reload', data: '{}' }
		]);
		expect(rest).toBe('event: run\nda');
	});
	it('ignores heartbeats', () => {
		expect(parseSSE(': ping\n\n').events).toEqual([]);
	});
});

describe('ago', () => {
	const now = Date.parse('2026-11-11T12:00:00Z');
	it('reads like a feed app', () => {
		expect(ago('2026-11-11T11:59:30Z', now, 'en')).toBe('just now');
		expect(ago('2026-11-11T11:55:00Z', now, 'en')).toBe('5 min ago');
		expect(ago('2026-11-11T09:00:00Z', now, 'de')).toBe('vor 3 Std.');
		expect(ago('2026-11-09T12:00:00Z', now, 'en')).toBe('2 d ago');
	});
});
