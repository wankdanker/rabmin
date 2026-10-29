import type { Schemas } from './api/client';

type Role = 'viewer' | 'operator' | 'admin';
const rank: Record<Role, number> = { viewer: 1, operator: 2, admin: 3 };

/** The logged-in user, shared app-wide. */
export const session = $state<{ me: Schemas['MeBody'] | null; version: string }>({ me: null, version: '' });

export function can(role: Role): boolean {
	const r = session.me?.role as Role | undefined;
	return !!r && rank[r] >= rank[role];
}
