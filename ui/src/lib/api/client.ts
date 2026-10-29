import createClient, { type Middleware } from 'openapi-fetch';
import type { paths, components } from './schema';

export type Schemas = components['schemas'];

/** Error raised for non-2xx API responses, carrying huma's error detail. */
export class ApiError extends Error {
	status: number;
	errors: { message?: string; location?: string }[];
	constructor(status: number, message: string, errors: { message?: string; location?: string }[] = []) {
		super(message);
		this.status = status;
		this.errors = errors;
	}
}

let onUnauthorized: (() => void) | null = null;

/** Registers the handler run when any request comes back 401. */
export function setUnauthorizedHandler(fn: () => void) {
	onUnauthorized = fn;
}

const authWatcher: Middleware = {
	onResponse({ response, request }) {
		if (response.status === 401 && !request.url.includes('/v1/auth/login') && !request.url.includes('/v1/status')) {
			onUnauthorized?.();
		}
		return response;
	}
};

export const api = createClient<paths>({ baseUrl: '/api', credentials: 'same-origin' });
api.use(authWatcher);

type Result<T> = { data?: T; error?: unknown; response: Response };

/** Unwraps an openapi-fetch result, throwing ApiError on failure. */
export async function unwrap<T>(p: Promise<Result<T>>): Promise<T> {
	const { data, error, response } = await p;
	if (!response.ok) {
		const e = (error ?? {}) as { detail?: string; title?: string; errors?: { message?: string; location?: string }[] };
		let msg = e.detail || e.title || response.statusText || `HTTP ${response.status}`;
		if (e.errors?.length) {
			msg += ': ' + e.errors.map((x) => [x.location, x.message].filter(Boolean).join(' ')).join('; ');
		}
		throw new ApiError(response.status, msg, e.errors);
	}
	return data as T;
}

/** Raw fetch for endpoints that are not JSON (downloads, uploads). */
export async function rawFetch(path: string, init?: RequestInit): Promise<Response> {
	const res = await fetch('/api' + path, { credentials: 'same-origin', ...init });
	if (res.status === 401) onUnauthorized?.();
	if (!res.ok) {
		let msg = res.statusText;
		try {
			const e = await res.json();
			msg = e.detail || e.title || msg;
		} catch {}
		throw new ApiError(res.status, msg);
	}
	return res;
}

export function errMsg(e: unknown): string {
	if (e instanceof Error) return e.message;
	return String(e);
}
