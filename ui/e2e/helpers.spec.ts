import {expect, test} from '@playwright/test';

// Device helpers end to end, against the daemon on the fake system: a helper added in its dialog
// for the fake system's measuring actuator, counting when the device reports, its total set to a
// meter reading, its day values read-only on the page and through the API, switched off and on,
// and removed with all its variables. The projects share the device, so each removes its helper.
test.describe.configure({mode: 'serial'});

const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const PSM = '000A1B2C3D4E12:1';

test('a device helper: added, counting, set, switched off, removed', async ({page, request}, info) => {
    const p = info.project.name;
    const report = async (value: number) => expect((await request.post(FAKE + 'dev/event', {data: {interface: 'HmIP-RF', address: PSM, param: 'ENERGY_COUNTER', value}})).ok()).toBeTruthy();
    const value = async (key: string) => (await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === key)?.value;
    const noScroll = async () => expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await expect(page.getByTestId('help-helpers')).toHaveAttribute('href', /docs\/en\/#helpers$/);
    await page.getByTestId('helper-new').click();
    const dialog = page.getByTestId('helper-dialog');
    // the channels of the system that have a counter: the actuator's energy, the weather sensor's rain and sunshine
    const device = dialog.getByTestId('helper-device');
    await expect(device.locator('option')).toHaveText(['Waschmaschine Messung – Energy (HmIP-PSM)', 'Wetterstation – Rain (HmIP-SWO-PR)', 'Wetterstation – Sunshine (HmIP-SWO-PR)']);
    await expect(dialog.getByTestId('helper-state')).toHaveText('');
    await expect(dialog.getByTestId('help-helpers')).toHaveAttribute('href', /docs\/en\/#helpers$/);
    // the name is the channel's; the variables' names follow it
    const name = dialog.getByTestId('helper-name');
    await expect(name).toHaveValue('Waschmaschine Messung');
    const names = dialog.getByTestId('helper-names').locator('li');
    await expect(names).toHaveText(['total: Energy counter Waschmaschine Messung (Wh)', 'today: Energy today Waschmaschine Messung (Wh)', 'yesterday: Energy yesterday Waschmaschine Messung (Wh)']);
    // another device, another name
    await device.selectOption({label: 'Wetterstation – Rain (HmIP-SWO-PR)'});
    await expect(name).toHaveValue('Wetterstation');
    await expect(names.nth(0)).toHaveText('total: Rainfall Wetterstation (mm)');
    await device.selectOption({label: 'Waschmaschine Messung – Energy (HmIP-PSM)'});
    // without a name it is not made; the list of names keeps its height without the day values
    const height = (await dialog.getByTestId('helper-names').boundingBox())!.height;
    await name.fill('');
    await dialog.getByTestId('helper-save').click();
    await expect(dialog.getByTestId('helper-name-error')).toHaveText('The helper needs a name.');
    await name.fill(`Waschmaschine ${p}`);
    await expect(dialog.getByTestId('helper-name-error')).toHaveText('');
    await dialog.getByTestId('helper-days').uncheck();
    await expect(names).toHaveText([`total: Energy counter Waschmaschine ${p} (Wh)`, '', '']);
    expect((await dialog.getByTestId('helper-names').boundingBox())!.height).toBe(height);
    await dialog.getByTestId('helper-days').check();
    await noScroll();
    await dialog.getByTestId('helper-save').click();
    await expect(dialog).toHaveCount(0);

    const total = `energy-counter-waschmaschine-${p}`;
    const today = `energy-today-waschmaschine-${p}`;
    const yesterday = `energy-yesterday-waschmaschine-${p}`;
    const row = page.getByTestId(`variable-${total}`);
    await expect(row.getByTestId('flag-helper')).toBeVisible();
    await expect(row.getByTestId('variable-type')).toHaveText('Number');
    // the day values are shown, not entered; the total takes a value
    for (const key of [today, yesterday]) {
        await expect(page.getByTestId(`variable-${key}`).getByTestId('flag-helper')).toBeVisible();
        await expect(page.getByTestId(`variable-${key}`).getByTestId('value-set')).toHaveCount(0);
        expect(await page.getByTestId(`variable-${key}`).getByTestId('value').evaluate((el) => el.tagName)).toBe('SPAN');
    }
    expect((await request.post(`api/v1/variables/${today}/value`, {headers: H, data: {value: 5}})).status()).toBe(422);
    // the device is no candidate any more
    await page.getByTestId('helper-new').click();
    await expect(dialog.getByTestId('helper-device').locator('option')).toHaveText(['Wetterstation – Rain (HmIP-SWO-PR)', 'Wetterstation – Sunshine (HmIP-SWO-PR)']);
    await dialog.getByTestId('helper-cancel').click();
    await expect(dialog).toHaveCount(0);

    // the helper counts from the device's next report: the first only says where the counter stands
    await report(5000);
    await report(5001.5);
    await expect.poll(() => value(total)).toBe(1.5);
    await expect.poll(() => value(today)).toBe(1.5);
    await expect(page.getByTestId(`variable-${today}`).getByTestId('value')).toHaveText('1.5 Wh');
    // the meter reading: the total set on the page, counting goes on from it
    await row.getByTestId('value').fill('20000');
    await row.getByTestId('value-set').click();
    await expect.poll(() => value(total)).toBe(20000);
    await report(5002);
    await expect.poll(() => value(total)).toBe(20000.5);
    await expect.poll(() => value(today)).toBe(2);

    // switched off in the total's dialog: nothing is added; on again, it counts on from where the device stands
    await row.getByTestId('variable-edit').click();
    const edit = page.getByTestId('variable-dialog');
    await expect(edit.getByTestId('var-type')).toBeDisabled();
    await edit.getByTestId('var-helper-off').check();
    await edit.getByTestId('var-save').click();
    await expect(edit).toHaveCount(0);
    await expect(row.getByTestId('flag-helper-off')).toBeVisible();
    await report(5010);
    await page.getByTestId(`variable-${today}`).getByTestId('variable-edit').click();
    await expect(edit.getByTestId('var-helper-off')).toHaveCount(0); // said on the total only
    await edit.getByRole('button', {name: 'Cancel'}).click();
    expect(await value(total)).toBe(20000.5);
    await row.getByTestId('variable-edit').click();
    await edit.getByTestId('var-helper-off').uncheck();
    await edit.getByTestId('var-save').click();
    await expect(edit).toHaveCount(0);
    await expect(row.getByTestId('flag-helper-off')).toHaveCount(0);
    await report(5011);
    await expect.poll(() => value(total)).toBe(20001.5);
    await noScroll();

    // removed with all its variables, from any of them
    await page.getByTestId(`variable-${yesterday}`).getByTestId('variable-delete').click();
    const confirm = page.getByTestId('confirm-delete');
    await expect(confirm).toContainText('Delete the device helper?');
    await expect(confirm).toContainText(`Energy counter Waschmaschine ${p}`);
    await expect(confirm).toContainText(`Energy today Waschmaschine ${p}`);
    await page.getByTestId('confirm-delete-yes').click();
    for (const key of [total, today, yesterday]) await expect(page.getByTestId(`variable-${key}`)).toHaveCount(0);
    expect((await (await request.get('api/v1/helpers')).json()).helpers).toHaveLength(0);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the dialog, and a rain helper without day values', async ({page, request}, info) => {
        const p = info.project.name;
        await page.goto('./');
        await page.getByRole('tab', {name: 'Systemvariablen'}).click();
        await page.getByTestId('helper-new').click();
        const dialog = page.getByTestId('helper-dialog');
        await dialog.getByTestId('helper-device').selectOption({label: 'Wetterstation – Regen (HmIP-SWO-PR)'});
        await dialog.getByTestId('helper-name').fill(`Garten ${p}`);
        await expect(dialog.getByTestId('helper-names').locator('li')).toHaveText([`Gesamtstand: Regenmenge Garten ${p} (mm)`, `heute: Regen heute Garten ${p} (mm)`, `gestern: Regen gestern Garten ${p} (mm)`]);
        await dialog.getByTestId('helper-days').uncheck();
        await dialog.getByTestId('helper-save').click();
        await expect(dialog).toHaveCount(0);
        const key = `regenmenge-garten-${p}`;
        await expect(page.getByTestId(`variable-${key}`).getByTestId('flag-helper')).toHaveText('Helfer');
        await expect(page.getByTestId(`variable-regen-heute-garten-${p}`)).toHaveCount(0);
        expect((await request.delete(`api/v1/helpers/${key}`, {headers: H})).ok()).toBeTruthy();
    });
});
