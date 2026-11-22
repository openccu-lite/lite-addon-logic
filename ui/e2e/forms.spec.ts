import {expect, test, type Locator, type Page} from '@playwright/test';

// The forms of the channels that have one, end to end, on devices described as real ones are
// (internal/fakesys/descriptions): a signal light, a sound, a siren, a coloured light and a blind
// with slats are set in their forms - colours to click, shares, the devices' lists in words -,
// stored as the same values the plain form writes, sent as one command each; the plain form is
// one click away and back; an imported COMBINED_PARAMETER string is converted into a form. Every
// project makes its own programs and removes them again.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const SIGNAL = 'HmIP-RF/000A1B2C3D4E06:1';
const SOUND = 'HmIP-RF/000A1B2C3D4E07:1';
const SIREN = 'HmIP-RF/000A1B2C3D4E08:1';
const LIGHT = 'HmIP-RF/000A1B2C3D4E09:1';
const BLIND = 'HmIP-RF/000A1B2C3D4E10:1';

interface Write {
    address: string;
    values?: Record<string, unknown>;
}

async function addOn(page: Page, editor: Locator, index: number, channel: string, search: string): Promise<Locator> {
    await editor.getByTestId('action-add').click();
    const act = editor.getByTestId('ed-action').nth(index);
    await act.getByTestId('action-kind').selectOption('channel');
    await act.getByTestId('action-device').click();
    const picker = page.getByTestId('device-picker');
    await picker.getByTestId('picker-search').fill(search);
    await picker.getByTestId(`pick-${channel}`).getByTestId('pick').click();
    return act;
}

const heights = (dialog: Locator) => dialog.locator('[data-testid^="field-"].lgc-cf-row').evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));

