import { error } from '@sveltejs/kit';
import { INDUSTRIES, industry } from '#lib/site/industries.ts';
import type { EntryGenerator, PageLoad } from './$types';

// Every kind of business, in both languages.
export const entries: EntryGenerator = () =>
	INDUSTRIES.flatMap((i) => [{ slug: i.slug }, { slug: i.slug, lang: 'de' }]);

export const load: PageLoad = ({ params }) => {
	const i = industry(params.slug);
	if (!i) error(404, 'Not found');
	return { industry: i };
};
