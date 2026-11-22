import {expect, test} from '@playwright/test';

// The start of the system, the loop guard and the pause between write commands, on the third
// daemon (playwright.config.ts): its tree (e2e/table-seed) has a program with "evaluate at
// system start" on, and the daemon's first start in its fresh data directory is the first one
// after this boot - so the program was evaluated once before the tests look.

const BASE = 'http://127.0.0.1:18303/addons/logic/';
test.use({baseURL: BASE});

test('a program with "evaluate at system start" ran once after the start, and the switch is on its view', async ({page, request}) => {
    await page.goto('./');
    await page.getByTestId('program-start').getByTestId('program-open').click();
    await expect(page.getByTestId('detail-last-run')).toContainText('· Then · system start');
    // only once: the other programs have the switch off and did not run
    const overview = await (await request.get(BASE + 'api/v1/overview')).json();
    const ran = overview.programs.filter((p: {state: {last_run?: {trigger: string}}}) => p.state.last_run?.trigger === 'start').map((p: {program: {key: string}}) => p.program.key);
    expect(ran).toEqual(['start']);

    // the switch: off and on again, kept in the program's file
    const sw = page.getByTestId('detail-at-start');
    await expect(sw).toBeChecked();
    await sw.uncheck();
    await expect.poll(async () => (await (await request.get(BASE + 'api/v1/overview')).json()).programs.find((p: {program: {key: string}}) => p.program.key === 'start').program.at_start ?? false).toBe(false);
    await expect(sw).not.toBeChecked();
    await sw.check();
    await expect.poll(async () => (await (await request.get(BASE + 'api/v1/overview')).json()).programs.find((p: {program: {key: string}}) => p.program.key === 'start').program.at_start).toBe(true);

    // the ?-icon beside it opens the guide's section
    await expect(page.getByTestId('help-start')).toHaveAttribute('href', /docs\/en\/#start$/);
    const guide = await request.get(BASE + 'docs/en/');
    expect(await guide.text()).toContain('id="start"');
});

test('the settings: the pause between write commands and the loop guard', async ({page, request}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'Settings'}).click();
    const box = page.getByTestId('settings-runtime');
    await expect(box).toBeVisible();
    // what is not set shows its default
    await expect(page.getByTestId('pacing')).toHaveValue('300');
    await expect(page.getByTestId('loop-runs')).toHaveValue('20');
    await expect(page.getByTestId('loop-window')).toHaveValue('10');
    await expect(page.getByTestId('loop-pause')).toHaveValue('60');
    await expect(page.getByTestId('reset-runtime')).toBeDisabled();

    // a figure outside its limits is refused, and the message takes the line kept for it
    const height = (await box.boundingBox())!.height;
    await page.getByTestId('pacing').fill('50');
    await page.getByTestId('save-runtime').click();
    await expect(page.getByTestId('runtime-message')).toHaveText('Pause between write commands: a whole number from 100 to 10000.');
    expect((await box.boundingBox())!.height).toBe(height);

    await page.getByTestId('pacing').fill('500');
    await page.getByTestId('loop-runs').fill('5');
    await page.getByTestId('save-runtime').click();
    await expect(page.getByTestId('runtime-message')).toHaveText('The settings were saved.');
    expect((await box.boundingBox())!.height).toBe(height);
    const stored = await (await request.get(BASE + 'api/v1/settings')).json();
    expect(stored).toMatchObject({pacing_ms: 500, loop_guard: {runs: 5}});
    // they are there after a reload, and the location's form leaves them alone
    await page.reload();
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(page.getByTestId('pacing')).toHaveValue('500');
    await page.getByTestId('latitude').fill('52.52');
    await page.getByTestId('longitude').fill('13.405');
    await page.getByTestId('save-location').click();
    await expect(page.getByTestId('settings-message')).toHaveText('The location was saved.');
    expect(await (await request.get(BASE + 'api/v1/settings')).json()).toMatchObject({pacing_ms: 500, loop_guard: {runs: 5}, location: {latitude: 52.52}});
    await page.getByTestId('clear-location').click();
    await expect(page.getByTestId('settings-message')).toHaveText('The location was removed.');

    // back to the defaults: nothing is stored any more
    await page.getByTestId('reset-runtime').click();
    await expect(page.getByTestId('runtime-message')).toHaveText('The settings were saved.');
    await expect(page.getByTestId('pacing')).toHaveValue('300');
    await expect(page.getByTestId('loop-runs')).toHaveValue('20');
    expect(await (await request.get(BASE + 'api/v1/settings')).json()).toEqual({});

    // the ?-icons
    await expect(page.getByTestId('help-pacing')).toHaveAttribute('href', /#pacing$/);
    await expect(page.getByTestId('help-loop-guard')).toHaveAttribute('href', /#loop-guard$/);
    // nothing scrolls sideways
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the run after the start and the settings speak German', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-start').getByTestId('program-open').click();
        await expect(page.getByTestId('detail-last-run')).toContainText('· Dann · Systemstart');
        await expect(page.getByText('beim Systemstart prüfen')).toBeVisible();
        await page.getByTestId('program-back').click();
        await page.getByRole('tab', {name: 'Einstellungen'}).click();
        await expect(page.getByRole('heading', {name: 'Schleifenschutz'})).toBeVisible();
        await expect(page.getByRole('heading', {name: 'Schreibbefehle'})).toBeVisible();
    });
});
