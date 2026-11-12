import { describe, expect, it } from 'vitest';
import { cellHref, cellText, columnLabel, csvName, osmSource, rowHeading } from './lists.ts';

describe('lists', () => {
	it('names columns', () => {
		expect(columnLabel('email', 'de')).toBe('E-Mail');
		expect(columnLabel('opening_days', 'en')).toBe('opening days');
	});
	it('shows rows and cells', () => {
		expect(rowHeading({ email: 'a@b.de', name: 'Café' })).toBe('Café');
		expect(cellText('https://www.kuchenzeit.de/')).toBe('kuchenzeit.de');
		expect(cellText(4.5)).toBe('4.5');
		expect(cellHref('phone', '0341 / 123 45')).toBe('tel:034112345');
		expect(cellHref('email', 'a@b.de')).toBe('mailto:a@b.de');
		expect(cellHref('note', 'hello')).toBeNull();
	});
	it('names the CSV file', () => {
		expect(csvName('Leads (Leipzig)')).toBe('leads-leipzig.csv');
		expect(csvName('???')).toBe('list.csv');
	});
});

describe('osmSource', () => {
	it('knows OpenStreetMap pages', () => {
		expect(osmSource('https://www.openstreetmap.org/way/123')).toBe(
			'https://www.openstreetmap.org/way/123'
		);
		expect(osmSource('https://evil.example/way/1')).toBeNull();
		expect(osmSource(undefined)).toBeNull();
	});
});
