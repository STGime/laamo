import { describe, expect, it } from 'vitest';
import { copy } from './copy';
import { INDUSTRIES } from './industries';
import { PAGES } from './site';
import { USE_CASES, useCase } from './usecases';

// All the site's words as one string per language.
function words(v: unknown): string {
	if (typeof v === 'string') return v;
	if (typeof v === 'function') return String(v('x@example.com'));
	if (Array.isArray(v)) return v.map(words).join(' ');
	if (v && typeof v === 'object') return Object.values(v).map(words).join(' ');
	return '';
}

const all = [words(copy), words(USE_CASES), words(INDUSTRIES)].join(' ');

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
	it('has every kind of business in both languages, each job with its use case', () => {
		for (const ind of INDUSTRIES) {
			for (const lang of ['en', 'de'] as const) {
				const t = ind.text[lang];
				expect(
					t.title &&
						t.metaTitle &&
						t.description &&
						t.intro.length &&
						t.week.length &&
						t.faq.length,
					`${ind.slug} ${lang}`
				).toBeTruthy();
				expect(t.jobs.length, `${ind.slug} ${lang}`).toBeGreaterThanOrEqual(3);
				for (const job of t.jobs) expect(useCase(job.useCase), job.useCase).toBeTruthy();
			}
			// The same jobs in both languages, in the same order.
			expect(ind.text.de.jobs.map((j) => j.useCase)).toEqual(
				ind.text.en.jobs.map((j) => j.useCase)
			);
			expect(PAGES.map((p) => p.path)).toContain(`/for/${ind.slug}`);
		}
	});
	it('keeps titles and descriptions a length search results show', () => {
		const pages = [
			...USE_CASES.flatMap((u) => [u.text.en, u.text.de]),
			...INDUSTRIES.flatMap((i) => [i.text.en, i.text.de])
		];
		for (const t of pages) {
			expect(t.metaTitle.length, t.metaTitle).toBeLessThanOrEqual(70);
			expect(t.description.length, t.metaTitle).toBeGreaterThanOrEqual(90);
			expect(t.description.length, t.metaTitle).toBeLessThanOrEqual(260);
		}
		// No two pages share a title.
		const titles = pages.map((t) => t.metaTitle);
		expect(new Set(titles).size).toBe(titles.length);
	});
});
