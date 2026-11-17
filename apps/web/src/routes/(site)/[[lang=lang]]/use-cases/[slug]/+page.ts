import { error } from '@sveltejs/kit';
import { USE_CASES, useCase } from '#lib/site/usecases.ts';
import type { EntryGenerator, PageLoad } from './$types';

// Every use case, in both languages.
export const entries: EntryGenerator = () =>
	USE_CASES.flatMap((u) => [{ slug: u.slug }, { slug: u.slug, lang: 'de' }]);

export const load: PageLoad = ({ params }) => {
	const u = useCase(params.slug);
	if (!u) error(404, 'Not found');
	return { useCase: u };
};
