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
// Why the Feed didn't load: 0 = it did, -1 = no connection, else the HTTP status.
let failed = $state(0);

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
	get failed() {
		return failed;
	},
	get filter() {
		return filter;
	},

	async load(f: Filter = filter) {
		filter = f;
		loading = true;
		try {
			failed = 0;
			const page = await fetchPage();
			if (page) {
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

/**
 * Listens to the workspace's live updates until the returned stop function
 * is called. Any change reloads the first page (debounced); a dropped
 * stream reconnects with backoff and reloads.
 */
export function listen(onChange: () => void, token: () => Promise<string | null>): () => void {
	let stopped = false;
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
					if (parsed.events.length) changed();
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
		ctrl?.abort();
	};
}
