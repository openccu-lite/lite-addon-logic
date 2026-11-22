import {defineConfig, devices} from '@playwright/test';

// The page against the daemon and the fake system (scripts/dev-serve.sh): build the page first
// (npm run build). Run as `env -u WAYLAND_DISPLAY -u DISPLAY npx playwright test` on WSL.
// Four projects: desktop and phone width, each light and dark.
const phone = {viewport: {width: 390, height: 844}, isMobile: true, hasTouch: true};
export default defineConfig({
    testDir: 'e2e',
    timeout: 30_000,
    // the page asks the daemon every five seconds: what a test waits for may take that long
    expect: {timeout: 12_000},
    workers: 1,
    reporter: 'list',
    use: {baseURL: 'http://127.0.0.1:18301/addons/logic/', locale: 'en-GB'},
    projects: [
        {name: 'desktop-light', use: {...devices['Desktop Chrome'], colorScheme: 'light'}},
        {name: 'desktop-dark', use: {...devices['Desktop Chrome'], colorScheme: 'dark'}},
        {name: 'phone-light', use: {...devices['Desktop Chrome'], ...phone, colorScheme: 'light'}},
        {name: 'phone-dark', use: {...devices['Desktop Chrome'], ...phone, colorScheme: 'dark'}},
    ],
    webServer: [
        {
            command: '../scripts/dev-serve.sh',
            url: 'http://127.0.0.1:18301/addons/logic/api/v1/health',
            reuseExistingServer: false,
            timeout: 120_000,
        },
        // a second daemon for the time modules (e2e/time.spec.ts): in Europe/Berlin, with the
        // programs of e2e/time-seed, on a clock that stands still until the tests move it
        {
            command: '../scripts/dev-serve.sh',
            env: {LOGIC_PORT: '18302', LOGIC_RPC_PORT: '39312', LOGIC_SYSTEM_PORT: '18082', LOGIC_DEV_CLOCK: '2026-11-21T06:00:00+01:00', LOGIC_SEED: 'ui/e2e/time-seed', TZ: 'Europe/Berlin'},
            url: 'http://127.0.0.1:18302/addons/logic/api/v1/health',
            reuseExistingServer: false,
            timeout: 120_000,
        },
        // a third one for the lists' shape (e2e/tables.spec.ts): names and texts too long for their columns
        {
            command: '../scripts/dev-serve.sh',
            env: {LOGIC_PORT: '18303', LOGIC_RPC_PORT: '39313', LOGIC_SYSTEM_PORT: '18083', LOGIC_SEED: 'ui/e2e/table-seed'},
            url: 'http://127.0.0.1:18303/addons/logic/api/v1/health',
            reuseExistingServer: false,
            timeout: 120_000,
        },
        // a fourth one for what the page says about broken data (e2e/states.spec.ts): a file that is
        // no program, a program that names a variable there is none of
        {
            command: '../scripts/dev-serve.sh',
            env: {LOGIC_PORT: '18304', LOGIC_RPC_PORT: '39314', LOGIC_SYSTEM_PORT: '18084', LOGIC_SEED: 'ui/e2e/state-seed'},
            url: 'http://127.0.0.1:18304/addons/logic/api/v1/health',
            reuseExistingServer: false,
            timeout: 120_000,
        },
        // a fifth one for the README's screenshots (e2e/screenshots.spec.ts, scripts/screenshots.sh): only
        // when they are asked for, with the programs and variables of e2e/shot-seed
        ...(process.env.LOGIC_SCREENSHOTS
            ? [{
                command: '../scripts/dev-serve.sh',
                env: {LOGIC_PORT: '18305', LOGIC_RPC_PORT: '39315', LOGIC_SYSTEM_PORT: '18085', LOGIC_SEED: 'ui/e2e/shot-seed', TZ: 'Europe/Berlin'},
                url: 'http://127.0.0.1:18305/addons/logic/api/v1/health',
                reuseExistingServer: false,
                timeout: 120_000,
            }]
            : []),
    ],
});
