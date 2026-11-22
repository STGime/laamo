// Lists (#47): how rows and columns read on screen.
import { catalog } from '@laamo/catalog';
import { readableDate } from './time.ts';

/** A column's name: the catalog's label, or the key in words ("opening_days" → "opening days"). */
export function columnLabel(key: string, lang: 'en' | 'de'): string {
	if (key === 'list_row_id') return '';
	return catalog.fields.find((f) => f.key === key)?.label[lang] ?? key.replaceAll('_', ' ');
}

/** What a row is called: its name, title, subject or (a receipt) vendor. */
export function rowHeading(data: Record<string, unknown>): string {
	for (const k of ['name', 'title', 'subject', 'vendor']) {
		const v = data[k];
		if (typeof v === 'string' && v.trim()) return v;
	}
	return '';
}

/** A cell as short text: web addresses show their host. */
export function cellText(v: unknown): string {
	if (v == null) return '';
	if (typeof v === 'string') {
		// Dates and times ("published_at") for people, not machines.
		const date = readableDate(v);
		if (date) return date;
		if (/^https?:\/\//.test(v)) {
			try {
				const u = new URL(v);
				return u.host.replace(/^www\./, '') + (u.pathname.length > 1 ? u.pathname : '');
			} catch {
				return v;
			}
		}
		return v;
	}
	return String(v);
}

/** A link for a cell, if it's one (web address, email, phone). */
export function cellHref(key: string, v: unknown): string | null {
	if (typeof v !== 'string' || !v) return null;
	if (/^https?:\/\//.test(v)) return v;
	if (key === 'email' && v.includes('@')) return `mailto:${v}`;
	if (key === 'phone') return `tel:${v.replace(/[^\d+]/g, '')}`;
	const at = key === 'location' ? /^(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)$/.exec(v) : null;
	if (at)
		return `https://www.openstreetmap.org/?mlat=${at[1]}&mlon=${at[2]}#map=18/${at[1]}/${at[2]}`;
	return null;
}

/** "Leads (Leipzig)" → "leads-leipzig.csv" (or ".zip", with the pictures) */
export function csvName(name: string, ext: 'csv' | 'zip' = 'csv'): string {
	const base = name
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '');
	return `${base || 'list'}.${ext}`;
}

/**
 * A row's picture (a receipt photo, a drawn picture): its storage path, or
 * null. The app shows it instead of the path; `image_alt` is its description.
 */
export function rowPicture(data: Record<string, unknown>): string | null {
	const v = data.image;
	return typeof v === 'string' && v.startsWith('images/') ? v : null;
}

/**
 * The other pictures of the same thing (`more_images`): the further parts
 * of a long receipt taken in several photos.
 */
export function morePictures(data: Record<string, unknown>): string[] {
	const v = data.more_images;
	if (typeof v !== 'string') return [];
	return v
		.split(',')
		.map((p) => p.trim())
		.filter((p) => p.startsWith('images/'));
}

/** An OpenStreetMap page this came from, or null. */
export function osmSource(url: unknown): string | null {
	return typeof url === 'string' &&
		/^https:\/\/www\.openstreetmap\.org\/(node|way|relation)\/\d+$/.test(url)
		? url
		: null;
}
