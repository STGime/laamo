import { describe, expect, it } from 'vitest';
import { AuthError, createAuth, expiresIn } from './auth';

function fakeFetch(
	answers: Record<string, [number, unknown]>,
	calls: { path: string; body: any }[]
) {
	return (async (input: RequestInfo | URL, init?: RequestInit) => {
		const path = String(input).replace('https://eb.test/v1/auth/', '');
		calls.push({ path, body: JSON.parse(String(init?.body)) });
		expect((init?.headers as Record<string, string>).apikey).toBe('eb_pk_test');
		const [status, body] = answers[path] ?? [404, { error: 'no' }];
		return new Response(JSON.stringify(body), { status });
	}) as typeof fetch;
}

describe('auth', () => {
	it('signs up a new email with a random password, then sends a link', async () => {
		const calls: { path: string; body: any }[] = [];
		const auth = createAuth({
			url: 'https://eb.test',
			key: 'eb_pk_test',
			fetch: fakeFetch({ signup: [201, {}], 'request-magic-link': [200, {}] }, calls)
		});
		await auth.sendLink(' Anna@Example.com ', 'http://localhost:5173/auth/callback');
		expect(calls.map((c) => c.path)).toEqual(['signup', 'request-magic-link']);
		expect(calls[0].body.email).toBe('anna@example.com');
		expect(calls[0].body.password.length).toBeGreaterThanOrEqual(40);
		expect(calls[1].body).toEqual({
			email: 'anna@example.com',
			email_redirect_to: 'http://localhost:5173/auth/callback'
		});
	});
	it('treats an existing email like a new one (no enumeration)', async () => {
		const calls: { path: string; body: any }[] = [];
		const auth = createAuth({
			url: 'https://eb.test',
			key: 'eb_pk_test',
			fetch: fakeFetch(
				{ signup: [400, { error: 'email already registered' }], 'request-magic-link': [200, {}] },
				calls
			)
		});
		await auth.sendLink('anna@example.com', 'x');
		expect(calls.map((c) => c.path)).toEqual(['signup', 'request-magic-link']);
	});
	it('stops on rate limits and server errors', async () => {
		const auth = createAuth({
			url: 'https://eb.test',
			key: 'eb_pk_test',
			fetch: fakeFetch({ signup: [429, { error: 'slow down' }] }, [])
		});
		await expect(auth.sendLink('a@b.c', 'x')).rejects.toBeInstanceOf(AuthError);
	});
	it('exchanges the link token', async () => {
		const auth = createAuth({
			url: 'https://eb.test',
			key: 'eb_pk_test',
			fetch: fakeFetch(
				{ 'signin-magic-link': [200, { access_token: 'a', refresh_token: 'r' }] },
				[]
			)
		});
		expect(await auth.signIn('tok')).toEqual({ access_token: 'a', refresh_token: 'r' });
	});
	it('reads JWT expiry', () => {
		const jwt = 'x.' + btoa(JSON.stringify({ exp: 2000 })) + '.y';
		expect(expiresIn(jwt, 1_000_000)).toBe(1000);
		expect(expiresIn('nonsense')).toBe(0);
	});
});
