// Chats helpers post to (#34).

export const placeIcons: Record<string, string> = {
	discord: '🎮',
	slack: '💬',
	teams: '👥',
	telegram: '✈️',
	email: '✉️',
	posta: '📣'
};

/** Chat types that can be connected now; the rest come later (M4, post-MVP). */
export const connectable = ['discord', 'slack'] as const;
export type Connectable = (typeof connectable)[number];

/** Where to find the webhook link, step by step (i18n keys). */
export const guides: Record<Connectable, string[]> = {
	discord: ['places.guide.discord.1', 'places.guide.discord.2', 'places.guide.discord.3'],
	slack: ['places.guide.slack.1', 'places.guide.slack.2', 'places.guide.slack.3']
};

export const placeholders: Record<Connectable, string> = {
	discord: 'https://discord.com/api/webhooks/…',
	slack: 'https://hooks.slack.com/services/…'
};
