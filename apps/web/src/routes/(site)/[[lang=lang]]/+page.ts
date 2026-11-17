import type { EntryGenerator } from './$types';

// Both languages are built: / and /de.
export const entries: EntryGenerator = () => [{}, { lang: 'de' }];
