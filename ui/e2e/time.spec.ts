import {expect, test, type APIRequestContext, type Page} from '@playwright/test';

// The time modules end to end: the daemon in Europe/Berlin on a clock that stands still until
// these tests move it (scripts/dev-serve.sh with LOGIC_DEV_CLOCK, the second server of
// playwright.config.ts), with the programs of e2e/time-seed. Each test sets the clock to its
// own moment first - the way a correction of the system's clock would - so the tests do not
// depend on each other, nor on how often they ran before.

const BASE = 'http://127.0.0.1:18302/addons/logic/';
test.use({baseURL: BASE, timezoneId: 'Europe/Berlin'});
test.describe.configure({mode: 'serial'});

async function clock(request: APIRequestContext, body?: object): Promise<Date> {
    const res = body ? await request.post(BASE + 'api/v1/dev/clock', {data: body, headers: {'X-CCU-Addon-Request': '1'}}) : await request.get(BASE + 'api/v1/dev/clock');
    expect(res.ok(), await res.text()).toBeTruthy();
    return new Date((await res.json()).now);
}

/** Sets the clock to a moment and lets the scheduler look at it once. */
async function setClock(request: APIRequestContext, iso: string) {
    const now = await clock(request);
    const jump = Math.round((new Date(iso).getTime() - now.getTime()) / 1000);
    if (jump !== 0) await clock(request, {jump_seconds: jump});
    await clock(request, {advance: '21s'});
}

const advance = (request: APIRequestContext, d: string) => clock(request, {advance: d});

async function open(page: Page, key: string) {
    await page.goto('./');
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await expect(page.getByTestId('program-detail')).toBeVisible();
}

test('a daily point: the next time is shown, it runs at its instant, and the next is tomorrow', async ({page, request}) => {
    await setClock(request, '2026-11-21T06:00:00+01:00');
    await page.goto('./');
    const row = page.getByTestId('program-taeglich');
    await expect(row.getByTestId('program-next')).toHaveText('21/11/2026, 06:30');
    await open(page, 'taeglich');
    await expect(page.getByTestId('detail-next-time')).toHaveText('21/11/2026, 06:30');
    await expect(page.getByTestId('time-next')).toContainText('21/11/2026, 06:30 · 22/11/2026, 06:30 · 23/11/2026, 06:30');
    await expect(page.getByTestId('time-active')).toHaveCount(0);

    await advance(request, '45min');
    await expect(page.getByTestId('detail-last-run')).toContainText('21/11/2026, 06:30:00 · Then · time module');
    await expect(page.getByTestId('time-last')).toHaveText('last: 21/11/2026, 06:30');
    await expect(page.getByTestId('detail-next-time')).toHaveText('22/11/2026, 06:30');
    // a point is true at its instant only
    await expect(page.getByTestId('time-active')).toHaveCount(0);
});

test('a range: Then at its begin, true in between, Else at its end', async ({page, request}) => {
    await setClock(request, '2026-11-21T21:50:00+01:00');
    await open(page, 'nacht');
    await expect(page.getByTestId('time-next')).toContainText('21/11/2026, 22:00 (begin) · 22/11/2026, 06:00 (end)');
    await advance(request, '15min');
    await expect(page.getByTestId('detail-last-run')).toContainText('21/11/2026, 22:00:00 · Then · time module');
    await expect(page.getByTestId('time-active')).toBeVisible();
    await expect(page.getByTestId('time-last')).toHaveText('last: 21/11/2026, 22:00 (begin)');
    await expect(page.getByTestId('detail-next-time')).toHaveText('22/11/2026, 06:00');

    await advance(request, '8h');
    await expect(page.getByTestId('detail-last-run')).toContainText('22/11/2026, 06:00:00 · Else · time module');
    await expect(page.getByTestId('time-active')).toHaveCount(0);
    await expect(page.getByTestId('time-last')).toHaveText('last: 22/11/2026, 06:00 (end)');
});

