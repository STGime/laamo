// Times as the user likes them: "08:55" stays 24h unless the workspace
// picked 12h, then it's "8:55 AM". Stored values are always "HH:MM".
import { i18n } from '#lib/i18n/index.svelte.ts';

/** "14:05" → "2:05 PM" (12h) or "14:05" (24h). */
export function clock(hhmm: string, twelve = i18n.twelve): string {
	const m = /^([01]?\d|2[0-3]):([0-5]\d)$/.exec(hhmm);
	if (!m || !twelve) return hhmm;
	const h = Number(m[1]);
	return `${h % 12 || 12}:${m[2]} ${h < 12 ? 'AM' : 'PM'}`;
}

/** Every time in a sentence: "Every weekday at 07:30" → "… at 7:30 AM". */
export function clockText(text: string, twelve = i18n.twelve): string {
	if (!twelve) return text;
	return text.replace(/\b([01]?\d|2[0-3]):([0-5]\d)\b/g, (t) => clock(t, true));
}

/** Hours for a picker: 0–23, or 12, 1–11 for AM/PM. */
export function split(hhmm: string): { h: number; m: number } {
	const [h, m] = hhmm.split(':').map(Number);
	return { h: Number.isFinite(h) ? h : 8, m: Number.isFinite(m) ? m : 0 };
}

export const join = (h: number, m: number) =>
	`${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
