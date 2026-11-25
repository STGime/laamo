// Laamo's service worker. It does one thing: show notifications (Web Push)
// and open the app when one is tapped. It caches nothing and handles no
// requests, so the app always comes fresh from the network.
//
// Plain JavaScript on purpose: this file is served as it is, at /sw.js.

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));

// A path in the app, never another site.
function appUrl(path) {
	const ok = typeof path === 'string' && path.startsWith('/') && !path.startsWith('//');
	return new URL(ok ? path : '/feed', self.location.origin).href;
}

self.addEventListener('push', (event) => {
	let m = {};
	try {
		m = event.data ? event.data.json() : {};
	} catch {
		// Not ours: show the plain notification below. Browsers (Safari
		// first) take a subscription away from sites that stay silent.
	}
	const options = {
		body: typeof m.body === 'string' ? m.body : '',
		icon: '/icon-192.png',
		data: { url: appUrl(m.url) }
	};
	// Same tag: the newer notification replaces the older, and still rings.
	if (typeof m.tag === 'string' && m.tag) {
		options.tag = m.tag;
		options.renotify = true;
	}
	const title = typeof m.title === 'string' && m.title ? m.title : 'Laamo';
	event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
	event.notification.close();
	const url = appUrl(event.notification.data && event.notification.data.url);
	event.waitUntil(
		(async () => {
			const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
			const open = windows.find((w) => new URL(w.url).origin === self.location.origin);
			if (open) {
				// The app is open: bring it forward and let it go to the page.
				open.postMessage({ type: 'laamo.open', url });
				return open.focus();
			}
			return self.clients.openWindow(url);
		})()
	);
});
