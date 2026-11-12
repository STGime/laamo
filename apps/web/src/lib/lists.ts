// Lists (#47): how rows and columns read on screen.
import { catalog } from '@laamo/catalog';

/** A column's name: the catalog's label, or the key in words ("opening_days" → "opening days"). */
export function columnLabel(key: string, lang: 'en' | 'de'): string {
	if (key === 'list_row_id') return '';
	return catalog.fields.find((f) => f.key === key)?.label[lang] ?? key.replaceAll('_', ' ');
}

/** What a row is called: its name, title or subject. */
export function rowHeading(data: Record<string, unknown>): string {
	for (const k of ['name', 'title', 'subject']) {
		const v = data[k];
		if (typeof v === 'string' && v.trim()) return v;
	}
	return '';
}

/** A cell as short text: web addresses show their host. */
export function cellText(v: unknown): string {
	if (v == null) return '';
	if (typeof v === 'string') {
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
	return null;
}

/** "Leads (Leipzig)" → "leads-leipzig.csv" */
export function csvName(name: string): string {
	const base = name
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '');
	return `${base || 'list'}.csv`;
}

/** An OpenStreetMap page this came from, or null. */
export function osmSource(url: unknown): string | null {
	return typeof url === 'string' &&
		/^https:\/\/www\.openstreetmap\.org\/(node|way|relation)\/\d+$/.test(url)
		? url
		: null;
}
