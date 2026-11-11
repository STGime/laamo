import { describe, expect, it } from 'vitest';
import { hostOf, safeURL } from './links';

describe('safeURL', () => {
	it('allows http(s) only', () => {
		expect(safeURL('https://rbb24.de/a')).toBe('https://rbb24.de/a');
		expect(safeURL('krause.example')).toBe('https://krause.example/');
		expect(safeURL('javascript:alert(1)')).toBeNull();
		expect(safeURL('data:text/html,hi')).toBeNull();
		expect(safeURL(42)).toBeNull();
	});
	it('shows the host', () => {
		expect(hostOf('https://www.morgenpost.de/berlin/x')).toBe('morgenpost.de');
	});
});
