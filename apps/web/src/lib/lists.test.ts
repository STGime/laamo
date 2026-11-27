import { describe, expect, it } from 'vitest';
import {
	cellHref,
	cellText,
	columnLabel,
	csvName,
	osmSource,
	morePictures,
	rowHeading,
	rowPicture
} from './lists.ts';

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
		expect(cellHref('location', '51.34,12.37')).toBe(
			'https://www.openstreetmap.org/?mlat=51.34&mlon=12.37#map=18/51.34/12.37'
		);
		expect(cellHref('location', '51.34;evil')).toBeNull();
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

describe('dates', () => {
	it('shows dates and times for people', () => {
		expect(cellText('2026-11-13')).toMatch(/13 Nov 2026/);
		expect(cellText('2026-11-13T13:18:35Z')).toMatch(/13 Nov 2026, \d{2}:18/);
		expect(cellText('2026-11-13 is a date in text')).toBe('2026-11-13 is a date in text');
	});
	it('knows a receipt row: its vendor, its picture, its export name', () => {
		expect(rowHeading({ vendor: 'Café Sonne', total: '11.50' })).toBe('Café Sonne');
		expect(rowPicture({ image: 'images/ws/run/p1.jpg' })).toBe('images/ws/run/p1.jpg');
		expect(rowPicture({ image: 'https://example.com/x.jpg' })).toBeNull();
		expect(rowPicture({ vendor: 'x' })).toBeNull();
		expect(csvName('Belege 2026', 'zip')).toBe('belege-2026.zip');
		// A long receipt taken in several photos keeps its other parts.
		expect(morePictures({ more_images: 'images/ws/in-2/p1.jpg, images/ws/in-3/p1.jpg' })).toEqual([
			'images/ws/in-2/p1.jpg',
			'images/ws/in-3/p1.jpg'
		]);
		expect(morePictures({ more_images: '' })).toEqual([]);
		expect(morePictures({})).toEqual([]);
	});
});

describe('what a row shows first', () => {
	it('marks a result by how it starts', async () => {
		const { resultMark, longCell } = await import('./lists.ts');
		expect(resultMark('All good: all 12 emails passed.')).toBe('good');
		expect(resultMark('Minor problem: 1 of 2 emails failed, …')).toBe('minor');
		expect(resultMark('Problem: 3 of 20 emails did not come from …')).toBe('bad');
		expect(resultMark('Someone else is sending')).toBe(null);
		expect(resultMark(undefined)).toBe(null);
		// An email's text is folded away, a short value isn't.
		expect(longCell('text', 'Hi')).toBe(true);
		expect(longCell('subject', 'Report domain: news.example.com')).toBe(false);
		expect(longCell('failed_sources', 'x'.repeat(300))).toBe(true);
	});
});
