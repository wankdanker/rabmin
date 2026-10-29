import { api, unwrap, type Schemas } from './api/client';

type VHost = Schemas['VHost'];

/** Vhosts per cluster, cached for selectors. */
const cache = $state<Record<string, VHost[]>>({});

export async function loadVHosts(cluster: string, force = false): Promise<VHost[]> {
	if (!force && cache[cluster]) return cache[cluster];
	const list = await unwrap(api.GET('/v1/clusters/{cluster}/vhosts', { params: { path: { cluster } } }));
	cache[cluster] = list;
	return list;
}

export function vhostsOf(cluster: string): VHost[] {
	return cache[cluster] ?? [];
}
