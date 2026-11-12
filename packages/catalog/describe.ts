// A brick as one plain sentence, like catalog.Sentence in Go
// (backend/internal/catalog/describe.go). Pills stay as written; place and
// list ids are shown as is (the UI swaps in their names).
import { brick, catalog, matches, withDefaults } from './index.ts';
import type { Lang, Setting, StackBrick, Text } from './index.ts';

const dayNames: Record<string, Text> = {
	mon: { en: 'Monday', de: 'Montag' },
	tue: { en: 'Tuesday', de: 'Dienstag' },
	wed: { en: 'Wednesday', de: 'Mittwoch' },
	thu: { en: 'Thursday', de: 'Donnerstag' },
	fri: { en: 'Friday', de: 'Freitag' },
	sat: { en: 'Saturday', de: 'Samstag' },
	sun: { en: 'Sunday', de: 'Sonntag' }
};

const strings = (v: unknown): string[] =>
	Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];

function join(items: string[], lang: Lang): string {
	if (items.length < 2) return items[0] ?? '';
	return items.slice(0, -1).join(', ') + (lang === 'de' ? ' und ' : ' and ') + items.at(-1);
}

/** A setting's value in words: option labels, day names, field labels. */
export function display(set: Setting, v: unknown, lang: Lang): string {
	switch (set.type) {
		case 'select': {
			const o = set.options?.find((o) => String(o.value) === String(v));
			if (o) return o.label[lang];
			break;
		}
		case 'days':
			return join(
				strings(v).flatMap((d) => (dayNames[d] ? [dayNames[d][lang]] : [])),
				lang
			);
		case 'fields':
			return join(
				strings(v).map((k) => catalog.fields.find((f) => f.key === k)?.label[lang] ?? k),
				lang
			);
		case 'tags':
			return join(strings(v), lang);
	}
	if (v === undefined || v === null) return '';
	return String(v);
}

export function sentence(sb: StackBrick, lang: Lang): string {
	const b = brick(sb.type);
	if (!b) return sb.type;
	const cfg = withDefaults(b, sb.config ?? {});
	let text = b.title[lang];
	for (const r of b.sentence) {
		if (matches(r.if, cfg)) {
			text = r.text[lang];
			break;
		}
	}
	for (const set of b.settings)
		text = text.replaceAll(`[${set.key}]`, display(set, cfg[set.key], lang));
	return text;
}
