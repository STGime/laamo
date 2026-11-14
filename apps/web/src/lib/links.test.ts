import { describe, expect, it } from 'vitest';
import { hostOf, mailto, safeURL } from './links';

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

describe('mailto', () => {
	it('fills in a draft for the user’s own mail app', () => {
		expect(mailto('hallo@kuchenzeit.de', 'Kuchen & mehr', 'Guten Tag,\nwir backen.')).toBe(
			'mailto:hallo@kuchenzeit.de?subject=Kuchen%20%26%20mehr&body=Guten%20Tag%2C%0Awir%20backen.'
		);
		expect(mailto('a@b.de?cc=evil@x.de', 's', 'b')).toBeNull();
		expect(mailto('not an email', 's', 'b')).toBeNull();
	});
});
