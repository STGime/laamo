// Passwordless sign-in with Eurobase auth, straight from the browser (per-IP
// limits then apply per user, not to our server).
//
// One flow for new and returning users, so the screen never reveals whether
// an email has an account: sign up with a random password nobody ever sees
// (an existing email just fails), then request a magic link. Clicking the
// link proves the email and signs in.

export interface Tokens {
	access_token: string;
	refresh_token: string;
	expires_in?: number;
}

export interface AuthOptions {
	url?: string;
	key?: string;
	fetch?: typeof fetch;
}

export class AuthError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
	}
}

function randomPassword(): string {
	const bytes = new Uint8Array(32);
	crypto.getRandomValues(bytes);
	return btoa(String.fromCharCode(...bytes));
}

export function createAuth(opts: AuthOptions = {}) {
	const url = (opts.url ?? __EUROBASE_URL__).replace(/\/$/, '');
	const key = opts.key ?? __EUROBASE_PUBLIC_KEY__;
	const f = opts.fetch ?? fetch;

	async function post<T>(path: string, body: unknown): Promise<T> {
		const res = await f(`${url}/v1/auth/${path}`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', apikey: key },
			body: JSON.stringify(body)
		});
		const data = await res.json().catch(() => ({}));
		if (!res.ok)
			throw new AuthError(res.status, (data as { error?: string }).error ?? `HTTP ${res.status}`);
		return data as T;
	}

	return {
		/** Sends a sign-in link to email; the link opens redirectTo?token=… */
		async sendLink(email: string, redirectTo: string): Promise<void> {
			email = email.trim().toLowerCase();
			try {
				await post('signup', { email, password: randomPassword() });
			} catch (e) {
				// Already registered (or sign-up closed): a link still signs them in.
				if (!(e instanceof AuthError) || e.status >= 500 || e.status === 429) throw e;
			}
			await post('request-magic-link', { email, email_redirect_to: redirectTo });
		},
		/** Exchanges the link's token for a session. */
		signIn(token: string): Promise<Tokens> {
			return post<Tokens>('signin-magic-link', { token });
		},
		refresh(refreshToken: string): Promise<Tokens> {
			return post<Tokens>('refresh', { refresh_token: refreshToken });
		}
	};
}

export type Auth = ReturnType<typeof createAuth>;

/** The callback URL magic links lead to (must be in Eurobase's redirect_urls). */
export function callbackURL(origin = globalThis.location?.origin ?? ''): string {
	return `${origin}/auth/callback`;
}

/** Seconds until a JWT expires (0 if unreadable or expired). */
export function expiresIn(jwt: string, now = Date.now()): number {
	try {
		const payload = JSON.parse(atob(jwt.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
		return Math.max(0, Math.floor(payload.exp - now / 1000));
	} catch {
		return 0;
	}
}
