// Runs while the site is prerendered: <html lang> matches the page's language.
import type { Handle } from '@sveltejs/kit/hooks';
import { langOf } from '#lib/site/site.ts';

export const handle: Handle = ({ event, resolve }) =>
	resolve(event, {
		transformPageChunk: ({ html }) => html.replace('%lang%', langOf(event.url.pathname))
	});
