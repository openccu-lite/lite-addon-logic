import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';

import {expect, test} from '@playwright/test';

// The import end to end: the fixture database of a stock OpenCCU (twenty programs built the way
// the WebUI builds them) is read into a dry run, the report is looked at, the import is applied,
// and the programs are there - against the daemon on the fake system. The four projects share
// one daemon, so every project after the first imports the same source again: an update.
const here = dirname(fileURLToPath(import.meta.url));
const fixture = join(here, '../../internal/regadom/testdata/fixture.regadom');
const SYSTEM = 'http://127.0.0.1:18081';

test.describe.configure({mode: 'serial'});

test('a dry run shows the report and writes nothing; "Import" takes it over', async ({page}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Import'}).click();
    await expect(page.getByTestId('import-start')).toBeVisible();
    await expect(page.getByTestId('import-read-file')).toBeDisabled();
    // the way to the guide's chapter for those who come from a CCU
    await expect(page.getByTestId('import-from-ccu').getByTestId('help-from-ccu')).toHaveAttribute('href', /docs\/en\/#from-ccu$/);
    await page.getByTestId('import-file').setInputFiles(fixture);
    await page.getByTestId('import-read-file').click();

    // the report: 18 programs, 7 of them inactive because of a script, 2 counter programs left out
    await expect(page.getByTestId('import-summary')).toContainText('Would be imported: 18 programs (7 of them inactive) and 38 system variables; not imported: 2 programs.');
    await expect(page.getByTestId('import-scripts')).toContainText('Node-RED (RedMatic), Home Assistant or ioBroker');
    await expect(page.getByTestId('import-program')).toHaveCount(20);
    const wetter = page.getByTestId('import-program').filter({hasText: 'Wetter holen'});
    await expect(wetter).toContainText('inactive');
    await expect(wetter).toContainText('a `system.Exec` became the activity "command"');
    await expect(page.getByTestId('import-program').filter({hasText: 'zzL_Ia5'})).toContainText('holds a script');
    const counter = page.getByTestId('import-program').filter({hasText: 'prgDailySunshineRainCounter_4730'});
    await expect(counter).toContainText('not imported');
    await expect(counter).toContainText('a device helper replaces it');
    await expect(page.getByTestId('import-program').filter({hasText: 'Markise bei Regen einfahren'})).toContainText('ends running delays for its targets');
    // such notes are the CCU's quirks: the guide has them in plain words
    await expect(page.getByTestId('import-quirks').getByTestId('help-quirks')).toHaveAttribute('href', /docs\/en\/#quirks$/);

    // the variables are folded away until asked for
    await page.getByTestId('import-variables').locator('summary').click();
    await expect(page.getByTestId('import-variables')).toContainText('Außentemperatur Mittel');
    await expect(page.getByTestId('import-variables')).toContainText('12.5');

    await expect(page.getByTestId('import-report')).toBeVisible();

    // the report can be kept
    const [download] = await Promise.all([page.waitForEvent('download'), page.getByTestId('import-download').click()]);
    expect(download.suggestedFilename()).toBe('import-ccu.md');
    const md = readFileSync(await download.path(), 'utf8');
    expect(md).toContain('# Import report');
    expect(md).toContain('| Markise bei Regen einfahren | `programs/markise-bei-regen-einfahren.yaml` |');

    // the dry run survives a reload, and "Import" applies it
    await page.reload();
    await page.getByRole('tab', {name: 'Import'}).click();
    await expect(page.getByTestId('import-summary')).toBeVisible();
    await page.getByTestId('import-apply').click();
    await expect(page.getByTestId('import-done')).toContainText('Imported: 18 programs (7 of them inactive) and 38 system variables.');
    await page.getByTestId('import-to-programs').click();

    // the programs are there, in the CCU's layout
    await expect(page.getByTestId('program-markise-bei-regen-einfahren')).toBeVisible();
    await expect(page.getByTestId('program-markise-bei-regen-einfahren').getByTestId('program-state')).toHaveText('active');
    await expect(page.getByTestId('program-wetter-holen').getByTestId('program-state')).toHaveText('inactive');
    await expect(page.getByTestId('program-zzl-ia5')).toContainText('script');
    // the two counter programs were not imported; nothing internal is hidden
    await expect(page.getByTestId('show-internal')).toHaveCount(0);
});

test('a program is shown as the CCU shows it, and can be switched and run', async ({page}) => {
    await page.goto('./');
    await page.getByTestId('program-search').fill('markise regen');
    await expect(page.getByTestId('program-list').locator('tbody tr')).toHaveCount(1);
    await page.getByTestId('program-open').click();
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('program-name')).toHaveText('Markise bei Regen einfahren');
    await expect(detail.getByTestId('rule-0')).toContainText('Condition: If…');
    await expect(detail.getByTestId('rule-1')).toContainText('Else…');
    const lines = detail.getByTestId('rule-0').getByTestId('line');
    await expect(lines).toHaveCount(4);
    await expect(lines.nth(0)).toContainText('System variable');
    await expect(lines.nth(0)).toContainText('Regen');
    await expect(lines.nth(0)).toContainText('is es regnet');
    await expect(lines.nth(0)).toContainText('trigger when changed');
    await expect(lines.nth(1)).toContainText('greater than 5 °C');
    await expect(lines.nth(1)).toContainText('check only');
    await expect(lines.nth(3)).toContainText('trigger when updated');
    await expect(detail.getByTestId('rule-0')).toContainText('AND');
    await expect(detail.getByTestId('rule-0')).toContainText('OR');
    await expect(detail.getByTestId('rule-0').getByTestId('cancel-pending')).toBeVisible();
    const actions = detail.getByTestId('rule-0').getByTestId('action');
    await expect(actions.nth(0)).toContainText('to Beschattung');
    await expect(actions.nth(0)).toContainText('immediately');
    await expect(actions.nth(1)).toContainText('delayed by 30 s');

    // run the first "Then" by hand: the variable it sets has the value afterwards
    await detail.getByTestId('run-then').click();
    await expect(detail.getByTestId('detail-last-run')).toContainText('Then');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await expect(page.getByTestId('variable-rollladen-status').getByTestId('value')).toHaveValue('Beschattung');

    // switch the program off and on again
    await page.getByRole('tab', {name: 'Programs'}).click();
    const row = page.getByTestId('program-markise-bei-regen-einfahren');
    await row.getByTestId('program-active').uncheck();
    await expect(row.getByTestId('program-state')).toHaveText('inactive');
    await row.getByTestId('program-active').check();
    await expect(row.getByTestId('program-state')).toHaveText('active');
});

test('a script that was one system.Exec is a command: the line as written, through the shell, its output into the variable', async ({page}) => {
    await page.goto('./');
    await page.getByTestId('program-search').fill('wetter');
    // (a command is no script: the program can be switched on once it was looked at)
    await expect(page.getByTestId('program-wetter-holen').getByTestId('program-active')).toBeEnabled();
    await page.getByTestId('program-open').click();
    await expect(page.getByTestId('script-pointer')).toHaveCount(0);
    const action = page.getByTestId('program-detail').getByTestId('action').nth(0);
    await expect(action).toContainText('Command');
    await expect(action).toContainText('wget -q -O - http://example.invalid/wetter');
    await expect(action).toContainText('through a shell, the block waits for it, output into Markise-Hinweis');
    await page.getByTestId('program-back').click();
    await expect(page.getByTestId('program-list')).toBeVisible();
});

test('a script is kept as text, the program cannot be switched on, and the page points to other tools', async ({page}) => {
    await page.goto('./');
    await page.getByTestId('program-search').fill('zzL_Ia5');
    await expect(page.getByTestId('program-zzl-ia5').getByTestId('program-active')).toBeDisabled();
    await page.getByTestId('program-open').click();
    await expect(page.getByTestId('script-pointer')).toContainText('Node-RED (RedMatic), Home Assistant or ioBroker');
    await expect(page.getByTestId('script-text').first()).toContainText('dom.GetObject("zzL_TT")');
    // its time module knows its next times although the program is not active
    await expect(page.getByTestId('time-next')).toContainText(/next: \d\d\/\d\d\/\d{4}, \d\d:\d[05] · /);
    await expect(page.getByTestId('detail-active')).toBeDisabled();
    await page.getByTestId('program-back').click();
    await expect(page.getByTestId('program-list')).toBeVisible();
});

test('a time module program says when it would fire', async ({page}) => {
    await page.goto('./');
    await page.getByTestId('program-search').fill('nachtabsenkung');
    await page.getByTestId('program-open').click();
    await expect(page.getByTestId('rule-0').getByTestId('line').first()).toContainText('weekly Mon, Wed, from 22:00, for 8 h');
    await expect(page.getByTestId('rule-0').getByTestId('line').first()).toContainText('trigger at the times');
    // below the line: when it fires next
    await expect(page.getByTestId('rule-0').getByTestId('time-line').first()).toContainText('next:');
});

test('the database kept on the system: everything inactive, discarded', async ({page, request}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Import'}).click();
    await request.put(`${SYSTEM}/dev/regadom`, {data: readFileSync(fixture)});
    await page.reload();
    await page.getByRole('tab', {name: 'Import'}).click();
    await page.getByTestId('import-all-inactive').check();
    await page.getByTestId('import-read-system').click();
    await expect(page.getByTestId('import-summary')).toContainText('Every program is imported inactive.');
    // a second import of the same source updates the same files
    await expect(page.getByTestId('import-program').filter({hasText: 'Kalender'})).toContainText('updates the file of an earlier import');
    await page.getByTestId('import-discard').click();
    await expect(page.getByTestId('import-start')).toBeVisible();
    await request.delete(`${SYSTEM}/dev/regadom`);
    await page.reload();
    await page.getByRole('tab', {name: 'Import'}).click();
    await expect(page.getByTestId('import-no-system')).toBeVisible();
});

// The fixture's system had no radio, so its report lists no device. The second test database has
// them (internal/regadom/testdata/devices.regadom, written by hand): a key, a blind's two
// channels and a CUxD channel. None of its addresses is a device of the fake system - here the
// key gets the address of one that is, so the table shows both a channel that is there and
// channels that are missing. The dry run is discarded: nothing of it reaches the tree.
// The room Terrasse becomes one a CCU comes with, kept by its key roomTerrace: the report names it
// as the WebUI shows it.
const withDevices = () => Buffer.from(readFileSync(join(here, '../../internal/regadom/testdata/devices.regadom'), 'latin1').replaceAll('LEQ0123456', 'NEQ0000001').replace('<name>Terrasse</name>', '<name>roomTerrace</name>'), 'latin1');

test('the devices that programs use: present or missing on this system, with the warning', async ({page}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Import'}).click();
    await page.getByTestId('import-file').setInputFiles({name: 'devices.regadom', mimeType: 'application/octet-stream', buffer: withDevices()});
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-summary')).toContainText('Would be imported: 3 programs (3 of them inactive) and 3 system variables; not imported: 1 program.');
    // three channels are not there: the blind's two, and the CUxD one, which never is
    await expect(page.getByTestId('import-missing')).toHaveText('3 channels that programs use do not exist on this system: those programs stay inactive.');
    const devices = page.getByTestId('import-devices');
    await expect(devices).toHaveAttribute('open', '');
    await expect(devices.locator('summary')).toHaveText('Devices that programs use (4)');
    const rows = devices.getByTestId('import-device');
    await expect(rows).toHaveCount(4);
    const key = rows.filter({hasText: 'BidCos-RF/NEQ0000001:1'});
    await expect(key).toContainText('Taster Flur oben');
    await expect(key).toContainText('Taster Flur (HM-PB-2-WM55)');
    await expect(key).toContainText('Flur');
    await expect(key.locator('.lgc-pill')).toHaveText('present');
    await expect(key).toContainText('Markise bei Wind');
    const blind = rows.filter({hasText: 'HmIP-RF/00111BE9AB9012:4'});
    await expect(blind).toContainText('Markise Behang');
    await expect(blind).toContainText('Markise Terrasse (HmIP-BROLL)');
    await expect(blind).toContainText('Terrace, Flur');
    await expect(blind).not.toContainText('roomTerrace');
    await expect(blind.locator('.lgc-pill')).toHaveText('missing');
    const cuxd = rows.filter({hasText: 'CUxD/CUX2801001:1'});
    await expect(cuxd.locator('.lgc-pill')).toHaveText('missing');
    await expect(cuxd).toContainText('Frostwarnung');
    // the program says which of its channels are missing, and stays inactive
    const wind = page.getByTestId('import-program').filter({hasText: 'Markise bei Wind'});
    await expect(wind).toContainText('inactive');
    await expect(wind).toContainText('devices missing on this system: HmIP-RF/00111BE9AB9012:3, HmIP-RF/00111BE9AB9012:4');
    // the table keeps the page's width
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await page.getByTestId('import-discard').click();
    await expect(page.getByTestId('import-start')).toBeVisible();
});

test('a database without devices: no device table and no warning', async ({page}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Import'}).click();
    await page.getByTestId('import-file').setInputFiles({name: 'devices.regadom', mimeType: 'application/octet-stream', buffer: withDevices()});
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-devices')).toBeVisible();
    await page.getByTestId('import-discard').click();
    // the fixture has no device at all: no table, no warning
    await page.getByTestId('import-file').setInputFiles(fixture);
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-summary')).toBeVisible();
    await expect(page.getByTestId('import-devices')).toHaveCount(0);
    await expect(page.getByTestId('import-missing')).toHaveCount(0);
    await page.getByTestId('import-discard').click();
});

test('a file that is neither a backup nor a database is refused with the reason', async ({page}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Import'}).click();
    await page.getByTestId('import-file').setInputFiles({name: 'foto.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('this is not a backup, whatever its name says '.repeat(20))});
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-error')).not.toBeEmpty();
    await expect(page.getByTestId('import-start')).toBeVisible();
});

test('nothing scrolls sideways: the list, a program, the variables, the report', async ({page}) => {
    const overflow = () => page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    await page.goto('./');
    await expect(page.getByTestId('program-list')).toBeVisible();
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.getByTestId('program-vergleiche-alle').getByTestId('program-open').click();
    await expect(page.getByTestId('program-detail')).toBeVisible();
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.getByRole('tab', {name: 'System variables'}).click();
    await expect(page.getByTestId('variable-list')).toBeVisible();
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.getByRole('tab', {name: 'Import'}).click();
    await page.getByTestId('import-file').setInputFiles(fixture);
    await page.getByTestId('import-read-file').click();
    await expect(page.getByTestId('import-programs')).toBeVisible();
    expect(await overflow()).toBeLessThanOrEqual(0);
    await page.getByTestId('import-discard').click();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the program reads as on the CCU', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-search').fill('heizperiode');
        await page.getByTestId('program-heizperiode-schalten').getByTestId('program-open').click();
        const detail = page.getByTestId('program-detail');
        await expect(detail.getByTestId('rule-0')).toContainText('Bedingung: Wenn…');
        await expect(detail.getByTestId('rule-1')).toContainText('Sonst, wenn…');
        await expect(detail.getByTestId('rule-3')).toContainText('Sonst…');
        await expect(detail.getByTestId('rule-0')).toContainText('Aktivität: Dann…');
        await expect(detail.getByTestId('rule-0').getByTestId('line').first()).toContainText('kleiner als 5 °C');
        await expect(detail.getByTestId('rule-0').getByTestId('line').first()).toContainText('bei Änderung auslösen');
        await expect(detail.getByTestId('rule-1').getByTestId('line').first()).toContainText('im Wertebereich von 5 °C bis kleiner 15 °C');
        await expect(detail.getByTestId('rule-2').getByTestId('action').first()).toContainText('verzögert um 2 h');
        await expect(detail.getByTestId('rule-0').getByTestId('action').first()).toContainText('sofort');
        await expect(detail.getByTestId('rule-2').getByTestId('line').nth(1)).toContainText('nur prüfen');
    });
    test('the report is German, and so is its file', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Import'}).click();
        await page.getByTestId('import-file').setInputFiles(fixture);
        await page.getByTestId('import-read-file').click();
        await expect(page.getByTestId('import-summary')).toContainText('Übernommen würden: 18 Programme (davon 7 inaktiv) und 38 Systemvariablen; nicht übernommen: 2 Programme.');
        await expect(page.getByTestId('import-quirks')).toContainText('Das Handbuch zählt diese Eigenheiten in einfachen Worten auf.');
        await expect(page.getByTestId('help-quirks')).toHaveAttribute('href', /docs\/de\/#quirks$/);
        await expect(page.getByTestId('import-program').filter({hasText: 'Wetter holen'})).toContainText('wurde zur Aktivität „Befehl“');
        await expect(page.getByTestId('import-program').filter({hasText: 'zzL_Ia5'})).toContainText('enthält ein Script');
        await page.getByTestId('import-discard').click();
        await page.getByTestId('import-file').setInputFiles({name: 'devices.regadom', mimeType: 'application/octet-stream', buffer: withDevices()});
        await page.getByTestId('import-read-file').click();
        await expect(page.getByTestId('import-missing')).toHaveText('3 Kanäle, die Programme verwenden, gibt es auf diesem System nicht: diese Programme bleiben inaktiv.');
        const key = page.getByTestId('import-device').filter({hasText: 'NEQ0000001:1'});
        await expect(key.locator('.lgc-pill')).toHaveText('vorhanden');
        await expect(page.getByTestId('import-device').filter({hasText: 'CUX2801001'}).locator('.lgc-pill')).toHaveText('fehlt');
        await expect(page.getByTestId('import-devices').locator('summary')).toHaveText('Geräte, die in Programmen vorkommen (4)');
        await expect(page.getByTestId('import-device').filter({hasText: 'HmIP-RF/00111BE9AB9012:4'})).toContainText('Terrasse, Flur');
        await page.getByTestId('import-discard').click();
        await page.getByTestId('import-file').setInputFiles(fixture);
        await page.getByTestId('import-read-file').click();
        const [download] = await Promise.all([page.waitForEvent('download'), page.getByTestId('import-download').click()]);
        expect(readFileSync(await download.path(), 'utf8')).toContain('# Importbericht');
        await page.getByTestId('import-discard').click();
    });
});
