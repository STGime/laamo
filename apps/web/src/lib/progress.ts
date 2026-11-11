import type { Schemas } from './api/client';
import { t } from './i18n/index.svelte.ts';

/** One line about a run that's going on now ("Running · step 2 of 5"). */
export function progressText(run: Schemas['RunProgress']): string {
	if (run.status === 'queued') return t('helpers.queued');
	if (run.status === 'waiting') return t('helpers.waiting');
	return t('helpers.running', { step: Math.min(run.step + 1, run.steps), steps: run.steps });
}

/** Whether Run now must wait (a run is queued or running). */
export function busy(run: Schemas['RunProgress'] | undefined): boolean {
	return !!run && run.status !== 'waiting';
}

type Brick = { id: string; group: string; title: string };

/**
 * Bricks of a run that's going on: done ones say what they made, the
 * current one is at work (or waiting for you), later ones are dim.
 * Returns the bricks with their line and the index of the current one.
 */
export function brickProgress(
	bricks: Brick[],
	run: Schemas['RunProgress'] | undefined,
	steps: Schemas['StepProgress'][] | undefined
): { items: (Brick & { sub: string })[]; current: number } {
	const current = run ? run.step : -1;
	const items = bricks.map((b, i) => {
		const step = run ? steps?.find((s) => s.brick_id === b.id) : undefined;
		let sub = '';
		if (step?.status === 'done' && b.group !== 'when')
			sub = t('helper.cards', { n: step.cards_out });
		else if (run && i === current)
			sub = run.status === 'waiting' ? t('helper.waitsHere') : t('helper.atWork');
		return { ...b, sub };
	});
	return { items, current };
}
