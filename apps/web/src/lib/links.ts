// Links from read content are untrusted: only http(s) may become a link.
export function safeURL(u: unknown): string | null {
	if (typeof u !== 'string') return null;
	try {
		const url = new URL(u.includes('://') ? u : `https://${u}`);
		return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : null;
	} catch {
		return null;
	}
}

export function hostOf(u: string): string {
	try {
		return new URL(u).hostname.replace(/^www\./, '');
	} catch {
		return '';
	}
}
