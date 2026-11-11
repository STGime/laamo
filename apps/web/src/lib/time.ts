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
