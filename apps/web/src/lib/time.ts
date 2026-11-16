import { i18n, t, type Lang } from './i18n/index.svelte.ts';

/** "5 min ago" style, for Feed items. */
export function ago(iso: string, now = Date.now(), lang?: Lang): string {
	const s = Math.max(0, (now - new Date(iso).getTime()) / 1000);
	if (s < 60) return t('time.now', {}, lang);
	if (s < 3600) return t('time.min', { n: Math.floor(s / 60) }, lang);
	if (s < 86400) return t('time.hour', { n: Math.floor(s / 3600) }, lang);
	return t('time.day', { n: Math.floor(s / 86400) }, lang);
}

/** Short weekday + time in the user's language, e.g. "Thu 14:00" (or "Thu 2:00 pm"). */
export function when(iso: string, lang: Lang, twelve = i18n.twelve): string {
	return new Date(iso).toLocaleString(lang === 'de' ? 'de-DE' : 'en-GB', {
		weekday: 'short',
		hour: twelve ? 'numeric' : '2-digit',
		minute: '2-digit',
		hour12: twelve
	});
}

/**
 * A date or date and time for people, in their language and clock:
 * "2026-11-13T13:18:35Z" → "Fri 13 Nov 2026, 14:18" (or "2:18 pm");
 * "2026-11-13" → "Fri 13 Nov 2026". Not a date: null.
 */
export function readableDate(
	v: string,
	lang: Lang = i18n.lang,
	twelve = i18n.twelve
): string | null {
	const m = /^(\d{4})-(\d{2})-(\d{2})(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$/.exec(
		v.trim()
	);
	if (!m) return null;
	const d = m[4] ? new Date(v) : new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
	if (Number.isNaN(d.getTime())) return null;
	const locale = lang === 'de' ? 'de-DE' : 'en-GB';
	const day: Intl.DateTimeFormatOptions = {
		weekday: 'short',
		day: 'numeric',
		month: 'short',
		year: 'numeric'
	};
	if (!m[4]) return d.toLocaleDateString(locale, day);
	return d.toLocaleString(locale, {
		...day,
		hour: twelve ? 'numeric' : '2-digit',
		minute: '2-digit',
		hour12: twelve
	});
}
