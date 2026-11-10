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
}

export function createApi(opts: ApiOptions = {}) {
	return createClient<paths>({ baseUrl: opts.baseUrl ?? '/api', fetch: opts.fetch });
}

export type Api = ReturnType<typeof createApi>;

export const api = createApi();
