// What the app is waiting for from the server right now. The layout shows
// a moving bar while anything is on its way, so a tap that takes a moment
// is seen to have worked (and isn't tapped again).
let pending = $state(0);

export const busy = {
	/** Requests on their way. */
	get pending() {
		return pending;
	}
};

/** fetch, counted while it runs. */
export const countedFetch: typeof fetch = (input, init) => {
	pending++;
	return fetch(input, init).finally(() => {
		pending = Math.max(0, pending - 1);
	});
};
