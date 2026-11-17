// The public site (routes/(site)): prerendered pages for people and search
// engines. English at /, German under /de, one language per URL.
import type { Lang } from '#lib/i18n/index.svelte.ts';
import { USE_CASES } from './usecases.ts';

/** Where the site lives; canonical links and the sitemap use it. */
export const SITE_URL = __SITE_URL__.replace(/\/+$/, '');

/** Only the production build may be indexed (staging must stay out of search). */
export const INDEXABLE = __SITE_INDEX__;

export const LANGS: Lang[] = ['en', 'de'];

/** A page of the site, by its English path ("/", "/use-cases/customer-finder"). */
export interface SitePage {
	path: string;
	/** For the sitemap: how often it changes. */
	changefreq?: 'daily' | 'weekly' | 'monthly' | 'yearly';
	priority?: number;
}

/** Every page of the site. New site pages are added here (sitemap, alternates). */
export const PAGES: SitePage[] = [
	{ path: '/', changefreq: 'weekly', priority: 1 },
	{ path: '/use-cases', changefreq: 'monthly', priority: 0.8 },
	...USE_CASES.map((u) => ({
		path: `/use-cases/${u.slug}`,
		changefreq: 'monthly' as const,
		priority: 0.7
	}))
];

/** The path of a page in a language: ("/", "de") → "/de". */
export function localPath(path: string, lang: Lang): string {
	if (lang === 'en') return path;
	return path === '/' ? `/${lang}` : `/${lang}${path}`;
}

/** The full URL of a page in a language. */
export function pageURL(path: string, lang: Lang): string {
	const p = localPath(path, lang);
	return SITE_URL + (p === '/' ? '/' : p);
}

/** The language of a URL path: "/de/…" → de, anything else → en. */
export function langOf(pathname: string): Lang {
	return /^\/de(\/|$)/.test(pathname) ? 'de' : 'en';
}

/** The English path behind a localized one: "/de/use-cases/x" → "/use-cases/x". */
export function basePath(pathname: string): string {
	const p = pathname.replace(/^\/de(?=\/|$)/, '');
	return p === '' ? '/' : p;
}

const esc = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

/** sitemap.xml with every page in both languages, each listing its alternates. */
export function sitemap(pages: SitePage[] = PAGES): string {
	const urls = pages.flatMap((pg) =>
		LANGS.map((lang) => {
			const alts = [
				...LANGS.map(
					(l) =>
						`    <xhtml:link rel="alternate" hreflang="${l}" href="${esc(pageURL(pg.path, l))}"/>`
				),
				`    <xhtml:link rel="alternate" hreflang="x-default" href="${esc(pageURL(pg.path, 'en'))}"/>`
			];
			return [
				'  <url>',
				`    <loc>${esc(pageURL(pg.path, lang))}</loc>`,
				...alts,
				pg.changefreq ? `    <changefreq>${pg.changefreq}</changefreq>` : '',
				pg.priority !== undefined ? `    <priority>${pg.priority.toFixed(1)}</priority>` : '',
				'  </url>'
			]
				.filter(Boolean)
				.join('\n');
		})
	);
	return [
		'<?xml version="1.0" encoding="UTF-8"?>',
		'<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">',
		...urls,
		'</urlset>',
		''
	].join('\n');
}

/** robots.txt: the site is open, the app isn't; staging is closed entirely. */
export function robots(indexable: boolean = INDEXABLE): string {
	if (!indexable) return 'User-agent: *\nDisallow: /\n';
	const app = [
		'/feed',
		'/helpers',
		'/lists',
		'/me',
		'/new',
		'/places',
		'/welcome',
		'/auth',
		'/kit'
	];
	return [
		'User-agent: *',
		'Allow: /',
		...app.map((p) => `Disallow: ${p}`),
		'',
		`Sitemap: ${SITE_URL}/sitemap.xml`,
		''
	].join('\n');
}
