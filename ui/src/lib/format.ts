const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

export function bytes(n: number | undefined | null, digits = 1): string {
	if (n == null || isNaN(n)) return '–';
	let i = 0;
	let v = Math.abs(n);
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return `${n < 0 ? '-' : ''}${i === 0 ? v : v.toFixed(digits)} ${units[i]}`;
}

export function num(n: number | undefined | null): string {
	if (n == null || isNaN(n)) return '–';
	return n.toLocaleString('en-US');
}

/** Compact number: 1.2k, 3.4M. */
export function compact(n: number | undefined | null, digits = 1): string {
	if (n == null || isNaN(n)) return '–';
	const a = Math.abs(n);
	if (a < 1000) return (Math.round(n * 10) / 10).toString();
	if (a < 1e6) return (n / 1e3).toFixed(digits) + 'k';
	if (a < 1e9) return (n / 1e6).toFixed(digits) + 'M';
	if (a < 1e12) return (n / 1e9).toFixed(digits) + 'G';
	return (n / 1e12).toFixed(digits) + 'T';
}

export function rate(n: number | undefined | null): string {
	return compact(n) + '/s';
}

export function bytesRate(n: number | undefined | null): string {
	return bytes(n) + '/s';
}

export function pct(part: number, total: number): number {
	if (!total) return 0;
	return Math.min(100, Math.max(0, (part / total) * 100));
}

/** Formats nanoseconds (Go time.Duration in JSON) as a human duration. */
export function nanos(ns: number | undefined | null): string {
	if (ns == null) return '–';
	if (ns === 0) return '0s';
	return duration(ns / 1e6);
}

/** Formats milliseconds as 1d 2h, 3m 4s, 120ms. */
export function duration(ms: number): string {
	if (ms < 1000) return `${Math.round(ms)}ms`;
	let s = Math.floor(ms / 1000);
	const d = Math.floor(s / 86400);
	s -= d * 86400;
	const h = Math.floor(s / 3600);
	s -= h * 3600;
	const m = Math.floor(s / 60);
	s -= m * 60;
	if (d) return `${d}d ${h}h`;
	if (h) return `${h}h ${m}m`;
	if (m) return `${m}m ${s}s`;
	return `${s}s`;
}

export function ago(t: string | Date | undefined | null): string {
	if (!t) return '–';
	const d = typeof t === 'string' ? new Date(t) : t;
	if (isNaN(d.getTime()) || d.getFullYear() < 1971) return '–';
	const ms = Date.now() - d.getTime();
	if (ms < 0) return 'in ' + duration(-ms);
	if (ms < 5000) return 'just now';
	return duration(ms) + ' ago';
}

export function dateTime(t: string | Date | undefined | null): string {
	if (!t) return '–';
	const d = typeof t === 'string' ? new Date(t) : t;
	if (isNaN(d.getTime()) || d.getFullYear() < 1971) return '–';
	return d.toLocaleString(undefined, {
		year: 'numeric',
		month: 'short',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit'
	});
}

/** Parses a Go duration string ("1h30m", "500ms") into nanoseconds. */
export function parseGoDuration(s: string): number | null {
	s = s.trim();
	if (!s) return 0;
	const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h|d)/g;
	const mult: Record<string, number> = { ns: 1, us: 1e3, µs: 1e3, ms: 1e6, s: 1e9, m: 6e10, h: 3.6e12, d: 8.64e13 };
	let total = 0;
	let consumed = 0;
	for (const m of s.matchAll(re)) {
		total += parseFloat(m[1]) * mult[m[2]];
		consumed += m[0].length;
	}
	if (consumed !== s.length) return null;
	return Math.round(total);
}

/** Formats nanoseconds as a Go duration string for inputs. */
export function toGoDuration(ns: number | undefined | null): string {
	if (!ns) return '';
	const parts: [number, string][] = [
		[8.64e13, 'd'],
		[3.6e12, 'h'],
		[6e10, 'm'],
		[1e9, 's'],
		[1e6, 'ms']
	];
	let rest = ns;
	let out = '';
	for (const [v, u] of parts) {
		if (u === 'd') continue;
		const n = Math.floor(rest / v);
		if (n) {
			out += n + u;
			rest -= n * v;
		}
	}
	return out || `${ns}ns`;
}

export function decodeBase64(b64: string): Uint8Array {
	const bin = atob(b64);
	const out = new Uint8Array(bin.length);
	for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
	return out;
}

export function encodeBase64(bytes: Uint8Array): string {
	let s = '';
	for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
	return btoa(s);
}

/** A hex dump of the first bytes of a payload. */
export function hexdump(b64: string, max = 1024): string {
	const b = decodeBase64(b64).slice(0, max);
	const lines: string[] = [];
	for (let i = 0; i < b.length; i += 16) {
		const chunk = Array.from(b.slice(i, i + 16));
		const hex = chunk.map((x) => x.toString(16).padStart(2, '0')).join(' ');
		const asc = chunk.map((x) => (x >= 32 && x < 127 ? String.fromCharCode(x) : '.')).join('');
		lines.push(`${i.toString(16).padStart(6, '0')}  ${hex.padEnd(47)}  ${asc}`);
	}
	return lines.join('\n');
}

export function prettyJSON(text: string): string {
	try {
		return JSON.stringify(JSON.parse(text), null, 2);
	} catch {
		return text;
	}
}
