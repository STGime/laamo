import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	// Only the project URL and its PUBLIC key reach the browser (sign-in
	// talks to Eurobase auth directly). Read from the repo's .env or the
	// build environment; scripts/check-web-secrets.sh guards the rest.
	const env = { ...loadEnv(mode, '../..', 'EUROBASE_'), ...process.env };
	return {
		plugins: [
			sveltekit({
				// SPA: every route falls back to index.html; the Go API serves data.
				adapter: adapter({ fallback: 'index.html' })
			})
		],
		define: {
			__EUROBASE_URL__: JSON.stringify(env.EUROBASE_URL || 'https://laamo.eurobase.app'),
			__EUROBASE_PUBLIC_KEY__: JSON.stringify(env.EUROBASE_PUBLIC_KEY || '')
		},
		server: {
			port: 5173,
			proxy: {
				// Local dev: forward API calls to the Go api (task dev:api).
				'/api': process.env.API_PROXY_TARGET ?? 'http://localhost:8787'
			}
		}
	};
});
