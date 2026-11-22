import type { EntryGenerator } from './$types';

// Both languages are built: /try and /de/try.
export const entries: EntryGenerator = () => [{}, { lang: 'de' }];
