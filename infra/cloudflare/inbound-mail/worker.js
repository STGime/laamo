// Laamo incoming email: Cloudflare Email Routing sends every mail for
// *@in.laamo.app here (catch-all rule → this Worker). We pass the raw email
// on to the Laamo API, signed, and turn its answer into accept or reject.
//
// Settings (wrangler.toml / secrets):
//   API_URL         e.g. https://laamo.app/api/inbound/email
//   INBOUND_SECRET  the same value as the API's INBOUND_SECRET (wrangler secret put)

const MAX_BYTES = 15 * 1024 * 1024; // the API's mail.MaxInbound

// The reject texts have no full stop at the end: Cloudflare adds one, and
// the sender would read "…at this address..".

export default {
	async email(message, env) {
		if (message.rawSize > MAX_BYTES) {
			message.setReject('This email is too big for a Laamo helper (15 MB at most)');
			return;
		}
		const raw = new Uint8Array(await new Response(message.raw).arrayBuffer());
		const ts = Math.floor(Date.now() / 1000).toString();
		const signature = await sign(env.INBOUND_SECRET, ts, raw);

		const res = await fetch(env.API_URL, {
			method: 'POST',
			headers: {
				'Content-Type': 'message/rfc822',
				'X-Laamo-Timestamp': ts,
				'X-Laamo-Signature': signature,
				'X-Laamo-To': message.to
			},
			body: raw
		});

		switch (res.status) {
			case 202: // a run started, a duplicate, or ignored on purpose
				return;
			case 404:
				message.setReject('There is no Laamo helper at this address');
				return;
			case 410:
				message.setReject('This Laamo helper is switched off');
				return;
			case 413:
				message.setReject('This email is too big for a Laamo helper (15 MB at most)');
				return;
			case 422:
				message.setReject('This email could not be read');
				return;
			default:
				// 401 (secrets differ) or our API is down: fail this delivery so
				// it shows up in the Worker's logs instead of vanishing.
				throw new Error(`Laamo API answered ${res.status}`);
		}
	}
};

// sign is hex HMAC-SHA256 of "<timestamp>.<raw email>" (api.InboundSignature).
async function sign(secret, ts, raw) {
	const enc = new TextEncoder();
	const key = await crypto.subtle.importKey(
		'raw',
		enc.encode(secret),
		{ name: 'HMAC', hash: 'SHA-256' },
		false,
		['sign']
	);
	const prefix = enc.encode(ts + '.');
	const data = new Uint8Array(prefix.length + raw.length);
	data.set(prefix);
	data.set(raw, prefix.length);
	const mac = new Uint8Array(await crypto.subtle.sign('HMAC', key, data));
	return Array.from(mac, (b) => b.toString(16).padStart(2, '0')).join('');
}
