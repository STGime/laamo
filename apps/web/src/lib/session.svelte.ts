// The signed-in user: tokens, /me, and the current workspace. Tokens live in
// localStorage (wrapped: storage can be blocked). The access token is
// refreshed shortly before it expires, so a session lasts as long as the
// refresh token (Eurobase: 7 days, renewed on use).
import { createApi, type Schemas } from './api/client';
import { createAuth, expiresIn, type Tokens } from './auth';
import { browserLang, i18n } from './i18n/index.svelte.ts';

const KEY = 'laamo.session';

interface Stored {
	access: string;
	refresh?: string;
}

function load(): Stored | null {
	try {
		const raw = globalThis.localStorage?.getItem(KEY);
		if (raw) return JSON.parse(raw) as Stored;
		// A dev token pasted before sessions had refresh tokens.
		const legacy = globalThis.localStorage?.getItem('laamo.token');
		return legacy ? { access: legacy } : null;
	} catch {
		return null;
	}
}

function save(s: Stored | null) {
	try {
		if (s) globalThis.localStorage?.setItem(KEY, JSON.stringify(s));
		else globalThis.localStorage?.removeItem(KEY);
		globalThis.localStorage?.removeItem('laamo.token');
	} catch {
		/* private mode: the session lasts until the tab closes */
	}
}

type Status = 'loading' | 'signedOut' | 'signedIn' | 'offline';

const stored = load();
let tokens = $state<Stored | null>(stored);
let me = $state<Schemas['Me'] | null>(null);
let status = $state<Status>(stored ? 'loading' : 'signedOut');
let workspaceId = $state<string | null>(null);

export const auth = createAuth();

let refreshing: Promise<void> | null = null;

/** Refreshes the access token if it expires within a minute (once at a time). */
async function fresh(): Promise<string | null> {
	const t = tokens;
	if (!t) return null;
	if (t.refresh && expiresIn(t.access) < 60) {
		refreshing ??= auth
			.refresh(t.refresh)
			.then((n) => setTokens(n))
			.catch(() => {
				/* the API answers 401 and we sign out */
			})
			.finally(() => (refreshing = null));
		await refreshing;
	}
	return tokens?.access ?? null;
}

function setTokens(n: Tokens) {
	tokens = { access: n.access_token, refresh: n.refresh_token || tokens?.refresh };
	save(tokens);
}

/** The API client, sending a fresh token. */
export const api = createApi({ token: () => tokens?.access, before: fresh });

export const session = {
	get token() {
		return tokens?.access ?? null;
	},
	/** A token that is valid for at least a minute (refreshing if needed). */
	fresh,
	get status() {
		return status;
	},
	get me() {
		return me;
	},
	get workspace(): Schemas['WorkspaceSummary'] | null {
		return me?.workspaces.find((w) => w.id === workspaceId) ?? me?.workspaces[0] ?? null;
	},
	set workspaceId(id: string) {
		workspaceId = id;
	},

	/** Loads /me with the stored tokens. */
	async refresh() {
		if (!tokens) {
			status = 'signedOut';
			return;
		}
		try {
			const { data, response } = await api.GET('/me');
			if (response.status === 401) return this.signOut();
			if (!data) {
				status = 'offline';
				return;
			}
			me = data;
			status = 'signedIn';
			i18n.lang = (this.workspace?.locale as 'en' | 'de' | undefined) ?? browserLang();
		} catch {
			status = 'offline';
		}
	},

	/** Starts a session from a magic link sign-in. */
	async useTokens(n: Tokens): Promise<boolean> {
		setTokens(n);
		await this.refresh();
		return status === 'signedIn';
	},

	/** Dev only: a pasted access token (no refresh). */
	async useToken(t: string): Promise<boolean> {
		tokens = { access: t.trim() };
		save(tokens);
		await this.refresh();
		return status === 'signedIn';
	},

	signOut() {
		tokens = null;
		me = null;
		save(null);
		status = 'signedOut';
	}
};
