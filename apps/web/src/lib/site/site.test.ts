import { describe, expect, it } from 'vitest';
import { basePath, langOf, localPath, pageURL, robots, sitemap } from './site';

describe('site', () => {
	it('puts German under /de', () => {
		expect(localPath('/', 'en')).toBe('/');
		expect(localPath('/', 'de')).toBe('/de');
		expect(localPath('/use-cases/x', 'de')).toBe('/de/use-cases/x');
		expect(pageURL('/', 'de')).toBe('https://laamo.app/de');
		expect(pageURL('/', 'en')).toBe('https://laamo.app/');
	});
	it('reads the language and page from a path', () => {
		expect(langOf('/de')).toBe('de');
		expect(langOf('/de/use-cases/x')).toBe('de');
		expect(langOf('/demo')).toBe('en');
		expect(basePath('/de')).toBe('/');
		expect(basePath('/de/use-cases/x')).toBe('/use-cases/x');
		expect(basePath('/demo')).toBe('/demo');
	});
	it('lists every page in both languages with alternates', () => {
		const xml = sitemap([{ path: '/' }, { path: '/use-cases/x' }]);
		expect(xml.match(/<loc>/g)).toHaveLength(4);
		expect(xml).toContain('<loc>https://laamo.app/de/use-cases/x</loc>');
		expect(xml).toContain('hreflang="x-default" href="https://laamo.app/use-cases/x"');
	});
	it('keeps staging out of search, and the app always', () => {
		expect(robots(false)).toBe('User-agent: *\nDisallow: /\n');
		const prod = robots(true);
		expect(prod).toContain('Disallow: /feed');
		expect(prod).toContain('Sitemap: https://laamo.app/sitemap.xml');
		expect(prod).not.toMatch(/Disallow: \/\n/);
	});
});
