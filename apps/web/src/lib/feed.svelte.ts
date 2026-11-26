// The Feed: pages from the API, kept fresh by the live event stream
// (Server-Sent Events read with fetch, so the token stays in a header).
import type { Schemas } from './api/client';
import { api, session } from './session.svelte.ts';

export type FeedCard = Schemas['FeedCard'];
export type Filter = 'all' | 'needs_me' | 'results';

let cards = $state<FeedCard[]>([]);
let needsMe = $state(0);
let brain = $state<Schemas['BrainMeter'] | null>(null);
let next = $state<string | undefined>(undefined);
let filter = $state<Filter>('all');
let loading = $state(false);
let loaded = $state(false); // at least one page has come back
// Why the Feed didn't load: 0 = it did, -1 = no connection, else the HTTP status.
let failed = $state(0);

// Something new in the Feed since it was last looked at (the dot on the
// tab). "Looked at" is kept on this device; without storage the dot only
// knows this visit.
let unseen = $state(false);
const SEEN = 'laamo.feed.seen';
function seenAt(): string {
	try {
		return localStorage.getItem(SEEN) ?? '';
	} catch {
		return '';
	}
}

async function fetchPage(before?: string) {
	const ws = session.workspace;
	if (!ws) return null;
	const { data, response } = await api.GET('/workspaces/{workspaceId}/feed', {
		params: { path: { workspaceId: ws.id }, query: { filter, before, limit: 30 } }
	});
	if (!data) failed = response.status;
	return data ?? null;
}

export const feed = {
	get cards() {
		return cards;
	},
	get needsMe() {
		return needsMe;
	},
	get brain() {
		return brain;
	},
	get hasMore() {
		return !!next;
	},
	get loading() {
		return loading;
	},
	get loaded() {
		return loaded;
	},
	get failed() {
		return failed;
	},
	get filter() {
		return filter;
	},
	get unseen() {
		return unseen;
	},

	/** The Feed is on screen: what's in it now counts as seen. */
	seen() {
		unseen = false;
		try {
			localStorage.setItem(SEEN, new Date().toISOString());
		} catch {
			/* no storage: fine */
		}
	},

	/** Looks whether the newest card is newer than the last look (other screens). */
	async peek() {
		const ws = session.workspace;
		if (!ws) return;
		const { data } = await api.GET('/workspaces/{workspaceId}/feed', {
			params: { path: { workspaceId: ws.id }, query: { filter: 'all', limit: 1 } }
		});
		if (!data) return;
		needsMe = data.needs_me;
		const newest = data.cards[0]?.created_at ?? '';
		const last = seenAt();
		// Never looked on this device: don't greet with a dot.
		if (!last) return feed.seen();
		unseen = newest !== '' && new Date(newest) > new Date(last);
	},

	async load(f: Filter = filter) {
		filter = f;
		loading = true;
		try {
			failed = 0;
			const page = await fetchPage();
			if (page) {
				loaded = true;
				cards = page.cards;
				needsMe = page.needs_me;
				brain = page.brain;
				next = page.next_before;
			}
		} catch {
			failed = -1;
		} finally {
			loading = false;
		}
	},

	async more() {
		if (!next || loading) return;
		loading = true;
		try {
			const page = await fetchPage(next);
			if (page) {
				const seen = new Set(cards.map((c) => c.id));
				cards = [...cards, ...page.cards.filter((c) => !seen.has(c.id))];
				next = page.next_before;
			}
		} finally {
			loading = false;
		}
	},

	/** Replaces one card after an action (decide, dismiss). */
	update(card: FeedCard) {
		if (card.state === 'dismissed') cards = cards.filter((c) => c.id !== card.id);
		else cards = cards.map((c) => (c.id === card.id ? card : c));
		if (card.state !== 'open' && ['approval', 'pick', 'question'].includes(card.type))
			needsMe = Math.max(0, needsMe - 1);
	}
};

/** Parses a text/event-stream chunk buffer into events; returns the rest. */
export function parseSSE(buffer: string): {
	events: { event: string; data: string }[];
	rest: string;
} {
	const events: { event: string; data: string }[] = [];
	const blocks = buffer.split('\n\n');
	const rest = blocks.pop() ?? '';
	for (const block of blocks) {
		let event = 'message';
		const data: string[] = [];
		for (const line of block.split('\n')) {
			if (line.startsWith(':')) continue;
			if (line.startsWith('event:')) event = line.slice(6).trim();
			else if (line.startsWith('data:')) data.push(line.slice(5).trim());
		}
		if (data.length || event !== 'message') events.push({ event, data: data.join('\n') });
	}
	return { events, rest };
}

/** How often a screen with a run going looks again by itself. */
export const RECHECK_MS = 3000;

/**
 * Listens to the workspace's live updates until the returned stop function
 * is called. Events of the given kinds call onChange (debounced); a dropped
 * stream reconnects with backoff and calls it too.
 */
export function listen(
	onChange: () => void,
	token: () => Promise<string | null>,
	kinds: string[] = ['feed', 'reload'],
	/**
	 * A run is going on this screen: look again every few seconds as well.
	 * Live updates can go missing (a dropped stream, a server just started),
	 * and a run must never look stuck when it is long done.
	 */
	going?: () => boolean
): () => void {
	let stopped = false;
	const tick = going ? setInterval(() => going() && onChange(), RECHECK_MS) : undefined;
	let ctrl: AbortController | null = null;
	let timer: ReturnType<typeof setTimeout> | undefined;
	const changed = () => {
		clearTimeout(timer);
		timer = setTimeout(onChange, 250);
	};

	async function run() {
		let wait = 1000;
		while (!stopped) {
			const ws = session.workspace;
			const t = await token();
			if (!ws || !t) return;
			ctrl = new AbortController();
			const started = Date.now();
			try {
				const res = await fetch(`/api/workspaces/${ws.id}/events`, {
					headers: { Authorization: `Bearer ${t}`, Accept: 'text/event-stream' },
					signal: ctrl.signal
				});
				if (!res.ok || !res.body) throw new Error(`events: ${res.status}`);
				const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
				let buf = '';
				for (;;) {
					const { value, done } = await reader.read();
					if (done) break;
					const parsed = parseSSE(buf + value);
					buf = parsed.rest;
					// Each screen listens for what it shows: the Feed for its cards,
					// helper screens for run and step progress.
					if (parsed.events.some((e) => kinds.includes(e.event))) changed();
				}
			} catch {
				/* reconnect below */
			}
			if (stopped) return;
			if (Date.now() - started > 60_000) wait = 1000;
			await new Promise((r) => setTimeout(r, wait));
			wait = Math.min(wait * 2, 30_000);
			changed(); // we may have missed something
		}
	}
	run();
	return () => {
		stopped = true;
		clearTimeout(timer);
		clearInterval(tick);
		ctrl?.abort();
	};
}
