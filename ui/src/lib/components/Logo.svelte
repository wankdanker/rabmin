<script lang="ts">
	let { size = 24 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true" style="flex:none">
	<rect width="32" height="32" rx="7" fill="var(--accent)" />
	<path d="M11 16 17 9.5M11 16h6M11 16l6 6.5" fill="none" stroke="var(--accent-text)" stroke-width="1.8" stroke-linecap="round" opacity="0.75" />
	<circle cx="9.5" cy="16" r="3.4" fill="var(--accent-text)" />
	<rect x="17" y="7.6" width="8.5" height="3.8" rx="1.9" fill="var(--accent-text)" />
	<rect x="17" y="14.1" width="8.5" height="3.8" rx="1.9" fill="var(--accent-text)" />
	<rect x="17" y="20.6" width="8.5" height="3.8" rx="1.9" fill="var(--accent-text)" />
</svg>
