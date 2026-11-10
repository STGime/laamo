import { describe, expect, it } from 'vitest';
import { createApi } from './client.ts';

describe('createApi', () => {
	it('sends the access token when there is one', async () => {
		const seen: (string | null)[] = [];
		const fetch: typeof globalThis.fetch = async (input) => {
			seen.push((input as Request).headers.get('Authorization'));
			return Response.json({ user: { id: 'u', email: 'a@b.de' }, workspaces: [] });
		};
		let token: string | null = 'tok123';
		const api = createApi({ baseUrl: 'http://test/api', fetch, token: () => token });

		const { data } = await api.GET('/me');
		expect(data?.workspaces).toEqual([]);
		token = null;
		await api.GET('/me');
		expect(seen).toEqual(['Bearer tok123', null]);
	});
});
