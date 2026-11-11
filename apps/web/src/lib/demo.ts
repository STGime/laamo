// The demo before sign-up (#75): solve the server's small puzzle, guess the
// city, and remember the helper the visitor wants to keep until they have
// an account.

export type DemoTemplate = 'morning-briefing' | 'weekend-scout';

export type Pending = { template: DemoTemplate; business?: string; city: string };

const KEY = 'laamo.demo';

function leadingZeros(b: Uint8Array): number {
	let n = 0;
	for (const byte of b) {
		if (byte === 0) {
			n += 8;
			continue;
		}
		return n + Math.clz32(byte) - 24;
	}
	return n;
}

/** Finds a nonce so that sha256(token + ":" + nonce) starts with `bits` zero bits. */
export async function solve(token: string, bits: number): Promise<string> {
	const enc = new TextEncoder();
	const batch = 512;
	for (let start = 0; ; start += batch) {
		const sums = await Promise.all(
			Array.from({ length: batch }, (_, i) =>
				crypto.subtle.digest('SHA-256', enc.encode(`${token}:${start + i}`))
			)
		);
		const i = sums.findIndex((s) => leadingZeros(new Uint8Array(s)) >= bits);
		if (i >= 0) return String(start + i);
	}
}

/** A city from the browser's timezone ("Europe/Berlin" → "Berlin"), or "". */
export function guessCity(tz = Intl.DateTimeFormat().resolvedOptions().timeZone ?? ''): string {
	const [region, city] = tz.split('/');
	if (region !== 'Europe' || !city || tz.split('/').length !== 2) return '';
	return city.replaceAll('_', ' ');
}

export function savePending(p: Pending) {
	try {
		localStorage.setItem(KEY, JSON.stringify(p));
	} catch {
		/* private mode: the visitor sets it up again after sign-up */
	}
}

export function takePending(): Pending | null {
	try {
		const raw = localStorage.getItem(KEY);
		localStorage.removeItem(KEY);
		if (!raw) return null;
		const p = JSON.parse(raw) as Pending;
		return p.template && p.city ? p : null;
	} catch {
		return null;
	}
}

export function hasPending(): boolean {
	try {
		return localStorage.getItem(KEY) !== null;
	} catch {
		return false;
	}
}
