import { describe, expect, it } from 'vitest';
import { de } from './de';
import { en } from './en';
import { browserLang, t } from './index.svelte';

describe('i18n', () => {
	it('has the same keys in en and de', () => {
		expect(Object.keys(de).sort()).toEqual(Object.keys(en).sort());
	});
	it('fills placeholders', () => {
		expect(t('swipe.left', { n: 3 }, 'de')).toBe('noch 3');
		expect(t('me.signedInAs', { email: 'a@b.de' }, 'en')).toBe('Signed in as a@b.de');
	});
	it('picks de or en from the browser', () => {
		expect(browserLang(['de-AT', 'en'])).toBe('de');
		expect(browserLang(['fr-FR', 'en-GB'])).toBe('en');
		expect(browserLang(['fr'])).toBe('en');
	});
	it('has no buzzwords in copy', () => {
		const all = [...Object.values(en), ...Object.values(de)].join(' ');
		expect(all).not.toMatch(/AI-powered|supercharge|seamless|effortless|✨|KI-gestützt/i);
	});
});
