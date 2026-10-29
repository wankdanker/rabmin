/** Click-to-sort state for tables. */
export class Sorter<T> {
	key = $state('');
	dir = $state<1 | -1>(1);
	#get: (row: T, key: string) => unknown;

	constructor(key: string, dir: 1 | -1 = 1, get?: (row: T, key: string) => unknown) {
		this.key = key;
		this.dir = dir;
		this.#get = get ?? ((row, k) => k.split('.').reduce<any>((o, p) => o?.[p], row));
	}

	toggle(key: string, defaultDir: 1 | -1 = 1) {
		if (this.key === key) this.dir = this.dir === 1 ? -1 : 1;
		else {
			this.key = key;
			this.dir = defaultDir;
		}
	}

	sort(rows: T[]): T[] {
		const k = this.key;
		const d = this.dir;
		return [...rows].sort((a, b) => {
			const va = this.#get(a, k);
			const vb = this.#get(b, k);
			if (va == null && vb == null) return 0;
			if (va == null) return 1;
			if (vb == null) return -1;
			if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * d;
			if (typeof va === 'boolean' && typeof vb === 'boolean') return (Number(va) - Number(vb)) * d;
			return String(va).localeCompare(String(vb), undefined, { numeric: true }) * d;
		});
	}
}
