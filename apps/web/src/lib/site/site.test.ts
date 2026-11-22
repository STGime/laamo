import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import {
	APP_ROUTES,
	basePath,
	jsonLD,
	langOf,
	llmsTxt,
	localPath,
	PAGES,
	pageURL,
	robots,
	sitemap,
	SITE_UPDATED
} from './site';
import { USE_CASES } from './usecases';

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
	it('dates every page in the sitemap', () => {
		const xml = sitemap([{ path: '/' }, { path: '/x', updated: '2027-01-02' }]);
		expect(xml).toContain(`<lastmod>${SITE_UPDATED}</lastmod>`);
		expect(xml).toContain('<lastmod>2027-01-02</lastmod>');
		expect(SITE_UPDATED).toMatch(/^\d{4}-\d{2}-\d{2}$/);
		expect(PAGES.map((p) => p.path)).toContain('/try');
	});
	// An address that isn't a page must be a real 404, so the web server
	// sends app.html only for the app's own screens. Three lists have to
	// agree: the route folders, APP_ROUTES (robots.txt) and nginx.
	it('knows the app screens, like the web server does', () => {
		const folders = readdirSync(new URL('../../routes/(app)', import.meta.url), {
			withFileTypes: true
		})
			.filter((d) => d.isDirectory())
			.map((d) => d.name)
			.sort();
		expect(APP_ROUTES).toEqual(folders);
		const nginx = readFileSync(
			new URL('../../../../../deploy/docker/web.nginx.conf.template', import.meta.url),
			'utf8'
		);
		expect(nginx).toContain(`location ~ ^/(${APP_ROUTES.join('|')})(/|$) {`);
		for (const r of APP_ROUTES) expect(robots(true)).toContain(`Disallow: /${r}\n`);
	});
	it('tells AI assistants what Laamo is and where its pages are', () => {
		const txt = llmsTxt();
		expect(txt.startsWith('# Laamo\n\n> Laamo: AI helpers for small businesses.')).toBe(true);
		for (const u of USE_CASES) expect(txt).toContain(`(https://laamo.app/use-cases/${u.slug})`);
		expect(txt).toContain('(https://laamo.app/try)');
		expect(txt).toContain('invite-only');
	});
	it('keeps structured data from closing its script tag', () => {
		expect(jsonLD({ a: '</script><b>' })).not.toContain('<');
	});
});
