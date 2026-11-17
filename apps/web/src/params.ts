// URL param rules. lang: the site's second language, /de/… (English has no prefix).
import { defineParams } from '@sveltejs/kit/params';

export const params = defineParams({
	lang: (param) => (param === 'de' ? param : undefined)
});
