import { describe, expect, it } from 'vitest';
import { createApi } from './client.ts';
import { getHealth } from './health.ts';

const apiWith = (fetch: typeof globalThis.fetch) =>
	createApi({ baseUrl: 'http://test/api', fetch });

describe('getHealth', () => {
	it('calls GET /api/healthz through the generated client', async () => {
		let called = '';
		const fake: typeof fetch = async (input) => {
			const req = input as Request;
			called = `${req.method} ${new URL(req.url).pathname}`;
			return Response.json({ status: 'ok', env: 'dev', eurobase: 'reachable' });
		};
		const health = await getHealth(apiWith(fake));
		expect(called).toBe('GET /api/healthz');
		expect(health.eurobase).toBe('reachable');
	});

	it('throws on an error status', async () => {
		const fake: typeof fetch = async () =>
			Response.json({ error: 'down', code: 'internal' }, { status: 500 });
		await expect(getHealth(apiWith(fake))).rejects.toThrow('500');
	});
});
