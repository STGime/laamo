import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { brick, catalog, group, matches, withDefaults } from '@laamo/catalog';

describe('brick catalog', () => {
	it('loads the shared bricks.json', () => {
		expect(catalog.schema).toBe(1);
		expect(catalog.groups.map((g) => g.id)).toEqual(['when', 'look', 'think', 'do', 'ask']);
		expect(brick('look.search')?.group).toBe('look');
	});

	it('has a design token for every brick color', () => {
		const tokens = readFileSync(new URL('../styles/tokens.css', import.meta.url), 'utf8');
		for (const g of catalog.groups) expect(tokens).toContain(`--brick-${g.color}:`);
	});

	it('shows the days setting only for weekly schedules', () => {
		const every = brick('when.every')!;
		const days = every.settings.find((s) => s.key === 'days')!;
		expect(matches(days.show_if, withDefaults(every, {}))).toBe(false);
		expect(matches(days.show_if, withDefaults(every, { freq: 'weekly' }))).toBe(true);
		expect(group('when').color).toBe('yellow');
	});
});
