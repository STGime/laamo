// Connecting a social account: what the address says when the browser comes
// back from the network (Posta adds success, platform, and for Facebook how
// many Pages were connected, skipped or failed).

export interface ConnectResult {
	ok: boolean;
	/** The network's name, for the message; '' when it isn't one we know. */
	name: string;
	connected: number;
	skipped: number;
	failed: number;
}

const NAMES: Record<string, string> = {
	instagram: 'Instagram',
	facebook: 'Facebook',
	tiktok: 'TikTok',
	youtube: 'YouTube',
	linkedin: 'LinkedIn',
	threads: 'Threads',
	pinterest: 'Pinterest',
	bluesky: 'Bluesky',
	x: 'X',
	twitter: 'X'
};

// The address comes from outside: only a known network's name is shown,
// and only whole numbers are counted. The error text is never displayed.
export function connectResult(q: { get(key: string): string | null }): ConnectResult {
	const count = (key: string) => {
		const n = Number(q.get(key));
		return Number.isInteger(n) && n > 0 && n < 1000 ? n : 0;
	};
	return {
		ok: q.get('success') === 'true',
		name: NAMES[(q.get('platform') ?? '').toLowerCase()] ?? '',
		connected: count('connected'),
		skipped: count('skipped'),
		failed: count('failed')
	};
}
