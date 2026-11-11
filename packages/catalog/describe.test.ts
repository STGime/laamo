import { describe, expect, it } from 'vitest';
import { sentence } from './describe.ts';

// The same cases as TestSentence in Go.
describe('sentence', () => {
	const cases: [string, Record<string, unknown>, 'en' | 'de', string][] = [
		[
			'when.every',
			{ freq: 'weekly', days: ['mon', 'fri'], time: '08:00' },
			'en',
			'Every Monday and Friday at 08:00'
		],
		[
			'when.every',
			{ freq: 'weekly', days: ['mon', 'wed', 'fri'], time: '08:00' },
			'de',
			'Jeden Montag, Mittwoch und Freitag um 08:00'
		],
		['when.every', { time: '07:30' }, 'en', 'Every weekday at 07:30'],
		['look.find_businesses', { what: 'bakeries' }, 'en', 'Find bakeries in {my city}'],
		['think.extract', { fields: ['email', 'phone'] }, 'de', 'E-Mail und Telefon herausziehen'],
		[
			'think.keep',
			{ criteria: 'have no shop', max: 5 },
			'en',
			'Keep ones that have no shop, max 5'
		],
		['do.email', { to: 'fixed', address: 'a@b.de' }, 'en', 'Prepare an email to a@b.de'],
		['look.read_page', { depth: 'contact' }, 'en', 'Read their website and contact page'],
		['nope.brick', {}, 'en', 'nope.brick']
	];
	for (const [type, config, lang, want] of cases)
		it(`${type} (${lang})`, () => expect(sentence({ id: 'b1', type, config }, lang)).toBe(want));
});
