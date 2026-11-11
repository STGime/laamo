// The signed-in user: access token, /me, and the current workspace. The
// token lives in localStorage (wrapped: storage can be blocked); the app
// only ever holds the end user's own token, never a project key.
import { createApi, type Schemas } from './api/client';
import { browserLang, i18n } from './i18n/index.svelte';

const KEY = 'laamo.token';

function load(): string | null {
	try {
		return globalThis.localStorage?.getItem(KEY) ?? null;
	} catch {
		return null;
	}
}

function save(token: string | null) {
	try {
		if (token) globalThis.localStorage?.setItem(KEY, token);
		else globalThis.localStorage?.removeItem(KEY);
	} catch {
		/* private mode: the session lasts until the tab closes */
	}
}

type Status = 'loading' | 'signedOut' | 'signedIn' | 'offline';

let token = $state<string | null>(load());
let me = $state<Schemas['Me'] | null>(null);
let status = $state<Status>(token ? 'loading' : 'signedOut');
let workspaceId = $state<string | null>(null);

/** The API client, sending the current token. */
export const api = createApi({ token: () => token });

export const session = {
	get token() {
		return token;
	},
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

	/** Loads /me with the stored token. */
	async refresh() {
		if (!token) {
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

	/** Uses an access token (from sign-in, or pasted in dev). */
	async useToken(t: string): Promise<boolean> {
		token = t.trim();
		save(token);
		await this.refresh();
		return status === 'signedIn';
	},

	signOut() {
		token = null;
		me = null;
		save(null);
		status = 'signedOut';
	}
};
