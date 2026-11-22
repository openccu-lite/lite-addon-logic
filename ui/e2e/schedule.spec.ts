import {expect, test, type APIRequestContext} from '@playwright/test';

// The schedule page end to end, on the daemon whose clock stands still until the tests move it
// (Europe/Berlin, the programs of e2e/time-seed): the time modules with their next and last
// firing, a time on the other side of a clock change with its offset, and delayed activities
// carried out now or cancelled.

const BASE = 'http://127.0.0.1:18302/addons/logic/';
const FAKE = 'http://127.0.0.1:18082/';
const H = {'X-CCU-Addon-Request': '1'};
test.use({baseURL: BASE, timezoneId: 'Europe/Berlin'});
test.describe.configure({mode: 'serial'});

async function clock(request: APIRequestContext, body?: object): Promise<Date> {
    const res = body ? await request.post(BASE + 'api/v1/dev/clock', {data: body, headers: H}) : await request.get(BASE + 'api/v1/dev/clock');
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

test('the time modules: next and last firing, a range under way, the offset across a clock change, filter and search', async ({page, request}) => {
    await setClock(request, '2026-11-21T21:50:00+01:00');
    await page.goto('./');
    await page.getByRole('tab', {name: 'Schedule'}).click();
    await expect(page.getByTestId('help-schedule')).toHaveAttribute('href', /docs\/en\/#schedule$/);
    await expect(page.getByTestId('help-schedule-delays')).toHaveAttribute('href', /#schedule-delays$/);
    const night = page.getByTestId('schedule-t-nacht-1');
    await expect(night).toContainText('daily, from 22:00, for 8 h');
    await expect(night.getByTestId('schedule-program')).toHaveText('Nachtabsenkung');
    await expect(night.getByTestId('schedule-next')).toHaveText('21/11/2026, 22:00 (begin)');
    await expect(night.getByTestId('schedule-relative')).toHaveText('in 9 min');
    await expect(night.getByTestId('schedule-kind')).toHaveText('time module');
    // what fires next comes first
    const order = await page.getByTestId('schedule-list').locator('tbody tr').evaluateAll((trs) => trs.map((tr) => tr.getAttribute('data-testid')));
    expect(order.indexOf('schedule-t-nacht-1')).toBeLessThan(order.indexOf('schedule-t-taeglich-1'));

    await clock(request, {advance: '15min'});
    await expect(night.getByTestId('schedule-kind')).toHaveText('true now');
    await expect(night.getByTestId('schedule-last')).toHaveText('21/11/2026, 22:00 (begin)');
    await expect(night.getByTestId('schedule-next')).toHaveText('22/11/2026, 06:00 (end)');
    await expect(night.getByTestId('schedule-relative')).toHaveText('in 7 h 54 min');

    // every row has the same height, and nothing scrolls sideways
    const heights = await page.getByTestId('schedule-list').locator('tbody tr').evaluateAll((trs) => trs.map((tr) => Math.round(tr.getBoundingClientRect().height * 100) / 100));
    expect(heights.length).toBeGreaterThan(3);
    expect(new Set(heights).size, `row heights ${heights.join(', ')}`).toBe(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // the evening before the clock goes forward: 02:30 does not exist that night - it is listed
    // at the moment it happens, 03:00, with the offset the clock has then
    await setClock(request, '2026-03-28T20:00:00+01:00');
    const halfPastTwo = page.getByTestId('schedule-t-halb-drei-1');
    await expect(halfPastTwo.getByTestId('schedule-next')).toHaveText('29/03/2026, 03:00 UTC+2');
    await expect(halfPastTwo.getByTestId('schedule-relative')).toHaveText('in 5 h 59 min');
    // 06:30 the next morning is summer time as well; 22:00 tonight is not
    await expect(page.getByTestId('schedule-t-taeglich-1').getByTestId('schedule-next')).toHaveText('29/03/2026, 06:30 UTC+2');
    await expect(night.getByTestId('schedule-next')).toHaveText('28/03/2026, 22:00 (begin)');

    // the search and the filter
    await page.getByTestId('schedule-search').fill('nachtabsenkung');
    await expect(page.getByTestId('schedule-list').locator('tbody tr')).toHaveCount(1);
    await page.getByTestId('schedule-search').fill('');
    await page.getByTestId('schedule-filter').selectOption('delay');
    await expect(page.getByTestId('schedule-empty')).toHaveText('Nothing matches.');
    await page.getByTestId('schedule-filter').selectOption('time');
    // a click on the program opens it
    await night.getByTestId('schedule-program').click();
    await expect(page.getByTestId('program-name')).toHaveText('Nachtabsenkung');
});

test('delayed activities: listed until they are due, carried out now, cancelled', async ({page, request}, info) => {
    const p = info.project.name;
    await setClock(request, '2026-11-21T12:00:00+01:00');
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Zahl ${p}`, type: 'number', visible: true}, value: 0}})).status()).toBe(201);
    const address = `VERZ${p.replace(/[^a-z]/g, '').toUpperCase()}:1`;
    expect(
        (
            await request.post('api/v1/programs', {
                headers: H,
                data: {program: {name: `Verz ${p}`, active: false, visible: true, operable: true, rules: [{when: {op: 'any', groups: []}, then: [
                    {kind: 'variable', variable: `zahl-${p}`, to: 5, after: '10min'},
                    {kind: 'set', device: {interface: 'HmIP-RF', address, param: 'LEVEL'}, to: 1, after: '20min'},
                ]}]}},
            })
        ).status(),
    ).toBe(201);
    expect((await request.post(`api/v1/programs/verz-${p}/run`, {headers: H, data: {mode: 'then'}})).ok()).toBeTruthy();

    await page.goto('./');
    await page.getByRole('tab', {name: 'Schedule'}).click();
    await page.getByTestId('schedule-filter').selectOption('delay');
    const rows = page.getByTestId('schedule-list').locator('tbody tr');
    await expect(rows).toHaveCount(2);
    const first = rows.nth(0);
    await expect(first.getByTestId('schedule-kind')).toHaveText('delay');
    await expect(first).toContainText('delay of 10 min');
    await expect(first.getByTestId('schedule-program')).toHaveText(`Verz ${p} → Zahl ${p}`);
    await expect(first.getByTestId('schedule-next')).toHaveText('21/11/2026, 12:10:21');
    await expect(first.getByTestId('schedule-relative')).toHaveText('in 10 min');
    await expect(rows.nth(1)).toContainText('delay of 20 min');
    await expect(rows.nth(1).getByTestId('schedule-program')).toContainText('LEVEL');
    // half of the first one's way
    await clock(request, {advance: '5min'});
    await expect(first.getByTestId('schedule-relative')).toHaveText('in 5 min');

    // carried out now: done once, and gone
    await first.getByTestId('pending-run').click();
    await expect(page.getByText('The delayed activity was carried out.')).toBeVisible();
    await expect(rows).toHaveCount(1);
    const overview = await (await request.get('api/v1/overview')).json();
    expect(overview.variables.find((v: {def: {key: string}}) => v.def.key === `zahl-${p}`).value).toBe(5);
    // cancelled: it never happens
    await rows.nth(0).getByTestId('pending-cancel').click();
    await expect(page.getByText('The delayed activity was cancelled.')).toBeVisible();
    await expect(page.getByTestId('schedule-empty')).toBeVisible();
    await clock(request, {advance: '30min'});
    const writes: {address: string}[] = (await (await request.get(FAKE + 'dev/writes')).json()) ?? [];
    expect(writes.filter((w) => w.address === address)).toEqual([]);

    expect((await request.delete(`api/v1/programs/verz-${p}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/zahl-${p}`, {headers: H})).status()).toBe(200);
});
