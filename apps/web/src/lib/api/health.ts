// Placeholder until the typed client is generated from OpenAPI (issue #11).
export type Health = { status: string; env: string; eurobase: string };

export async function getHealth(fetchFn: typeof fetch = fetch): Promise<Health> {
	const res = await fetchFn('/api/healthz');
	if (!res.ok) throw new Error(`API returned ${res.status}`);
	return res.json();
}
