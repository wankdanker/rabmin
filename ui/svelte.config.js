import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		// The Go server embeds the build and serves index.html for every client route.
		adapter: adapter({ pages: '../web/dist', assets: '../web/dist', fallback: 'index.html', strict: false })
	}
};
