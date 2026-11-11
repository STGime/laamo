// Look & feel lint (product plan, "Never" list). Fails on:
// - gradients, glows/blurs, glassmorphism
// - box-shadow other than the brick edge (or none)
// - raw colors outside src/lib/styles/tokens.css
// - fonts other than the token stacks, or fonts from a CDN
// - scroll-triggered animation helpers
// - AI buzzwords and sparkle icons in copy
// Usage: node scripts/check-style.mjs [dir] (default: src)
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const root = new URL('..', import.meta.url).pathname;
const dir = join(root, process.argv[2] ?? 'src');
const TOKENS = join(root, 'src/lib/styles/tokens.css');

const rules = [
	{ re: /(linear|radial|conic)-gradient\s*\(/i, msg: 'no gradients' },
	{ re: /backdrop-filter|filter\s*:\s*[^;]*blur\(/i, msg: 'no blur or glass effects' },
	{ re: /text-shadow\s*:/i, msg: 'no text shadows or glows' },
	{
		re: /box-shadow\s*:\s*(?!\s*(none|var\(--brick-edge\))\s*[;}])/i,
		msg: 'box-shadow only as var(--brick-edge) or none'
	},
	{
		re: /font-family\s*:\s*(?!\s*(inherit|var\(--font-(ui|display)\))\s*[;}])/i,
		msg: 'fonts only via var(--font-ui) or var(--font-display)',
		skipTokens: true
	},
	{
		re: /fonts\.(googleapis|gstatic)\.com|use\.typekit|fonts\.bunny/i,
		msg: 'self-host fonts, no font CDN'
	},
	{
		re: /IntersectionObserver|data-aos|scroll-timeline|animation-timeline/i,
		msg: 'no scroll-triggered animation'
	},
	{
		re: /#[0-9a-f]{3,8}\b(?![^<]*<\/(a|code)>)|\brgba?\(|\bhsla?\(/i,
		msg: 'colors only from tokens (var(--…))',
		skipTokens: true,
		cssOnly: true
	},
	{
		re: /\bAI[- ]powered\b|\bsupercharg|\bunlock\b|\bseamless|\beffortless|\brevolutioni[sz]|✨/i,
		msg: 'no AI buzzwords or sparkles in copy'
	}
];

function files(d) {
	const out = [];
	for (const name of readdirSync(d)) {
		const p = join(d, name);
		if (statSync(p).isDirectory()) out.push(...files(p));
		else if (
			/\.(svelte|css|ts|json|html)$/.test(name) &&
			!name.endsWith('.d.ts') &&
			!name.endsWith('.test.ts')
		)
			out.push(p);
	}
	return out;
}

/** The parts of a file that are CSS: .css files, <style> blocks and style="…" attributes. */
function cssOf(path, text) {
	if (path.endsWith('.css')) return text;
	if (!path.endsWith('.svelte')) return '';
	const parts = [...text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]);
	parts.push(...[...text.matchAll(/style(?::[\w-]+)?="([^"]*)"/g)].map((m) => m[1]));
	return parts.join('\n');
}

let problems = 0;
for (const f of files(dir)) {
	const text = readFileSync(f, 'utf8');
	const css = cssOf(f, text);
	for (const r of rules) {
		if (r.skipTokens && f === TOKENS) continue;
		const hay = r.cssOnly ? css : text;
		hay.split('\n').forEach((line, i) => {
			if (r.re.test(line) && !line.includes('style-ok')) {
				problems++;
				console.error(`${relative(root, f)}: ${r.msg}\n    ${line.trim()}`);
			}
		});
	}
}
if (problems) {
	console.error(`\n${problems} look & feel problem(s). See "Look & feel" in the product plan.`);
	process.exit(1);
}
console.log('Look & feel check passed.');
