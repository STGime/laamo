import { describe, expect, it } from 'vitest';
import { copy } from './copy';
import { PAGES } from './site';
import { USE_CASES } from './usecases';

// All the site's words as one string per language.
function words(v: unknown): string {
	if (typeof v === 'string') return v;
	if (typeof v === 'function') return String(v('x@example.com'));
	if (Array.isArray(v)) return v.map(words).join(' ');
	if (v && typeof v === 'object') return Object.values(v).map(words).join(' ');
	return '';
}

const all = [words(copy), words(USE_CASES)].join(' ');

describe('site copy', () => {
	it('names no other tools, platforms or AI companies (Posta is ours)', () => {
		const brands =
			/\b(n8n|zapier|make\.com|hermes|tiktok|instagram|facebook|linkedin|youtube|bluesky|whatsapp|telegram|discord|slack|teams|gmail|outlook|google|apple|android|iphone|openai|chatgpt|claude|anthropic|mistral|gemini)\b/i;
		expect(all).not.toMatch(brands);
	});
	it('has no AI buzzwords', () => {
		expect(all).not.toMatch(
			/AI-powered|KI-gestützt|supercharge|seamless|effortless|revolution|unlock|game.?changer|honest take|dive in|fast-paced|✨/i
		);
	});
	it('has every use case in both languages, in the sitemap', () => {
		for (const u of USE_CASES) {
			for (const lang of ['en', 'de'] as const) {
				const t = u.text[lang];
				expect(
					t.title && t.description && t.stack.length && t.result.lines.length,
					`${u.slug} ${lang}`
				).toBeTruthy();
			}
			expect(PAGES.map((p) => p.path)).toContain(`/use-cases/${u.slug}`);
		}
	});
});
