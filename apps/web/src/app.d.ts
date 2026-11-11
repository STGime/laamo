// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {}
	/** Eurobase project URL (vite.config.ts). */
	const __EUROBASE_URL__: string;
	/** The project's public key: safe in the browser, used only for auth. */
	const __EUROBASE_PUBLIC_KEY__: string;
}

export {};