test('a time module on "check only" triggers all the same, as on the CCU: the page says so and it runs', async ({page, request}) => {
    await setClock(request, '2026-11-21T07:00:00+01:00');
    await page.goto('./');
    const row = page.getByTestId('program-pruefen');
    await expect(row.getByTestId('program-next')).toHaveText('21/11/2026, 07:15');
    await expect(row).toContainText('Time control');
    await open(page, 'pruefen');
    await expect(page.getByTestId('line').first()).toContainText('check only');
    await expect(page.getByTestId('line-mark').first()).toHaveText('⚡');
    await expect(page.getByTestId('line-mark').first()).toHaveAttribute('title', /whatever its mode says/);
    await advance(request, '20min');
    await expect(page.getByTestId('detail-last-run')).toContainText('21/11/2026, 07:15:00 · Then · time module');
});

test('an interval: every quarter of an hour on the clock', async ({page, request}) => {
    await setClock(request, '2026-11-21T10:01:00+01:00');
    await open(page, 'intervall');
    await expect(page.getByTestId('time-next')).toContainText('21/11/2026, 10:15 · 21/11/2026, 10:30 · 21/11/2026, 10:45');
    await advance(request, '32min');
    await expect(page.getByTestId('detail-last-run')).toContainText('21/11/2026, 10:30:00 · Then · time module');
    await expect(page.getByTestId('detail-next-time')).toHaveText('21/11/2026, 10:45');
    // once it has fired, an all-day interval is true all day (as on the CCU)
    await expect(page.getByTestId('time-active')).toBeVisible();
    // over midnight the count begins again
    await setClock(request, '2026-11-21T23:50:00+01:00');
    await expect(page.getByTestId('time-next')).toContainText('22/11/2026, 00:00 · 22/11/2026, 00:15');
});

test('an astro time needs the location: set on the settings tab, then sunset triggers', async ({page, request}) => {
    await request.put(BASE + 'api/v1/settings', {data: {}, headers: {'X-CCU-Addon-Request': '1'}});
    await setClock(request, '2026-06-15T12:00:00+02:00');
    await page.goto('./');
    const row = page.getByTestId('program-astro');
    await expect(row.getByTestId('flag-no-location')).toBeVisible();
    await expect(row.getByTestId('program-next')).toHaveText('–');
    await row.getByTestId('program-open').click();
    await expect(page.getByTestId('no-location')).toBeVisible();
    await expect(page.getByTestId('time-next')).toHaveText('next: –');

    await page.getByTestId('go-settings').click();
    await expect(page.getByTestId('settings')).toBeVisible();
    await expect(page.getByTestId('settings-message')).toHaveText('No location is set.');
    await expect(page.getByTestId('system-time')).toContainText('15/06/2026, 12:00');
    // the message line is there before and after: nothing jumps
    const before = await page.getByTestId('settings').boundingBox();
    await page.getByTestId('latitude').fill('north');
    await page.getByTestId('longitude').fill('13,405');
    await page.getByTestId('save-location').click();
    await expect(page.getByTestId('settings-message')).toContainText('are numbers');
    await page.getByTestId('latitude').fill('95');
    await page.getByTestId('save-location').click();
    await expect(page.getByTestId('settings-message')).toContainText('between -90 and 90');
    await page.getByTestId('latitude').fill('52,52');
    await page.getByTestId('save-location').click();
    await expect(page.getByTestId('settings-message')).toHaveText('The location was saved.');
    expect((await page.getByTestId('settings').boundingBox())?.height).toBe(before?.height);
    await expect(page.getByTestId('latitude')).toHaveValue('52.52');

    await page.getByRole('tab', {name: 'Programs'}).click();
    await expect(row.getByTestId('flag-no-location')).toHaveCount(0);
    await row.getByTestId('program-open').click();
    await expect(page.getByTestId('no-location')).toHaveCount(0);
    // sunset in Berlin on 15 June is at 21:31
    await expect(page.getByTestId('detail-next-time')).toHaveText(/^15\/06\/2026, 21:3[01]:\d\d$/);
    await advance(request, '10h');
    await expect(page.getByTestId('detail-last-run')).toContainText(/15\/06\/2026, 21:3[01]:\d\d · Then · time module/);
    await expect(page.getByTestId('time-active')).toBeVisible();
    await expect(page.getByTestId('time-next')).toContainText(/16\/06\/2026, 04:4[23]:\d\d \(end\)/);

    // and the location can be removed again: the program says so at once
    await page.getByRole('tab', {name: 'Settings'}).click();
    await page.getByTestId('clear-location').click();
    await expect(page.getByTestId('settings-message')).toHaveText('The location was removed.');
    await expect(page.getByTestId('clear-location')).toBeDisabled();
    await page.getByRole('tab', {name: 'Programs'}).click();
    await expect(row.getByTestId('flag-no-location')).toBeVisible();
    await request.put(BASE + 'api/v1/settings', {data: {location: {latitude: 52.52, longitude: 13.405}}, headers: {'X-CCU-Addon-Request': '1'}});
});

