import { describe, expect, it } from 'vitest';
import { capability, isIOS, keyBytes } from './push.svelte.ts';

const iphone =
	'Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1';
const ipad =
	'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15';
const android =
	'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36';

describe('isIOS', () => {
	it('knows iPhones, and iPads that say they are Macs', () => {
		expect(isIOS(iphone, 5)).toBe(true);
		expect(isIOS(ipad, 5)).toBe(true);
		expect(isIOS(ipad, 0)).toBe(false); // a real Mac
		expect(isIOS(android, 5)).toBe(false);
	});
});

describe('capability', () => {
	const all = { hasWorker: true, hasPush: true, hasNotification: true, ios: false };
	it('works where the browser has push', () => {
		expect(capability({ ...all, standalone: false, permission: 'default' })).toBe('ok');
		expect(capability({ ...all, standalone: false, permission: 'granted' })).toBe('ok');
	});
	it('says so when the person blocked notifications', () => {
		expect(capability({ ...all, standalone: false, permission: 'denied' })).toBe('blocked');
	});
	it('asks iPhone users in a Safari tab to add Laamo to the Home Screen', () => {
		const tab = { hasWorker: true, hasPush: false, hasNotification: false, ios: true };
		expect(capability({ ...tab, standalone: false })).toBe('homeScreen');
		// From the Home Screen on an old iOS: nothing to be done.
		expect(capability({ ...tab, standalone: true })).toBe('unsupported');
		// From the Home Screen on iOS 16.4+: push is there.
		expect(capability({ ...all, ios: true, standalone: true, permission: 'default' })).toBe('ok');
	});
	it('gives up on browsers without push', () => {
		expect(
			capability({
				hasWorker: false,
				hasPush: false,
				hasNotification: true,
				ios: false,
				standalone: false
			})
		).toBe('unsupported');
	});
});

describe('keyBytes', () => {
	it('decodes the server key (base64url without padding)', () => {
		// 65 bytes: 0x04, then 0..63.
		const bytes = Uint8Array.from([4, ...Array.from({ length: 64 }, (_, i) => i)]);
		const b64url = btoa(String.fromCharCode(...bytes))
			.replace(/\+/g, '-')
			.replace(/\//g, '_')
			.replace(/=+$/, '');
		expect(Array.from(keyBytes(b64url))).toEqual(Array.from(bytes));
	});
});
