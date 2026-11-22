// The public site (routes/(site)): prerendered pages for people and search
// engines. English at /, German under /de, one language per URL.
import type { Lang } from '#lib/i18n/index.svelte.ts';
import { copy } from './copy.ts';
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
	/** When its words last changed (YYYY-MM-DD), if later than SITE_UPDATED. */
	updated?: string;
}

/**
 * When the site's pages last changed as a whole (YYYY-MM-DD): the sitemap's
 * lastmod. Set it by hand when the words change, not on every build: a date
 * that moves without a change teaches search engines to ignore it.
 */
export const SITE_UPDATED = '2026-11-22';

/** Every page of the site. New site pages are added here (sitemap, alternates). */
export const PAGES: SitePage[] = [
	{ path: '/', changefreq: 'weekly', priority: 1 },
	{ path: '/try', changefreq: 'monthly', priority: 0.8 },
	{ path: '/use-cases', changefreq: 'monthly', priority: 0.8 },
	...USE_CASES.map((u) => ({
		path: `/use-cases/${u.slug}`,
		changefreq: 'monthly' as const,
		priority: 0.7
	})),
	{ path: '/legal-notice', changefreq: 'yearly', priority: 0.2 },
	{ path: '/privacy', changefreq: 'yearly', priority: 0.2 },
	{ path: '/terms', changefreq: 'yearly', priority: 0.2 }
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
				`    <lastmod>${pg.updated ?? SITE_UPDATED}</lastmod>`,
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
	return [
		'User-agent: *',
		'Allow: /',
		...APP_ROUTES.map((p) => `Disallow: /${p}`),
		'',
		`Sitemap: ${SITE_URL}/sitemap.xml`,
		''
	].join('\n');
}

/**
 * The app's screens, by their first path part (routes/(app)). They're drawn
 * in the browser from app.html and aren't for search engines. The web
 * server sends app.html for these and nothing else, so an address that
 * doesn't exist is a real 404 (deploy/docker/web.nginx.conf.template has
 * the same list; site.test.ts checks both against the route folders).
 */
export const APP_ROUTES = [
	'auth',
	'feed',
	'helpers',
	'kit',
	'lists',
	'me',
	'new',
	'places',
	'shared',
	'signin',
	'start',
	'welcome'
];

/** A value as JSON for a <script type="application/ld+json"> tag. */
export function jsonLD(value: unknown): string {
	return JSON.stringify(value).replace(/</g, '\\u003c');
}

/** Who makes Laamo, as schema.org data (shared by the pages' structured data). */
export function organizationLD() {
	return {
		'@type': 'Organization',
		'@id': `${SITE_URL}/#organization`,
		name: 'Laamo',
		url: SITE_URL,
		logo: `${SITE_URL}/logo.png`,
		parentOrganization: { '@type': 'Organization', name: 'Eurobase OÜ' }
	};
}

/**
 * llms.txt (llmstxt.org): what Laamo is and where its pages are, as plain
 * Markdown for AI assistants and agents that look a site up. Built from the
 * same words as the pages, so it can't drift from them.
 */
export function llmsTxt(): string {
	const c = copy.en;
	const line = (path: string, name: string, about: string) =>
		`- [${name}](${pageURL(path, 'en')}): ${about}`;
	return [
		'# Laamo',
		'',
		`> Laamo: ${c.category}. ${c.meta.description}`,
		'',
		'Laamo is a web app for phone and computer. A helper is a short stack of bricks (when it runs, where it looks, what it thinks about, what it does, when it asks you first) that does one recurring task. People pick a ready-made helper and answer up to three questions, or build their own from bricks. Helpers never send anything to other people without the owner checking it first.',
		'',
		'Status: invite-only. Anyone can try two helpers on the demo page without an account, and join the waiting list on the home page. Laamo is made by Eurobase OÜ, Estonia; data is stored in the EU. The site is in English and German (German pages are under /de).',
		'',
		'## Start here',
		'',
		line('/', 'Home', 'what Laamo is, how it works, questions and answers, and the waiting list'),
		line('/try', 'Try a helper without an account', c.tryPage.description),
		line('/use-cases', 'All helpers', UI_LEAD),
		'',
		'## What a helper can do',
		'',
		...USE_CASES.map((u) =>
			line(
				`/use-cases/${u.slug}`,
				u.text.en.name + (u.soon ? ' (coming soon)' : ''),
				u.text.en.description
			)
		),
		'',
		'## Questions and answers',
		'',
		...c.faq.items.map((i) => `- ${i.q} ${i.a}`),
		'',
		'## Legal',
		'',
		line('/privacy', 'Privacy policy', 'what data Laamo keeps and why'),
		line('/terms', 'Terms', 'the rules for using Laamo'),
		line('/legal-notice', 'Legal notice', 'who runs Laamo and how to reach them'),
		''
	].join('\n');
}

const UI_LEAD =
	'each helper is a short stack of bricks you can change; one page per helper with an example result';
