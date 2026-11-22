import {svelte} from '@sveltejs/vite-plugin-svelte';
import {defineConfig} from 'vitest/config';

// The page is served by the addon's Go backend at /addons/logic/, so every asset URL is relative.
export default defineConfig({
    base: './',
    plugins: [svelte()],
    resolve: {dedupe: ['svelte']},
    build: {
        outDir: 'dist',
        emptyOutDir: true,
        target: 'es2022',
        sourcemap: false,
    },
    server: {
        // development against a backend started with -no-session-check -port 8301
        proxy: {'/addons/logic/api': 'http://127.0.0.1:8301'},
    },
    test: {
        environment: 'jsdom',
        include: ['src/**/*.test.ts'],
        // the page's logic lives in the .ts modules and is held to a floor here; the components are
        // covered by the Playwright specs
        coverage: {
            provider: 'v8',
            include: ['src/**/*.ts'],
            exclude: ['src/**/*.test.ts', 'src/main.ts', 'src/messages.ts'],
            thresholds: {lines: 90, functions: 90, branches: 85, statements: 90},
        },
    },
});
