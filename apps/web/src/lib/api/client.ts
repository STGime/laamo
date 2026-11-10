// Typed API client, generated from packages/openapi/openapi.yaml.
// Run `task gen` after changing the spec.
import createClient from 'openapi-fetch';
import type { components, paths } from './schema';

export type Schemas = components['schemas'];
export type ApiError = Schemas['Error'];

export interface ApiOptions {
	/** Absolute in tests (Node has no page URL to resolve /api against). */
	baseUrl?: string;
	fetch?: typeof fetch;
	/** Current Eurobase access token, or null when signed out. */
	token?: () => string | null | undefined;
}

export function createApi(opts: ApiOptions = {}) {
	const client = createClient<paths>({ baseUrl: opts.baseUrl ?? '/api', fetch: opts.fetch });
	const token = opts.token;
	if (token) {
		client.use({
			onRequest({ request }) {
				const t = token();
				if (t) request.headers.set('Authorization', `Bearer ${t}`);
				return request;
			}
		});
	}
	return client;
}

export type Api = ReturnType<typeof createApi>;

export const api = createApi();
