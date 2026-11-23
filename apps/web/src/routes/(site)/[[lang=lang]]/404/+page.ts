import type { EntryGenerator } from './$types';

// Built as 404.html and de/404.html: the web server sends them (with status
// 404) for addresses that don't exist.
export const entries: EntryGenerator = () => [{}, { lang: 'de' }];
