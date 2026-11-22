// Photos for Read the photo (the camera button): shrunk in the browser
// before they're sent, so a phone's 5 MB picture uploads fast on mobile
// data. The server shrinks and turns it again; this is only for speed.

/** The longest side we send, in pixels (the server keeps 1800). */
export const MAX_SIDE = 1800;
/** The most we send, in bytes (the API takes 1 MB per request). */
export const MAX_BYTES = 900_000;

/** The size a w×h picture is drawn at: its long side at most max. */
export function fit(w: number, h: number, max = MAX_SIDE): { w: number; h: number } {
	const long = Math.max(w, h);
	if (long <= max) return { w, h };
	return {
		w: Math.max(1, Math.round((w * max) / long)),
		h: Math.max(1, Math.round((h * max) / long))
	};
}

/**
 * A picture file as a JPEG the API takes: upright, at most MAX_SIDE on its
 * long side and at most MAX_BYTES. Throws if the browser can't read the file.
 */
export async function shrink(file: Blob): Promise<Blob> {
	const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' });
	try {
		for (const side of [MAX_SIDE, 1400, 1000]) {
			const { w, h } = fit(bitmap.width, bitmap.height, side);
			const canvas = document.createElement('canvas');
			canvas.width = w;
			canvas.height = h;
			const ctx = canvas.getContext('2d');
			if (!ctx) throw new Error('no canvas');
			// White under a see-through picture: JPEG has no transparency.
			ctx.fillStyle = '#fff';
			ctx.fillRect(0, 0, w, h);
			ctx.drawImage(bitmap, 0, 0, w, h);
			for (const quality of [0.85, 0.7, 0.55]) {
				const blob = await new Promise<Blob | null>((done) =>
					canvas.toBlob(done, 'image/jpeg', quality)
				);
				if (blob && blob.size <= MAX_BYTES) return blob;
			}
		}
	} finally {
		bitmap.close();
	}
	throw new Error('photo too big');
}
