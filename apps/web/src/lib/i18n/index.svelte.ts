// Tiny i18n: en + de from day one. The language comes from the workspace
// once signed in, before that from the browser.
import { de } from './de';
import { en, type Key } from './en';

export type Lang = 'en' | 'de';
export type { Key };

const dicts: Record<Lang, Record<Key, string>> = { en, de };

export function browserLang(
	langs: readonly string[] = globalThis.navigator?.languages ?? []
): Lang {
	for (const l of langs) {
		const base = l.toLowerCase().split('-')[0];
		if (base === 'de' || base === 'en') return base;
	}
	return 'en';
}

let current = $state<Lang>(browserLang());
// Times: 24h unless the workspace picked 12h (AM/PM).
let twelve = $state(false);

export const i18n = {
	get lang(): Lang {
		return current;
	},
	get twelve(): boolean {
		return twelve;
	},
	set twelve(v: boolean) {
		twelve = v;
	},
	set lang(l: Lang) {
		current = l;
		if (typeof document !== 'undefined') document.documentElement.lang = l;
	}
};

/** Translate key, filling {name} placeholders from params; n = 1 picks the "key.one" form. */
export function t(
	key: Key,
	params: Record<string, string | number> = {},
	lang: Lang = current
): string {
	// Plurals: "x.one" is used when n is 1 (→ 1 card).
	const one = `${key}.one` as Key;
	if (params.n === 1 && one in en) key = one;
	const text = dicts[lang][key] ?? en[key] ?? key;
	return text.replace(/\{(\w+)\}/g, (m, name: string) =>
		name in params ? String(params[name]) : m
	);
}