test('the forms: a signal light, a sound, a siren, a coloured light and a blind, each sent as one command', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Formulare ${p}`;
    const key = `formulare-${p}`;
    const writes = async (address: string) => (((await (await request.get(FAKE + 'dev/writes')).json()) ?? []) as Write[]).filter((w) => w.address === address && w.values);
    const stored = async (k: string) => (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === k).program;
    const before = Object.fromEntries(await Promise.all([SIGNAL, SOUND, SIREN, LIGHT, BLIND].map(async (c) => [c, (await writes(c.split('/')[1]!)).length])));

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    const form = page.getByTestId('form-dialog');

    // the signal light: its colours to click, the brightness, the behaviour in words, "for"
    const signal = await addOn(page, editor, 0, SIGNAL, 'diele');
    const rowHeight = (await signal.boundingBox())!.height;
    await expect(form).toContainText('Signal light Taster Diele Licht');
    await expect(form.getByTestId('help-forms')).toHaveAttribute('href', /docs\/en\/#forms$/);
    expect(await form.locator('[data-testid^="field-"].lgc-cf-row').evaluateAll((els) => els.map((el) => el.getAttribute('data-testid')))).toEqual(['field-color', 'field-level', 'field-behaviour', 'field-duration', 'field-ramp']);
    const first = await heights(form);
    await expect(form.getByTestId('swatch-WHITE')).toHaveAttribute('aria-checked', 'true');
    await form.getByTestId('swatch-RED').click();
    await expect(form.getByTestId('swatch-RED')).toHaveAttribute('aria-checked', 'true');
    await expect(form.getByTestId('swatch-WHITE')).toHaveAttribute('aria-checked', 'false');
    await expect(form.getByTestId('swatch-OLD_VALUE')).toHaveText('As before');
    const level = form.getByTestId('field-level');
    await level.getByTestId('field-value').fill('120');
    await form.getByTestId('form-save').click();
    await expect(form.getByTestId('form-error')).toHaveText('Brightness: outside 0 to 100');
    await level.getByTestId('field-range').fill('50');
    await expect(level.getByTestId('field-value')).toHaveValue('50');
    await expect(form.getByTestId('form-error')).toHaveText('');
    await form.getByTestId('field-behaviour').getByTestId('field-value').selectOption({label: 'Blinking, medium'});
    const duration = form.getByTestId('field-duration');
    await expect(duration.getByTestId('field-on')).not.toBeChecked();
    await duration.getByTestId('field-on').check();
    await duration.getByTestId('field-value').fill('10');
    await duration.getByTestId('field-unit').selectOption({label: 'min'});
    expect(await heights(form)).toEqual(first);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // the plain form with what is set here, a datapoint more, and back: the form keeps it beside its fields
    await form.getByTestId('form-all').click();
    const plain = page.getByTestId('values-dialog');
    await expect(plain.getByTestId('value-COLOR').getByTestId('value-on')).toBeChecked();
    await expect(plain.getByTestId('value-COLOR').getByTestId('value-field').locator('option:checked')).toHaveText('RED');
    await expect(plain.getByTestId('value-LEVEL').getByTestId('value-field')).toHaveValue('0.5');
    await expect(plain.getByTestId('value-RAMP_TIME_VALUE').getByTestId('value-on')).not.toBeChecked();
    await plain.getByTestId('value-COMBINED_PARAMETER').getByTestId('value-on').check();
    await plain.getByTestId('value-COMBINED_PARAMETER').getByTestId('value-field').fill('x');
    await plain.getByTestId('values-form').click();
    await expect(form.getByTestId('form-rest')).toHaveText('Also sent: COMBINED_PARAMETER = x');
    await expect(form.getByTestId('swatch-RED')).toHaveAttribute('aria-checked', 'true');
    await expect(duration.getByTestId('field-on')).toBeChecked();
    await form.getByTestId('form-all').click();
    await plain.getByTestId('value-COMBINED_PARAMETER').getByTestId('value-on').uncheck();
    await plain.getByTestId('values-form').click();
    await expect(form.getByTestId('form-rest')).toHaveText('');
    await form.getByTestId('form-save').click();
    await expect(form).toHaveCount(0);
    await expect(signal.getByTestId('action-values')).toHaveText('Red, Brightness 50 %, Blinking, medium, For 10 min');
    await expect(signal.getByTestId('action-values')).toHaveAttribute('title', 'COLOR = RED, LEVEL = 0.5, COLOR_BEHAVIOUR = BLINKING_MIDDLE, DURATION_VALUE = 10, DURATION_UNIT = M');
    expect((await signal.boundingBox())!.height).toBe(rowHeight);

    // the sound: a file from the device's list, the volume, repetitions
    const sound = await addOn(page, editor, 1, SOUND, 'gong ton');
    await expect(form).toContainText('Sound of Gong Ton');
    await form.getByTestId('field-sound').getByTestId('field-value').selectOption({label: 'Sound file 3'});
    await form.getByTestId('field-level').getByTestId('field-value').fill('30');
    await form.getByTestId('field-repetitions').getByTestId('field-on').check();
    await form.getByTestId('field-repetitions').getByTestId('field-value').selectOption({label: '2 repetitions'});
    await form.getByTestId('form-save').click();
    await expect(sound.getByTestId('action-values')).toHaveText('Sound file 3, Volume 30 %, 2 repetitions');

    // the siren: sound, light and how long - none of them can be left out
    const siren = await addOn(page, editor, 2, SIREN, 'sirene');
    await expect(form).toContainText('Siren Sirene');
    await expect(form.getByTestId('field-on')).toHaveCount(0);
    await form.getByTestId('field-acoustic').getByTestId('field-value').selectOption({label: 'Falling tone'});
    await form.getByTestId('field-optical').getByTestId('field-value').selectOption({label: 'Double flashes'});
    await form.getByTestId('field-duration').getByTestId('field-value').fill('3');
    await form.getByTestId('field-duration').getByTestId('field-unit').selectOption({label: 'min'});
    await form.getByTestId('form-save').click();
    await expect(siren.getByTestId('action-values')).toHaveText('Falling tone, Double flashes, For 3 min');

    // the coloured light: a colour by hue and saturation - or a colour temperature, one of the two
    const light = await addOn(page, editor, 3, LIGHT, 'lichtband');
    await expect(form).toContainText('Coloured light Lichtband');
    const hue = form.getByTestId('field-hue');
    const temperature = form.getByTestId('field-temperature');
    await expect(hue.getByTestId('field-on')).toBeChecked();
    const tall = await heights(form);
    await hue.getByTestId('field-range').fill('120');
    await expect(hue.getByTestId('field-value')).toHaveValue('120');
    await hue.getByTestId('field-sat-range').fill('80');
    await expect(hue.getByTestId('field-sat')).toHaveValue('80');
    await expect(hue.getByTestId('field-colour')).toHaveCSS('background-color', 'rgb(51, 255, 51)');
    await temperature.getByTestId('field-on').check();
    await expect(hue.getByTestId('field-on')).not.toBeChecked();
    await temperature.getByTestId('field-value').fill('500');
    await form.getByTestId('form-save').click();
    await expect(form.getByTestId('form-error')).toHaveText('Colour temperature: outside 1000 to 10200');
    await hue.getByTestId('field-on').check();
    await expect(temperature.getByTestId('field-on')).not.toBeChecked();
    await expect(form.getByTestId('form-error')).toHaveText('');
    await form.getByTestId('field-level').getByTestId('field-value').fill('60');
    await form.getByTestId('field-effect').getByTestId('field-on').check();
    await form.getByTestId('field-effect').getByTestId('field-value').selectOption({label: 'Effect 2, at once'});
    expect(await heights(form)).toEqual(tall);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await form.getByTestId('form-save').click();
    await expect(light.getByTestId('action-values')).toHaveText('hue 120°, saturation 80 %, Brightness 60 %, Effect 2, at once');

    // the blind: height and slats together
    const blind = await addOn(page, editor, 4, BLIND, 'jalousie');
    await expect(form).toContainText('Blind Jalousie Büro');
    await form.getByTestId('field-level').getByTestId('field-value').fill('40');
    await form.getByTestId('field-slats').getByTestId('field-range').fill('25');
    await form.getByTestId('form-save').click();
    await expect(blind.getByTestId('action-values')).toHaveText('Height 40 %, Slats 25 %');
    await editor.getByTestId('editor-save').click();

    // the file has the plain values, the lists' entries by their names
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('action').nth(4)).toContainText('LEVEL = 0.4, LEVEL_2 = 0.25');
    const then = (await stored(key)).rules[0].then;
    expect(then.map((a: {values: unknown}) => a.values)).toEqual([
        [{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 0.5}, {param: 'COLOR_BEHAVIOUR', value: 'BLINKING_MIDDLE'}, {param: 'DURATION_VALUE', value: 10}, {param: 'DURATION_UNIT', value: 'M'}],
        [{param: 'SOUNDFILE', value: 'SOUNDFILE_003'}, {param: 'LEVEL', value: 0.3}, {param: 'REPETITIONS', value: 'REPETITIONS_002'}],
        [{param: 'ACOUSTIC_ALARM_SELECTION', value: 'FREQUENCY_FALLING'}, {param: 'OPTICAL_ALARM_SELECTION', value: 'DOUBLE_FLASHING_REPEATING'}, {param: 'DURATION_VALUE', value: 3}, {param: 'DURATION_UNIT', value: 'M'}],
        [{param: 'HUE', value: 120}, {param: 'SATURATION', value: 0.8}, {param: 'LEVEL', value: 0.6}, {param: 'EFFECT', value: 'EFFECT_02_INTERRUPT_CURRENT_PROFILE'}],
        [{param: 'LEVEL', value: 0.4}, {param: 'LEVEL_2', value: 0.25}],
    ]);

    // run: one command per channel, each entry as the number the device takes
    await detail.getByTestId('run-then').click();
    const sent = async (channel: string) => (await writes(channel.split('/')[1]!))[before[channel]]?.values;
    await expect.poll(async () => await sent(BLIND)).toEqual({LEVEL: 0.4, LEVEL_2: 0.25});
    expect(await sent(SIGNAL)).toEqual({COLOR: 4, LEVEL: 0.5, COLOR_BEHAVIOUR: 3, DURATION_VALUE: 10, DURATION_UNIT: 1});
    expect(await sent(SOUND)).toEqual({SOUNDFILE: 3, LEVEL: 0.3, REPETITIONS: 2});
    expect(await sent(SIREN)).toEqual({ACOUSTIC_ALARM_SELECTION: 2, OPTICAL_ALARM_SELECTION: 3, DURATION_VALUE: 3, DURATION_UNIT: 1});
    expect(await sent(LIGHT)).toEqual({HUE: 120, SATURATION: 0.8, LEVEL: 0.6, EFFECT: 4});
    for (const c of [SIGNAL, SOUND, SIREN, LIGHT, BLIND]) expect((await writes(c.split('/')[1]!)).length).toBe(before[c] + 1);

    // the form again: it shows what is stored
    await detail.getByTestId('program-edit').click();
    await signal.getByTestId('action-values').click();
    await expect(form.getByTestId('swatch-RED')).toHaveAttribute('aria-checked', 'true');
    await expect(form.getByTestId('field-level').getByTestId('field-value')).toHaveValue('50');
    await expect(form.getByTestId('field-duration').getByTestId('field-unit').locator('option:checked')).toHaveText('min');
    await expect(form.getByTestId('field-ramp').getByTestId('field-on')).not.toBeChecked();
    await form.getByRole('button', {name: 'Cancel'}).click();
    await editor.getByTestId('editor-cancel').click();

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
});

test('a COMBINED_PARAMETER string of a CCU program is shown in words and converted into a form', async ({page, request}, info) => {
    const p = info.project.name;
    const made = await request.post('api/v1/programs', {headers: H, data: {program: {name: `Importiert ${p}`, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: [
        {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E06:1', param: 'COMBINED_PARAMETER'}, to: 'L=50,C=4,DV=10,DU=1,CB=1'},
        {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E06:1', param: 'COMBINED_PARAMETER'}, to: 'L=100,OT=5'},
    ]}]}}});
    expect(made.status()).toBe(201);
    const key = (await made.json()).program.key as string;

    await page.goto('./');
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    const detail = page.getByTestId('program-detail');
    await detail.getByTestId('program-edit').click();
    const editor = page.getByTestId('program-editor');
    const acts = editor.getByTestId('ed-action');
    // the one that can be read is said in words; the other stays the string it is
    await expect(acts.nth(0).getByTestId('action-decoded')).toHaveText('Red, Brightness 50 %, On, For 10 min');
    await expect(acts.nth(0).getByTestId('action-convert')).toHaveText('Convert into a form');
    await expect(acts.nth(1).getByTestId('action-convert')).toHaveCount(0);
    const height = (await acts.nth(0).boundingBox())!.height;
    expect((await acts.nth(1).boundingBox())!.height).toBe(height);
    await acts.nth(0).getByTestId('action-convert').click();
    await expect(acts.nth(0).getByTestId('action-kind')).toHaveValue('channel');
    await expect(acts.nth(0).getByTestId('action-device')).toHaveText('Taster Diele Licht');
    await expect(acts.nth(0).getByTestId('action-values')).toHaveText('Red, Brightness 50 %, On, For 10 min');
    await expect(acts.nth(0).getByTestId('action-convert')).toHaveCount(0);
    expect((await acts.nth(0).boundingBox())!.height).toBe(height);
    await editor.getByTestId('editor-save').click();
    await expect(detail.getByTestId('action').nth(0)).toContainText('COLOR = RED');
    const then = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program.rules[0].then;
    expect(then[0]).toEqual({kind: 'channel', channel: SIGNAL, values: [{param: 'LEVEL', value: 0.5}, {param: 'COLOR', value: 'RED'}, {param: 'DURATION_VALUE', value: 10}, {param: 'DURATION_UNIT', value: 'M'}, {param: 'COLOR_BEHAVIOUR', value: 'ON'}]});
    expect(then[1]).toMatchObject({kind: 'set', to: 'L=100,OT=5'});
    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the forms speak German', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-new').click();
        const editor = page.getByTestId('program-editor');
        await addOn(page, editor, 0, SIGNAL, 'diele');
        const form = page.getByTestId('form-dialog');
        await expect(form).toContainText('Signalleuchte Taster Diele Licht');
        await expect(form.getByTestId('help-forms')).toHaveAttribute('href', /docs\/de\/#forms$/);
        await expect(form.getByTestId('field-color')).toContainText('Farbe');
        await expect(form.getByTestId('swatch-RED')).toHaveAttribute('title', 'Rot');
        await expect(form.getByTestId('field-behaviour').getByTestId('field-value').locator('option:checked')).toHaveText('Ein');
        await expect(form.getByTestId('form-all')).toHaveText('Alle Datenpunkte…');
        await form.getByTestId('form-all').click();
        await expect(page.getByTestId('values-dialog').getByTestId('values-form')).toHaveText('Formular…');
        await page.getByTestId('values-dialog').getByTestId('values-form').click();
        await form.getByTestId('form-save').click();
        await expect(editor.getByTestId('ed-action').nth(0).getByTestId('action-values')).toHaveText('Weiß, Helligkeit 100 %, Ein');
        await editor.getByTestId('editor-cancel').click();
        await page.getByTestId('confirm-discard-yes').click();
    });
});
