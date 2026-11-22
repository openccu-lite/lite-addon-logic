import {mkdirSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';

import {expect, test, type Browser, type Page} from '@playwright/test';

// The README's screenshots, light and dark, the page in German (docs/screenshots/de/) and in
// English (docs/screenshots/en/) - only when asked for: scripts/screenshots.sh runs this with
// LOGIC_SCREENSHOTS set, against a daemon of its own (playwright.config.ts) that starts with the
// programs and variables of e2e/shot-seed on the fake system. Nothing here is a real system's.

const here = dirname(fileURLToPath(import.meta.url));
const OUT = join(here, '../../docs/screenshots');
const BASE = 'http://127.0.0.1:18305/addons/logic/';
const FAKE = 'http://127.0.0.1:18085/';
const H = {'X-CCU-Addon-Request': '1'};
const fixture = join(here, '../../internal/regadom/testdata/fixture.regadom');

test.skip(!process.env.LOGIC_SCREENSHOTS, 'only for scripts/screenshots.sh');
test.describe.configure({mode: 'serial'});
test.setTimeout(240_000);

const WORDS = {
    de: {programs: 'Programme', variables: 'Systemvariablen', schedule: 'Zeitplan', history: 'Verlauf', import: 'Import', triggers: 'Auslöser', settings: 'Einstellungen'},
    en: {programs: 'Programs', variables: 'System variables', schedule: 'Schedule', history: 'History', import: 'Import', triggers: 'Triggers', settings: 'Settings'},
} as const;

test('the data: a location, values from the devices, a few runs', async ({request}) => {
    expect((await request.put(BASE + 'api/v1/settings', {headers: H, data: {location: {latitude: 51.31, longitude: 9.48}}})).ok()).toBeTruthy();
    const report = async (address: string, param: string, value: unknown) =>
        expect((await request.post(FAKE + 'dev/event', {data: {interface: 'HmIP-RF', address, param, value}})).ok()).toBeTruthy();
    await report('000A1B2C3D4E04:1', 'ACTUAL_TEMPERATURE', 21.4);
    await report('000A1B2C3D4E04:1', 'HUMIDITY', 58);
    // the window opens: "holds for" starts counting (the Zeitplan shows it)
    await report('000A1B2C3D4E02:1', 'STATE', 1);
    // someone comes home, and the bell rings
    const set = async (key: string, value: unknown) => expect((await request.post(BASE + `api/v1/variables/${key}/value`, {headers: H, data: {value}})).ok()).toBeTruthy();
    await set('aussentemperatur', 7.5);
    await set('anwesend', false);
    await set('anwesend', true);
    await set('klingel', true);
    await expect.poll(async () => ((await (await request.get(BASE + 'api/v1/runs')).json()).runs ?? []).length, {timeout: 20_000}).toBeGreaterThanOrEqual(2);
    // a second version of a program, for the versions' comparison
    const program = (await (await request.get(BASE + 'api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === 'licht-ankunft').program;
    expect((await request.put(BASE + 'api/v1/programs/licht-ankunft', {headers: H, data: {program: {...program, note: 'Abends das Flurlicht für eine halbe Stunde, und der Heizmodus'}}})).ok()).toBeTruthy();
});

for (const lang of ['de', 'en'] as const) {
    test(`the page in ${lang}`, async ({browser}) => {
        mkdirSync(join(OUT, lang), {recursive: true});
        for (const dark of [false, true]) await shots(browser, lang, dark);
    });
}

async function shots(browser: Browser, lang: 'de' | 'en', dark: boolean) {
    const context = await browser.newContext({baseURL: BASE, locale: lang === 'de' ? 'de-DE' : 'en-GB', colorScheme: dark ? 'dark' : 'light', viewport: {width: 1280, height: 860}, timezoneId: 'Europe/Berlin'});
    const page = await context.newPage();
    const w = WORDS[lang];
    const path = (name: string) => join(OUT, lang, `${name}${dark ? '-dark' : ''}.png`);
    // a page: in a window high enough for all of it (the page scrolls inside itself), cut below
    // the last thing it shows; at most 1600 pixels
    const save = async (name: string) => {
        await page.mouse.move(0, 0);
        await page.setViewportSize({width: 1280, height: 1600});
        await page.waitForTimeout(500);
        const bottom = await page.evaluate(() =>
            Math.max(...[...document.querySelectorAll('body *')].filter((e) => e.children.length === 0).map((e) => e.getBoundingClientRect()).filter((r) => r.width > 0 && r.height > 0).map((r) => r.bottom)),
        );
        await page.screenshot({path: path(name), animations: 'disabled', caret: 'hide', clip: {x: 0, y: 0, width: 1280, height: Math.min(1600, Math.ceil(bottom + 16))}});
        await page.setViewportSize({width: 1280, height: 860});
    };
    // a dialog: the page down to the dialog's lower edge
    const saveDialog = async (name: string, testid: string) => {
        await page.mouse.move(0, 0);
        await page.waitForTimeout(400);
        const box = (await page.getByTestId(testid).boundingBox())!;
        await page.screenshot({path: path(name), animations: 'disabled', caret: 'hide', clip: {x: 0, y: 0, width: 1280, height: Math.min(860, Math.ceil(box.y + box.height + 24))}});
    };
    const tab = async (name: string) => {
        await page.getByRole('tab', {name, exact: true}).click();
        await page.waitForTimeout(500);
    };

    await page.goto('./');
    await expect(page.getByTestId('program-list')).toBeVisible();
    await save('01-programs');

    // a program as the CCU shows it
    await page.getByTestId('program-licht-ankunft').getByTestId('program-open').click();
    await expect(page.getByTestId('program-detail')).toBeVisible();
    await save('02-program');

    // the editor with a form open: the bell's sound
    await page.goto('./');
    await page.getByTestId('program-klingel').getByTestId('program-open').click();
    await page.getByTestId('program-detail').getByTestId('program-edit').click();
    const editor = page.getByTestId('program-editor');
    await expect(editor).toBeVisible();
    await save('03-editor');
    await editor.getByTestId('ed-action').nth(0).getByTestId('action-values').click();
    await expect(page.getByTestId('form-dialog')).toBeVisible();
    await saveDialog('04-form', 'form-dialog');

    // a command
    await page.goto('./');
    await page.getByTestId('program-wetter-holen').getByTestId('program-open').click();
    await page.getByTestId('program-detail').getByTestId('program-edit').click();
    await page.getByTestId('program-editor').getByTestId('ed-action').nth(0).getByTestId('action-command').click();
    await expect(page.getByTestId('command-dialog')).toBeVisible();
    await saveDialog('05-command', 'command-dialog');

    await page.goto('./');
    await tab(w.variables);
    await save('06-variables');
    await tab(w.schedule);
    await save('07-schedule');
    await tab(w.history);
    const runs = page.getByTestId('page-history').getByTestId('run');
    await expect(runs.first()).toBeVisible();
    await runs.filter({hasText: 'Licht bei Ankunft'}).first().getByTestId('run-head').click();
    await save('08-history');

    await tab(w.import);
    await page.getByTestId('import-file').setInputFiles(fixture);
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-summary')).toBeVisible();
    await save('09-import');
    await page.getByTestId('import-discard').click();
    await expect(page.getByTestId('import-file')).toBeVisible();

    // the guide's pictures (docs/de/handbuch.md, docs/en/guide.md)
    const open = async (key: string) => {
        await page.goto('./');
        await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
        await expect(page.getByTestId('program-detail')).toBeVisible();
    };
    const edit = async (key: string) => {
        await open(key);
        await page.getByTestId('program-detail').getByTestId('program-edit').click();
        await expect(page.getByTestId('program-editor')).toBeVisible();
        return page.getByTestId('program-editor');
    };
    let ed = await edit('klingel');
    await ed.getByTestId('ed-action').nth(0).getByTestId('action-device').click();
    await expect(page.getByTestId('device-picker')).toBeVisible();
    await saveDialog('10-picker', 'device-picker');
    ed = await edit('klingel');
    await ed.getByTestId('ed-action').nth(1).getByTestId('action-display').click();
    await expect(page.getByTestId('display-dialog')).toBeVisible();
    await saveDialog('11-display', 'display-dialog');

    ed = await edit('fenster-heizung');
    await ed.getByTestId('ed-line').nth(0).getByTestId('line-more').click();
    await expect(page.getByTestId('options-dialog')).toBeVisible();
    await saveDialog('12-options', 'options-dialog');

    ed = await edit('rollladen-abends');
    await ed.getByTestId('ed-line').nth(0).getByTestId('line-time').click();
    await expect(page.getByTestId('time-dialog')).toBeVisible();
    await saveDialog('13-time', 'time-dialog');

    await open('licht-ankunft');
    await page.getByTestId('dry-run').click();
    await expect(page.getByTestId('dry-run-dialog').getByTestId('sim-result')).toBeVisible();
    await saveDialog('14-dry-run', 'dry-run-dialog');
    await open('licht-ankunft');
    await page.getByTestId('program-versions').click();
    await expect(page.getByTestId('versions-dialog').getByTestId('version-diff')).toBeVisible();
    await saveDialog('15-versions', 'versions-dialog');

    await page.goto('./');
    await tab(w.variables);
    await page.getByTestId('helper-new').click();
    await expect(page.getByTestId('helper-dialog')).toBeVisible();
    await saveDialog('16-helper', 'helper-dialog');

    await page.goto('./');
    await tab(w.triggers);
    await save('17-triggers');
    await tab(w.settings);
    await save('18-settings');
    await tab(w.history);
    await runs.filter({hasText: 'Licht bei Ankunft'}).last().getByTestId('run-head').click();
    await save('19-not-triggered');
    await context.close();
}
