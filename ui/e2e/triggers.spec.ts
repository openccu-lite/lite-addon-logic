import {expect, test} from '@playwright/test';

// The triggers page end to end: what triggers what, from the source's side, with the last
// event of a device and of a variable - and the same list for one program on its view. Every
// project makes a program and a variable of its own name and removes them again.

const FAKE = 'http://127.0.0.1:18081/';
const H = {'X-CCU-Addon-Request': '1'};
const TEMP = 'HmIP-RF/000A1B2C3D4E04:1/ACTUAL_TEMPERATURE';

test('what triggers what: sources with their last event, the listening programs and their mode, filter, search, the program\'s own list', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Lüfter ${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Feucht ${p}`, type: 'bool', visible: true, labels: {true: 'feucht', false: 'trocken'}}, value: false}})).status()).toBe(201);
    expect(
        (
            await request.post('api/v1/programs', {
                headers: H,
                data: {program: {name, active: true, visible: true, operable: true, rules: [{when: {op: 'any', groups: [{op: 'all', lines: [
                    {device: {interface: 'HmIP-RF', address: '000A1B2C3D4E04:1', param: 'ACTUAL_TEMPERATURE'}, op: 'gt', value: 21, trigger: 'update'},
                    {variable: `feucht-${p}`, op: 'eq', value: true, trigger: 'check'},
                    {time: {kind: 'daily', at: '07:00'}, trigger: 'update'},
                ]}]}, then: []}]}},
            })
        ).status(),
    ).toBe(201);
    expect((await request.post(`api/v1/variables/feucht-${p}/value`, {headers: H, data: {value: true}})).ok()).toBeTruthy();

    await page.goto('./');
    await page.getByRole('tab', {name: 'Triggers'}).click();
    await expect(page.getByTestId('help-triggers-page')).toHaveAttribute('href', /docs\/en\/#triggers-page$/);
    await page.getByTestId('trigger-search').fill(name);
    const groups = page.getByTestId('trigger-list').locator('.lgc-trg');
    await expect(groups).toHaveCount(3);

    // the variable: its last write with the value in its own words; the program only checks it
    const variable = page.getByTestId(`trigger-v:feucht-${p}`);
    await expect(variable.getByTestId('trigger-kind')).toHaveText('variable');
    await expect(variable.getByTestId('trigger-title')).toHaveText(`Feucht ${p}`);
    await expect(variable.getByTestId('trigger-last')).toContainText('→ feucht');
    await expect(variable.getByTestId('trigger-listener')).toContainText('check only');
    await expect(variable.getByTestId('trigger-listener').locator('.lgc-line-mark')).toHaveText('');

    // the device by its name, with datapoint and address; the line triggers at every report
    const device = page.getByTestId(`trigger-d:${TEMP}`);
    await expect(device.getByTestId('trigger-title')).toContainText('Thermostat Bad');
    await expect(device.getByTestId('trigger-title')).toContainText('ACTUAL_TEMPERATURE · 000A1B2C3D4E04:1');
    const listener = device.getByTestId('trigger-listener').filter({hasText: name});
    await expect(listener).toContainText('trigger when updated');
    await expect(listener).toContainText('⚠ runs at every report');
    await expect(listener.locator('.lgc-line-mark')).toHaveText('⚡');
    // the device reports: its last event and value appear (the addon subscribes a moment after the program was made)
    await expect(async () => {
        await request.post(FAKE + 'dev/event', {data: {interface: 'HmIP-RF', address: '000A1B2C3D4E04:1', param: 'ACTUAL_TEMPERATURE', value: 21.4}});
        await expect(device.getByTestId('trigger-last')).toContainText('→ 21.4', {timeout: 6000});
    }).toPass({timeout: 25_000});

    // the time module in words
    await expect(page.getByTestId(`trigger-t:luefter-${p}:3`).getByTestId('trigger-title')).toHaveText('daily, at 07:00');

    // the filter by kind
    await page.getByTestId('trigger-filter').selectOption('device');
    await expect(groups).toHaveCount(1);
    await page.getByTestId('trigger-filter').selectOption('time');
    await expect(groups).toHaveCount(1);
    await page.getByTestId('trigger-filter').selectOption('');
    await page.getByTestId('trigger-search').fill('nothing like this');
    await expect(page.getByTestId('trigger-empty')).toHaveText('Nothing matches.');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // a click on the program opens it; its own view has the same list for this program only
    await page.getByTestId('trigger-search').fill(name);
    await variable.getByTestId('trigger-program').click();
    await expect(page.getByTestId('program-name')).toHaveText(name);
    const own = page.getByTestId('program-triggers');
    await expect(own.locator('.lgc-trg')).toHaveCount(3);
    await expect(own.getByTestId(`trigger-d:${TEMP}`).getByTestId('trigger-listener')).toHaveCount(1);
    // the search is kept when the tab is come back to
    await page.getByRole('tab', {name: 'Triggers'}).click();
    await expect(page.getByTestId('trigger-search')).toHaveValue(name);

    expect((await request.delete(`api/v1/programs/luefter-${p}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/feucht-${p}`, {headers: H})).status()).toBe(200);
});
