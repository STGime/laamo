// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {}
	/** Eurobase project URL (vite.config.ts). */
	const __EUROBASE_URL__: string;
	/** The project's public key: safe in the browser, used only for auth. */
	const __EUROBASE_PUBLIC_KEY__: string;
	/** The public site's URL, e.g. https://laamo.app (vite.config.ts). */
	const __SITE_URL__: string;
	/** Whether search engines may index the site (production only). */
	const __SITE_INDEX__: boolean;
}

export {};
