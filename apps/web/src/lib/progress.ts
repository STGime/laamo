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
