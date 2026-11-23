import { llmsTxt } from '#lib/site/site.ts';

export const prerender = true;

// For AI assistants and agents: what Laamo is and where its pages are.
export const GET = () =>
	new Response(llmsTxt(), { headers: { 'Content-Type': 'text/markdown; charset=utf-8' } });
