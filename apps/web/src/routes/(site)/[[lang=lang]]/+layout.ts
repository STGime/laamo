// The public site: plain HTML at build time, so it reads without JavaScript
// and search engines see the content. English at /, German at /de.
import type { Lang } from '#lib/i18n/index.svelte.ts';
import { basePath } from '#lib/site/site.ts';
import type { LayoutLoad } from './$types';

export const prerender = true;
export const ssr = true;

export const load: LayoutLoad = ({ params, url }) => ({
	lang: (params.lang === 'de' ? 'de' : 'en') as Lang,
	path: basePath(url.pathname)
});
