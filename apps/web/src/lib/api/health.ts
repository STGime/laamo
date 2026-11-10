import { api as defaultApi, type Api, type Schemas } from './client.ts';

export type Health = Schemas['Health'];

export async function getHealth(api: Api = defaultApi): Promise<Health> {
	const { data, response } = await api.GET('/healthz');
	if (!data) throw new Error(`API returned ${response.status}`);
	return data;
}
