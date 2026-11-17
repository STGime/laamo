import { sitemap } from '#lib/site/site.ts';

export const prerender = true;

export const GET = () =>
	new Response(sitemap(), { headers: { 'Content-Type': 'application/xml; charset=utf-8' } });
