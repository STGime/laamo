import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	// Only the project URL and its PUBLIC key reach the browser (sign-in
	// talks to Eurobase auth directly). Read from the repo's .env or the
	// build environment; scripts/check-web-secrets.sh guards the rest.
	const env = { ...loadEnv(mode, '../..', ['EUROBASE_', 'SITE_']), ...process.env };
	return {
		plugins: [
			sveltekit({
				// The site (routes/(site)) is prerendered to HTML; every other path
				// falls back to app.html, the browser-only app. The Go API serves data.
				adapter: adapter({ fallback: 'app.html' })
			})
		],
		define: {
			__EUROBASE_URL__: JSON.stringify(env.EUROBASE_URL || 'https://laamo.eurobase.app'),
			__EUROBASE_PUBLIC_KEY__: JSON.stringify(env.EUROBASE_PUBLIC_KEY || ''),
			// The public site: its own URL (canonical links, sitemap) and whether
			// search engines may index it (prod only: SITE_INDEX=1).
			__SITE_URL__: JSON.stringify(env.SITE_URL || 'https://laamo.app'),
			__SITE_INDEX__: JSON.stringify(env.SITE_INDEX === '1')
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
