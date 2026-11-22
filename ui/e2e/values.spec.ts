import {expect, test} from '@playwright/test';

// Several values of a channel in one command, end to end: the activity made in the editor with
// the device picker and the form of the channel's writable datapoints (a list's entry by its
// name, a number within its limits), run - the fake system receives one putParamset -, the
// history says it, what does not fit the device's description is refused at save, and a channel
// the system does not know is written by naming its datapoints. Every project makes its own
// programs and removes them again.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const THERMOSTAT = 'HmIP-RF/000A1B2C3D4E04:1';

interface Write {
    address: string;
    values?: Record<string, unknown>;
}

test('several values of a channel: chosen in the form, sent as one command, checked against the device', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Lüften ${p}`;
    const key = `lueften-${p}`;
    const sets = async () => (((await (await request.get(FAKE + 'dev/writes')).json()) ?? []) as Write[]).filter((w) => w.address === '000A1B2C3D4E04:1' && w.values);
    const stored = async (k: string) => (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === k).program;

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    await editor.getByTestId('action-add').click();
    const act = editor.getByTestId('ed-action').nth(0);
    await act.getByTestId('action-kind').selectOption('channel');
    await expect(act.getByTestId('action-problem')).toHaveText('Choose a device.');
    await expect(act.getByTestId('action-values')).toBeDisabled();
    const height = (await act.boundingBox())!.height;

    // the channel from the picker: its form opens, with the datapoints that can be written
    await act.getByTestId('action-device').click();
    const picker = page.getByTestId('device-picker');
    await picker.getByTestId('picker-search').fill('thermostat');
    await picker.getByTestId(`pick-${THERMOSTAT}`).getByTestId('pick').click();
    const dialog = page.getByTestId('values-dialog');
    await expect(dialog).toContainText('Values of Thermostat Bad');
    await expect(dialog.getByTestId('help-values')).toHaveAttribute('href', /docs\/en\/#values$/);
    const rows = dialog.locator('[data-testid^="value-"][role="listitem"]');
    await expect(rows).toHaveCount(4);
    expect(await rows.evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')))).toEqual(['value-BOOST_MODE', 'value-CONTROL_MODE', 'value-SET_POINT_TEMPERATURE', 'value-WINDOW_STATE']);
    const heights = await rows.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));
    expect(new Set(heights).size).toBe(1);
    await dialog.getByTestId('values-save').click();
    await expect(dialog.getByTestId('values-error')).toHaveText('Choose the datapoints to write.');
    const setPoint = dialog.getByTestId('value-SET_POINT_TEMPERATURE');
    await setPoint.getByTestId('value-on').check();
    await expect(dialog.getByTestId('values-error')).toHaveText('SET_POINT_TEMPERATURE: Enter a number.');
    await setPoint.getByTestId('value-field').fill('40');
    await expect(dialog.getByTestId('values-error')).toHaveText('SET_POINT_TEMPERATURE: outside 4.5 to 30.5');
    await setPoint.getByTestId('value-field').fill('21,5');
    await expect(dialog.getByTestId('values-error')).toHaveText('');
    const window = dialog.getByTestId('value-WINDOW_STATE');
    await window.getByTestId('value-on').check();
    await window.getByTestId('value-field').selectOption({label: 'OPEN'});
    await dialog.getByTestId('value-BOOST_MODE').getByTestId('value-on').check();
    expect(await rows.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height))).toEqual(heights);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('values-save').click();
    await expect(dialog).toHaveCount(0);
    const text = 'BOOST_MODE = true, SET_POINT_TEMPERATURE = 21.5, WINDOW_STATE = OPEN';
    await expect(act.getByTestId('action-values')).toHaveText(text);
    await expect(act.getByTestId('action-device')).toHaveText('Thermostat Bad');
    await expect(act.getByTestId('action-problem')).toHaveText('');
    expect((await act.boundingBox())!.height).toBe(height);
    await editor.getByTestId('editor-save').click();

    // the program says it in words; the file has the entry by its name
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('action').nth(0)).toContainText('Thermostat Bad');
    await expect(detail.getByTestId('action').nth(0)).toContainText(text);
    expect((await stored(key)).rules[0].then[0]).toEqual({kind: 'channel', channel: THERMOSTAT, values: [{param: 'BOOST_MODE', value: true}, {param: 'SET_POINT_TEMPERATURE', value: 21.5}, {param: 'WINDOW_STATE', value: 'OPEN'}]});

    // run: one command with all three, the entry as its number
    const before = (await sets()).length;
    await detail.getByTestId('run-then').click();
    await expect.poll(async () => (await sets()).length).toBe(before + 1);
    expect((await sets())[before]!.values).toEqual({BOOST_MODE: true, SET_POINT_TEMPERATURE: 21.5, WINDOW_STATE: 1});
    await page.getByRole('tab', {name: 'History'}).click();
    await page.getByTestId('run-search').fill(name);
    const runs = page.getByTestId('page-history').getByTestId('run-list').getByTestId('run');
    await runs.nth(0).getByTestId('run-head').click();
    await expect(runs.nth(0).getByTestId('run-step').nth(0)).toContainText(text);
    await expect(runs.nth(0).getByTestId('run-step').nth(0).getByTestId('run-status')).toContainText('sent, confirmed');

    // what the device's description does not take is refused when the program is saved
    const program = await stored(key);
    for (const [values, message] of [
        [[{param: 'WINDOW_STATE', value: 'AJAR'}], 'WINDOW_STATE has no entry "AJAR"'],
        [[{param: 'SET_POINT_TEMPERATURE', value: 99}], 'SET_POINT_TEMPERATURE: 99 is outside 4.5 to 30.5'],
        [[{param: 'ACTUAL_TEMPERATURE', value: 20}], 'ACTUAL_TEMPERATURE cannot be written'],
        [[{param: 'COLOR', value: 1}], 'the channel has no datapoint COLOR'],
    ] as const) {
        program.rules[0].then[0].values = values;
        const res = await request.put(`api/v1/programs/${key}`, {headers: H, data: {program}});
        expect(res.status()).toBe(422);
        expect((await res.json()).message).toContain(message);
    }

    // the form again: what is stored is ticked; one unticked is gone
    await page.getByRole('tab', {name: 'Programs'}).click();
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await detail.getByTestId('program-edit').click();
    await act.getByTestId('action-values').click();
    await expect(dialog.getByTestId('value-SET_POINT_TEMPERATURE').getByTestId('value-on')).toBeChecked();
    await expect(dialog.getByTestId('value-SET_POINT_TEMPERATURE').getByTestId('value-field')).toHaveValue('21.5');
    await expect(dialog.getByTestId('value-CONTROL_MODE').getByTestId('value-on')).not.toBeChecked();
    await dialog.getByTestId('value-WINDOW_STATE').getByTestId('value-on').uncheck();
    await dialog.getByTestId('values-save').click();
    await editor.getByTestId('editor-save').click();
    await expect(detail.getByTestId('action').nth(0)).toContainText('BOOST_MODE = true, SET_POINT_TEMPERATURE = 21.5');
    expect((await stored(key)).rules[0].then[0].values).toHaveLength(2);

    // a channel the system does not know: its datapoints are named; one the channel does not have is said
    const other = `Unbekannt ${p}`;
    const made = await request.post('api/v1/programs', {headers: H, data: {program: {name: other, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: [
        {kind: 'channel', channel: 'HmIP-RF/NEW0000001:3', values: [{param: 'LEVEL', value: 0.5}]},
        {kind: 'channel', channel: THERMOSTAT, values: [{param: 'BOOST_MODE', value: true}]},
    ]}]}}});
    expect(made.status()).toBe(201);
    const otherKey = (await made.json()).program.key as string;
    // (the second one's datapoint disappears from the stored program's point of view: written by hand)
    await page.getByTestId('program-back').click();
    await page.getByTestId(`program-${otherKey}`).getByTestId('program-open').click();
    await detail.getByTestId('program-edit').click();
    await expect(act.getByTestId('action-values')).toHaveText('LEVEL = 0.5');
    await act.getByTestId('action-values').click();
    await expect(dialog.getByTestId('value-free')).toHaveCount(1);
    await expect(dialog.getByTestId('value-free-name')).toHaveValue('LEVEL');
    await dialog.getByTestId('value-free-add').click();
    await dialog.getByTestId('value-free-name').nth(1).fill('LEVEL_2');
    await dialog.getByTestId('value-free-value').nth(1).fill('0,25');
    await dialog.getByTestId('value-free-add').click();
    await dialog.getByTestId('value-free-name').nth(2).fill('not a name');
    await dialog.getByTestId('values-save').click();
    await expect(dialog.getByTestId('values-error')).toHaveText('"not a name" is not a datapoint\'s name.');
    await dialog.getByTestId('value-free-remove').nth(2).click();
    await dialog.getByTestId('values-save').click();
    await expect(act.getByTestId('action-values')).toHaveText('LEVEL = 0.5, LEVEL_2 = 0.25');
    await editor.getByTestId('editor-save').click();
    await expect(detail.getByTestId('action').nth(0)).toContainText('LEVEL = 0.5, LEVEL_2 = 0.25');
    expect((await stored(otherKey)).rules[0].then[0].values).toEqual([{param: 'LEVEL', value: 0.5}, {param: 'LEVEL_2', value: 0.25}]);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
    expect((await request.delete(`api/v1/programs/${otherKey}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the activity speaks German', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-new').click();
        const editor = page.getByTestId('program-editor');
        await editor.getByTestId('action-add').click();
        const act = editor.getByTestId('ed-action').nth(0);
        await act.getByTestId('action-kind').selectOption('channel');
        await expect(act.getByTestId('action-kind').locator('option:checked')).toHaveText('Mehrere Werte');
        await expect(act.getByTestId('action-values')).toHaveText('Werte wählen…');
        await act.getByTestId('action-device').click();
        await page.getByTestId('device-picker').getByTestId(`pick-${THERMOSTAT}`).getByTestId('pick').click();
        const dialog = page.getByTestId('values-dialog');
        await expect(dialog).toContainText('Werte von Thermostat Bad');
        await expect(dialog.getByTestId('help-values')).toHaveAttribute('href', /docs\/de\/#values$/);
        await dialog.getByTestId('values-save').click();
        await expect(dialog.getByTestId('values-error')).toHaveText('Die Datenpunkte wählen, die geschrieben werden.');
        await dialog.getByRole('button', {name: 'Abbrechen'}).click();
        await editor.getByTestId('editor-cancel').click();
        await page.getByTestId('confirm-discard-yes').click();
    });
});
