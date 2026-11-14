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

/**
 * "Open in my email": a mailto link with the draft filled in. Outreach goes
 * out from the user's own mailbox, never from ours. Null without a valid
 * address.
 */
export function mailto(email: string, subject: string, body: string): string | null {
	const to = email.trim();
	if (!/^[^@\s<>"?&]+@[^@\s<>"?&]+\.[a-z]{2,}$/i.test(to)) return null;
	const q = new URLSearchParams();
	if (subject.trim()) q.set('subject', subject.trim());
	if (body.trim()) q.set('body', body.trim());
	// mailto wants %20 for spaces, not "+".
	return `mailto:${to}?${q.toString().replace(/\+/g, '%20')}`;
}
