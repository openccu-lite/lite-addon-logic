import {expect, test} from '@playwright/test';

// Computed values end to end, against the daemon on the fake system: a dew point made in the
// variable dialog from two datapoints chosen in the device picker, calculated when the device
// reports, read-only on the page and through the API; a difference computed from it that
// triggers a program; the function changed. Every project has variables of its own name; the
// device is the fake system's wall thermostat, which every project makes report itself.
test.describe.configure({mode: 'serial'});

const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const THERMOSTAT = '000A1B2C3D4E04:1';

test('a dew point from a device, a difference from it, a program on the difference', async ({page, request}, info) => {
    const p = info.project.name;
    const report = async (param: string, value: number) => expect((await request.post(FAKE + 'dev/event', {data: {interface: 'HmIP-RF', address: THERMOSTAT, param, value}})).ok()).toBeTruthy();
    const value = async (key: string) => (await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === key)?.value;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Grenze ${p}`, type: 'number', visible: true, unit: '°C'}, value: 4.3}})).status()).toBe(201);

    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-new').click();
    const dialog = page.getByTestId('variable-dialog');
    await dialog.getByTestId('var-name').fill(`Taupunkt ${p}`);
    await dialog.getByTestId('var-type').selectOption('computed');
    await expect(dialog.getByTestId('help-computed')).toHaveAttribute('href', /docs\/en\/#computed$/);
    await expect(dialog.getByTestId('var-function')).toHaveValue('dew-point');
    const inputs = dialog.getByTestId('var-input');
    await expect(inputs).toHaveCount(2);
    await expect(dialog.getByTestId('var-inputs')).toContainText('Temperature (°C)');
    await expect(dialog.getByTestId('var-inputs')).toContainText('Relative humidity (%)');
    // a dew point takes exactly two inputs: none can be added or removed
    await expect(dialog.getByTestId('input-add')).toBeDisabled();
    await expect(inputs.nth(0).getByTestId('input-remove')).toBeDisabled();
    // without inputs it is not saved
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-inputs-error')).toHaveText('Every input needs a datapoint or a variable.');
    // both from the thermostat's channel: the picker, then the datapoint the input asks for is chosen
    const picker = page.getByTestId('device-picker');
    for (const [i, param] of [[0, 'ACTUAL_TEMPERATURE'], [1, 'HUMIDITY']] as const) {
        await inputs.nth(i).getByTestId('input-device').click();
        await picker.getByTestId(`pick-HmIP-RF/${THERMOSTAT}`).getByTestId('pick').click();
        await expect(picker).toHaveCount(0);
        await expect(inputs.nth(i).getByTestId('input-param')).toHaveValue(param);
    }
    await dialog.getByTestId('var-unit').fill('°C');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);

    const dew = `taupunkt-${p}`;
    const row = page.getByTestId(`variable-${dew}`);
    await expect(row.getByTestId('variable-type')).toHaveText('Computed');
    await expect(row.getByTestId('flag-computed')).toBeVisible();
    // its value is shown, not entered
    await expect(row.getByTestId('value-set')).toHaveCount(0);
    expect(await row.getByTestId('value').evaluate((el) => el.tagName)).toBe('SPAN');
    // the device reports: 20 °C at 50 % is a dew point of 9.3 °C
    await report('ACTUAL_TEMPERATURE', 20);
    await report('HUMIDITY', 50);
    await expect(row.getByTestId('value')).toHaveText('9.3 °C');
    await expect(row.getByTestId('value')).toHaveAttribute('title', /^Dew point of .+ ACTUAL_TEMPERATURE, .+ HUMIDITY: 9\.3 °C$/);

    // nobody writes it, and its file says what it is
    const refused = await request.post(`api/v1/variables/${dew}/value`, {headers: H, data: {value: 1}});
    expect(refused.status()).toBe(422);
    expect((await refused.json()).error).toBe('read-only');
    const def = (await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === dew).def;
    expect(def).toMatchObject({type: 'number', unit: '°C', computed: {function: 'dew-point', inputs: [
        {device: {interface: 'HmIP-RF', address: THERMOSTAT, param: 'ACTUAL_TEMPERATURE'}}, {device: {interface: 'HmIP-RF', address: THERMOSTAT, param: 'HUMIDITY'}}]}});

    // a difference of the dew point and a variable, made in the dialog as well
    await page.getByTestId('variable-new').click();
    await dialog.getByTestId('var-name').fill(`Abstand ${p}`);
    await dialog.getByTestId('var-type').selectOption('computed');
    await dialog.getByTestId('var-function').selectOption('difference');
    await expect(dialog.getByTestId('var-inputs')).toContainText('minus');
    for (const [i, name] of [[0, `Taupunkt ${p}`], [1, `Grenze ${p}`]] as const) {
        await inputs.nth(i).getByTestId('input-kind').selectOption('variable');
        await inputs.nth(i).getByTestId('input-variable').selectOption({label: name});
    }
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    const gap = `abstand-${p}`;
    await expect(page.getByTestId(`variable-${gap}`).getByTestId('value')).toHaveText('5');

    // a program on the difference: above 6 it sets the limit to 5 - the difference follows that too
    const program = {name: `Lüften ${p} c`, active: true, visible: true, operable: true, rules: [{
        when: {op: 'any', groups: [{op: 'all', lines: [{variable: gap, op: 'gt', value: 6, trigger: 'change'}]}]},
        then: [{kind: 'variable', variable: `grenze-${p}`, to: 5}],
    }]};
    const made = await request.post('api/v1/programs', {headers: H, data: {program}});
    expect(made.status()).toBe(201);
    const key = (await made.json()).program.key as string;
    await report('HUMIDITY', 60); // a dew point of 12 °C: 7.7 above the limit
    await expect(row.getByTestId('value')).toHaveText('12 °C');
    await expect.poll(() => value(`grenze-${p}`)).toBe(5);
    await expect(page.getByTestId(`variable-${gap}`).getByTestId('value')).toHaveText('7');
    // the limit names who uses it: the program
    await expect(page.getByTestId(`variable-grenze-${p}`).getByTestId('variable-users')).toHaveText('1');

    // an input cannot be removed while a computed value uses it; a circle is refused
    const inUse = await request.delete(`api/v1/variables/${dew}`, {headers: H});
    expect(inUse.status()).toBe(409);
    expect(await inUse.json()).toMatchObject({error: 'in-use', users: [`Abstand ${p}`]});
    const circle = await request.put(`api/v1/variables/${dew}`, {headers: H, data: {variable: {...def, computed: {function: 'sum', inputs: [{variable: gap}, {variable: `grenze-${p}`}]}}, renames: {}}});
    expect(circle.status()).toBe(422);
    expect((await circle.json()).message).toContain('lead back to itself');

    // edited: the dialog shows what it is; as the absolute humidity it is 10.3 g/m³
    await row.getByTestId('variable-edit').click();
    await expect(dialog.getByTestId('var-type')).toHaveValue('computed');
    await expect(dialog.getByTestId('var-function')).toHaveValue('dew-point');
    await expect(inputs.nth(1).getByTestId('input-param')).toHaveValue('HUMIDITY');
    await dialog.getByTestId('var-function').selectOption('absolute-humidity');
    await dialog.getByTestId('var-unit').fill('g/m³');
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(row.getByTestId('value')).toHaveText('10.3 g/m³');
    // a sum takes up to eight inputs: they are added and removed in the dialog
    await page.getByTestId(`variable-${gap}`).getByTestId('variable-edit').click();
    await dialog.getByTestId('var-function').selectOption('sum');
    await dialog.getByTestId('input-add').click();
    await expect(inputs).toHaveCount(3);
    await inputs.nth(2).getByTestId('input-remove').click();
    await expect(inputs).toHaveCount(2);
    await dialog.getByRole('button', {name: 'Cancel'}).click();

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    for (const k of [gap, dew, `grenze-${p}`]) expect((await request.delete(`api/v1/variables/${k}`, {headers: H})).status()).toBe(200);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the dialog speaks German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Systemvariablen'}).click();
        await page.getByTestId('variable-new').click();
        const dialog = page.getByTestId('variable-dialog');
        await dialog.getByTestId('var-type').selectOption('computed');
        await expect(dialog.getByTestId('var-type').locator('option:checked')).toHaveText('Berechnet');
        await expect(dialog.getByTestId('var-function').locator('option:checked')).toHaveText('Taupunkt');
        await expect(dialog.getByTestId('var-inputs')).toContainText('Relative Luftfeuchte (%)');
        await expect(dialog.getByTestId('help-computed')).toHaveAttribute('href', /docs\/de\/#computed$/);
    });
});
