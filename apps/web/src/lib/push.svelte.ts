// Notifications on this device (Web Push): the browser subscribes at its
// maker's push service, we hand the subscription to the API, and the server
// sends a push when a helper puts something in the Feed. The service worker
// (static/sw.js) shows it.
//
// On an iPhone or iPad this only works once Laamo is on the Home Screen
// (iOS 16.4 or later); in a Safari tab there is no push.
import { api } from './session.svelte.ts';
import { i18n } from './i18n/index.svelte.ts';

// unknown: not checked yet. unsupported: this browser can't. homeScreen: an
// iPhone/iPad that needs Laamo on the Home Screen first. blocked: the person
// said no in the browser (only its settings can undo that). unavailable:
// the server has notifications off.
export type PushState =
	'unknown' | 'unsupported' | 'homeScreen' | 'blocked' | 'unavailable' | 'off' | 'on';

let state = $state<PushState>('unknown');
let busy = $state(false);
let failed = $state(false);

const DISMISSED = 'laamo.push.later';
function wasPostponed(): boolean {
	try {
		return globalThis.localStorage?.getItem(DISMISSED) === '1';
	} catch {
		return false;
	}
}
let postponed = $state(wasPostponed());

/** iPhone, iPad (which says it's a Mac) or iPod. */
export function isIOS(ua: string, touchPoints: number): boolean {
	return /iPad|iPhone|iPod/.test(ua) || (/Macintosh/.test(ua) && touchPoints > 1);
}

/** What this browser can do, before asking the service worker. */
export function capability(w: {
	hasWorker: boolean;
	hasPush: boolean;
	hasNotification: boolean;
	ios: boolean;
	standalone: boolean;
	permission?: string;
}): 'unsupported' | 'homeScreen' | 'blocked' | 'ok' {
	if (w.hasWorker && w.hasPush && w.hasNotification)
		return w.permission === 'denied' ? 'blocked' : 'ok';
	// iOS has push only for apps on the Home Screen.
	return w.ios && !w.standalone ? 'homeScreen' : 'unsupported';
}

/** The server's key as bytes, the way pushManager.subscribe wants it. */
export function keyBytes(base64url: string): Uint8Array<ArrayBuffer> {
	const b64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
	const raw = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
	const out = new Uint8Array(new ArrayBuffer(raw.length));
	for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
	return out;
}

function here() {
	const nav = globalThis.navigator;
	const standalone =
		(nav as Navigator & { standalone?: boolean })?.standalone === true ||
		globalThis.matchMedia?.('(display-mode: standalone)').matches === true;
	return capability({
		hasWorker: !!nav && 'serviceWorker' in nav,
		hasPush: 'PushManager' in globalThis,
		hasNotification: 'Notification' in globalThis,
		ios: !!nav && isIOS(nav.userAgent, nav.maxTouchPoints ?? 0),
		standalone,
		permission: globalThis.Notification?.permission
	});
}

async function registration(): Promise<ServiceWorkerRegistration> {
	await navigator.serviceWorker.register('/sw.js', { scope: '/' });
	return navigator.serviceWorker.ready;
}

async function save(sub: PushSubscription): Promise<boolean> {
	const j = sub.toJSON();
	if (!j.endpoint || !j.keys?.p256dh || !j.keys?.auth) return false;
	const { response } = await api.PUT('/me/push', {
		body: { endpoint: j.endpoint, keys: { p256dh: j.keys.p256dh, auth: j.keys.auth } }
	});
	return response.ok;
}

export const push = {
	get state() {
		return state;
	},
	get busy() {
		return busy;
	},
	/** The last try to turn it on or off didn't work. */
	get failed() {
		return failed;
	},
	/** Worth offering: it would work here, isn't on, and wasn't put off. */
	get offer() {
		return state === 'off' && !postponed;
	},
	/** "Not now": the Feed stops offering; the Me page still has the switch. */
	later() {
		postponed = true;
		try {
			globalThis.localStorage?.setItem(DISMISSED, '1');
		} catch {
			/* storage blocked: it's offered again next time */
		}
	},

	/**
	 * Looks at this browser and, when it has a subscription, hands it to the
	 * server again: push services renew subscriptions, and the person
	 * signed in here may be a different one than last time.
	 */
	async check() {
		const can = here();
		if (can !== 'ok') {
			state = can;
			return;
		}
		try {
			const reg = await registration();
			const sub = await reg.pushManager.getSubscription();
			if (!sub || Notification.permission !== 'granted') {
				state = 'off';
				return;
			}
			state = (await save(sub)) ? 'on' : 'off';
		} catch {
			state = 'unsupported';
		}
	},

	/** Turns notifications on. Call it from a tap: browsers ask only then. */
	async enable() {
		if (busy) return;
		busy = true;
		failed = false;
		try {
			// First, and before any waiting: Safari only asks while it still
			// knows a tap started this.
			const answer = await Notification.requestPermission();
			if (answer !== 'granted') {
				state = answer === 'denied' ? 'blocked' : 'off';
				return;
			}
			const { data, response } = await api.GET('/me/push');
			if (!data) {
				if (response.status === 503) state = 'unavailable';
				else failed = true;
				return;
			}
			const reg = await registration();
			const opts = { userVisibleOnly: true, applicationServerKey: keyBytes(data.public_key) };
			let sub: PushSubscription;
			try {
				sub = await reg.pushManager.subscribe(opts);
			} catch {
				// A subscription made with another server key: start over.
				await (await reg.pushManager.getSubscription())?.unsubscribe();
				sub = await reg.pushManager.subscribe(opts);
			}
			if (await save(sub)) state = 'on';
			else {
				await sub.unsubscribe();
				failed = true;
			}
		} catch {
			failed = true;
		} finally {
			busy = false;
		}
	},

	/** Turns notifications off on this device, here and on the server. */
	async disable() {
		if (busy) return;
		busy = true;
		failed = false;
		try {
			const reg = await navigator.serviceWorker.getRegistration('/');
			const sub = await reg?.pushManager.getSubscription();
			if (sub) {
				await api.DELETE('/me/push', { params: { query: { endpoint: sub.endpoint } } });
				await sub.unsubscribe();
			}
			state = 'off';
		} catch {
			failed = true;
		} finally {
			busy = false;
		}
	},

	/** Sends a test notification to this device. */
	async test(): Promise<boolean> {
		try {
			const reg = await navigator.serviceWorker.getRegistration('/');
			const sub = await reg?.pushManager.getSubscription();
			if (!sub) {
				state = 'off';
				return false;
			}
			const { response } = await api.POST('/me/push/test', {
				body: { endpoint: sub.endpoint, locale: i18n.lang }
			});
			if (response.status === 404) state = 'off';
			return response.ok;
		} catch {
			return false;
		}
	}
};

/** A tapped notification asks the open app to go somewhere (static/sw.js). */
export function onOpen(go: (path: string) => void): () => void {
	const sw = globalThis.navigator?.serviceWorker;
	if (!sw) return () => {};
	const handler = (e: MessageEvent) => {
		const d = e.data as { type?: string; url?: string } | null;
		if (d?.type !== 'laamo.open' || typeof d.url !== 'string') return;
		try {
			const u = new URL(d.url);
			if (u.origin === location.origin) go(u.pathname + u.search);
		} catch {
			/* not an address: ignore */
		}
	};
	sw.addEventListener('message', handler);
	return () => sw.removeEventListener('message', handler);
}
