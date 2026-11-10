import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [
		sveltekit({
			// SPA: every route falls back to index.html; the Go API serves data.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: {
		port: 5173,
		proxy: {
			// Local dev: forward API calls to the Go api (task dev:api).
			'/api': process.env.API_PROXY_TARGET ?? 'http://localhost:8787'
		}
	}
});
