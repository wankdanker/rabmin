import { api, unwrap, type Schemas } from './api/client';

/** Registered clusters, shared by the sidebar and pages. */
export const clusters = $state<{ list: Schemas['ClusterOut'][]; loaded: boolean }>({ list: [], loaded: false });

export async function refreshClusters() {
	clusters.list = await unwrap(api.GET('/v1/clusters'));
	clusters.loaded = true;
}

export function findCluster(id: string) {
	return clusters.list.find((c) => c.id === id);
}

/** A cluster's overall state for badges and dots. */
export function clusterState(c: Schemas['ClusterOut']): { tone: '' | 'ok' | 'warn' | 'bad'; text: string } {
	const s = c.status;
	if (!s.lastPoll) return { tone: '', text: 'Connecting' };
	if (!s.connected) return { tone: 'bad', text: 'Unreachable' };
	if (s.nodesRunning < s.nodes || s.alarms > 0) return { tone: 'bad', text: s.alarms > 0 ? 'Alarm' : 'Node down' };
	if (s.healthy < s.nodes || s.attention > 0) return { tone: 'warn', text: 'Needs attention' };
	return { tone: 'ok', text: 'Healthy' };
}