test('the day the clock goes forward: 02:30 does not exist and runs at 03:00', async ({page, request}) => {
    await setClock(request, '2026-03-29T01:50:00+01:00');
    await open(page, 'halb-drei');
    await expect(page.getByTestId('detail-next-time')).toHaveText('29/03/2026, 03:00');
    await expect(page.getByTestId('time-next')).toContainText('29/03/2026, 03:00 · 30/03/2026, 02:30');
    await advance(request, '15min');
    await expect(page.getByTestId('detail-last-run')).toContainText('29/03/2026, 03:00:00 · Then · time module');
    await expect(page.getByTestId('detail-next-time')).toHaveText('30/03/2026, 02:30');
    // the night's range ends at 06:00 on the clock, seven hours after it began
    await setClock(request, '2026-03-28T21:59:00+01:00');
    await open(page, 'nacht');
    await advance(request, '2min');
    await expect(page.getByTestId('time-next')).toContainText('29/03/2026, 06:00 (end)');
    await advance(request, '7h');
    await expect(page.getByTestId('detail-last-run')).toContainText('29/03/2026, 06:00:00 · Else · time module');
});

test('the day the clock goes back: 02:30 happens twice and runs once', async ({page, request}) => {
    await setClock(request, '2026-10-25T01:50:00+02:00');
    await open(page, 'halb-drei');
    await expect(page.getByTestId('detail-next-time')).toHaveText('25/10/2026, 02:30');
    await advance(request, '45min'); // 02:35, the first time
    await expect(page.getByTestId('detail-last-run')).toContainText('25/10/2026, 02:30:00 · Then · time module');
    await expect(page.getByTestId('detail-next-time')).toHaveText('26/10/2026, 02:30');
    await advance(request, '1h'); // 02:35 again
    await expect(page.getByTestId('detail-next-time')).toHaveText('26/10/2026, 02:30');
    await expect(page.getByTestId('time-last')).toHaveText('last: 25/10/2026, 02:30');
});

test('the ?-icons of the time modules open their sections of the guide', async ({page, context}) => {
    await open(page, 'taeglich');
    for (const [id, anchor] of [
        ['help-time-next', '#time-next'],
        ['help-time', '#time'],
    ]) {
        const [guide] = await Promise.all([context.waitForEvent('page'), page.getByTestId(id!).first().click()]);
        await guide.waitForLoadState();
        expect(guide.url()).toContain('/addons/logic/docs/en/' + anchor);
        await expect(guide.locator(anchor!)).toBeVisible();
        await guide.close();
    }
    await page.getByRole('tab', {name: 'Settings'}).click();
    const [guide] = await Promise.all([context.waitForEvent('page'), page.getByTestId('help-time-location').click()]);
    await guide.waitForLoadState();
    await expect(guide.locator('#time-location')).toHaveText('The location');
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the times and the words are German', async ({page, request}) => {
        await setClock(request, '2026-11-21T21:50:00+01:00');
        await open(page, 'nacht');
        await expect(page.getByTestId('time-next')).toContainText('nächste: 21.11.26, 22:00 (Beginn) · 22.11.26, 06:00 (Ende)');
        await page.getByRole('tab', {name: 'Einstellungen'}).click();
        await expect(page.getByTestId('settings')).toContainText('Breitengrad');
    });
});

test('nothing scrolls sideways with the time lines and the settings', async ({page, request}) => {
    await setClock(request, '2026-11-21T21:50:00+01:00');
    await open(page, 'nacht');
    await expect(page.getByTestId('time-next')).toContainText('(end)');
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(page.getByTestId('settings')).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
});
