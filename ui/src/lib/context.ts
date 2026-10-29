import { getContext, setContext } from 'svelte';
import type { LiveCluster } from './live.svelte';

const key = Symbol('live-cluster');

export function setLive(l: LiveCluster) {
	setContext(key, l);
}

/** The live cluster of the current /clusters/[cluster] route. */
export function getLive(): LiveCluster {
	return getContext<LiveCluster>(key);
}
