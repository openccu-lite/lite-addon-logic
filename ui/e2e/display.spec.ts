import {expect, test} from '@playwright/test';

// The display composer, end to end, on a display described as a real HmIP-WRCD is
// (internal/fakesys/descriptions/wrcd.json): five lines with text, icon, alignment and inversion,
// a preview that follows, a signal tone; stored as "display:" with its lines; sent as one
// command - the text a CCU writes into COMBINED_PARAMETER, the placeholders filled; the history
// shows the lines as they went out; a CCU's finished text is converted into the composer's
// lines. Every project makes its own programs and removes them again.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const DISPLAY = 'HmIP-RF/000A1B2C3D4E11:1';
const THERMOSTAT = 'HmIP-RF/000A1B2C3D4E04:1';

interface Write {
    address: string;
    values?: Record<string, unknown>;
}

test('a display is composed with its preview, stored as lines and sent as one command', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Anzeige ${p}`;
    const key = `anzeige-${p}`;
    const writes = async () => (((await (await request.get(FAKE + 'dev/writes')).json()) ?? []) as Write[]).filter((w) => w.address === '000A1B2C3D4E11:1' && w.values);
    const stored = async (k: string) => (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === k).program;

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    await editor.getByTestId('action-add').click();
    const act = editor.getByTestId('ed-action').nth(0);
    await act.getByTestId('action-kind').selectOption('display');
    await expect(act.getByTestId('action-problem')).toHaveText('Choose a device.');
    await expect(act.getByTestId('action-display')).toBeDisabled();
    await expect(act.getByTestId('action-display')).toHaveText('Compose the display…');
    const height = (await act.boundingBox())!.height;

    // a channel that is no display is said
    await act.getByTestId('action-device').click();
    const picker = page.getByTestId('device-picker');
    await picker.getByTestId('picker-search').fill('thermostat');
    await picker.getByTestId(`pick-${THERMOSTAT}`).getByTestId('pick').click();
    const dialog = page.getByTestId('display-dialog');
    await dialog.getByTestId('display-text').nth(0).fill('x');
    await dialog.getByTestId('display-save').click();
    await expect(dialog.getByTestId('display-error')).toHaveText('This channel is no display.');
    await dialog.getByRole('button', {name: 'Cancel'}).click();

    // the display from the picker: the composer opens
    await act.getByTestId('action-device').click();
    await picker.getByTestId('picker-search').fill('wandtaster');
    await picker.getByTestId(`pick-${DISPLAY}`).getByTestId('pick').click();
    await expect(dialog).toContainText('Display Wandtaster Flur Anzeige');
    await expect(dialog.getByTestId('help-display')).toHaveAttribute('href', /docs\/en\/#display$/);
    await expect(dialog.getByTestId('help-display-text')).toHaveAttribute('href', /docs\/en\/#display-text$/);
    await expect(dialog.getByTestId('display-placeholders')).toContainText('{var:…}');
    const lines = dialog.locator('[data-testid^="display-line-"]');
    await expect(lines).toHaveCount(5);
    const heights = await lines.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));
    expect(new Set(heights).size).toBe(1);
    await dialog.getByTestId('display-save').click();
    await expect(dialog.getByTestId('display-error')).toHaveText('Enter a text, choose an icon or a signal tone.');

    // line 1: a text and an icon; the preview follows
    const one = dialog.getByTestId('display-line-1');
    await one.getByTestId('display-text').fill('Fenster, offen');
    await expect(dialog.getByTestId('display-error')).toHaveText('Line 1: the display cannot show ",".');
    await one.getByTestId('display-text').fill('Fenster offen');
    await expect(dialog.getByTestId('display-error')).toHaveText('');
    await expect(one.getByTestId('display-icon').locator('option')).toHaveCount(32);
    await one.getByTestId('display-icon').selectOption({label: 'Window open'});
    const shown1 = dialog.getByTestId('display-shown-1');
    await expect(shown1).toContainText('Fenster offen');
    await expect(shown1.locator('.lgc-dsp-glyph')).toHaveAttribute('title', 'WINDOW_OPEN');
    await expect(shown1).toHaveCSS('justify-content', 'center');
    // line 2 stays empty; line 3: a placeholder, to the left; line 4: an umlaut, inverted
    const three = dialog.getByTestId('display-line-3');
    await three.getByTestId('display-text').fill('{program}');
    await three.getByTestId('display-align').selectOption('LEFT');
    await expect(dialog.getByTestId('display-shown-3')).toHaveText('‹program›');
    await expect(dialog.getByTestId('display-shown-3')).toHaveCSS('justify-content', 'flex-start');
    const four = dialog.getByTestId('display-line-4');
    await four.getByTestId('display-text').fill('Tür & Tor');
    await four.getByTestId('display-invert').check();
    await expect(dialog.getByTestId('display-shown-4')).toHaveCSS('background-color', 'rgb(17, 17, 17)');
    await expect(dialog.getByTestId('display-shown-4')).toHaveCSS('color', 'rgb(244, 244, 238)');
    await expect(dialog.getByTestId('display-shown-2')).toHaveText('');
    // the signal tone with its repetitions
    await expect(dialog.getByTestId('display-repetitions')).toBeDisabled();
    await dialog.getByTestId('display-sound').selectOption({label: 'Event'});
    await dialog.getByTestId('display-repetitions').selectOption({label: '2 repetitions'});
    await dialog.getByTestId('display-interval').selectOption({label: 'every 10 s'});
    expect(await lines.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height))).toEqual(heights);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('display-save').click();
    await expect(dialog).toHaveCount(0);
    const text = 'Window open Fenster offen | – | {program} | Tür & Tor · signal tone: Event';
    await expect(act.getByTestId('action-display')).toHaveText(text);
    await expect(act.getByTestId('action-device')).toHaveText('Wandtaster Flur Anzeige');
    await expect(act.getByTestId('action-problem')).toHaveText('');
    expect((await act.boundingBox())!.height).toBe(height);
    await editor.getByTestId('editor-save').click();

    // the program says it in words; the file has the lines, the defaults left out
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('action').nth(0)).toContainText('Wandtaster Flur Anzeige');
    await expect(detail.getByTestId('action').nth(0)).toContainText(text);
    expect((await stored(key)).rules[0].then[0]).toEqual({kind: 'display', channel: DISPLAY, display: {
        lines: [{text: 'Fenster offen', icon: 'WINDOW_OPEN'}, {}, {text: '{program}', align: 'LEFT'}, {text: 'Tür & Tor', color: 'WHITE', background: 'BLACK'}],
        sound: 'EVENT', repetitions: 2, interval: 10,
    }});

    // run: one command with the text a CCU writes, the placeholder filled and cut to the display's width
    const before = (await writes()).length;
    await detail.getByTestId('run-then').click();
    await expect.poll(async () => (await writes()).length).toBe(before + 1);
    const shownName = name.slice(0, 16).trimEnd();
    expect((await writes())[before]!.values).toEqual({COMBINED_PARAMETER:
        `{DDBC=WHITE,DDTC=BLACK,DDI=24,DDA=CENTER,DDS=Fenster offen,DDID=1},{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=LEFT,DDS=${shownName},DDID=3},{DDBC=BLACK,DDTC=WHITE,DDI=0,DDA=CENTER,DDS=T³r ] Tor,DDID=4,DDC=true},{R=2,IN=10,ANS=6}`});
    await page.getByRole('tab', {name: 'History'}).click();
    await page.getByTestId('run-search').fill(name);
    const runs = page.getByTestId('page-history').getByTestId('run-list').getByTestId('run');
    await runs.nth(0).getByTestId('run-head').click();
    await expect(runs.nth(0).getByTestId('run-step').nth(0)).toContainText(`Window open Fenster offen | – | ${shownName} | Tür & Tor · signal tone: Event`);
    await expect(runs.nth(0).getByTestId('run-step').nth(0).getByTestId('run-status')).toContainText('sent, confirmed');

    // what the display does not take is refused when the program is saved
    const program = await stored(key);
    program.rules[0].then[0].channel = THERMOSTAT;
    const res = await request.put(`api/v1/programs/${key}`, {headers: H, data: {program}});
    expect(res.status()).toBe(422);
    expect((await res.json()).message).toContain('this channel is no display');

    // the composer again: it shows what is stored; a line emptied is gone
    await page.getByRole('tab', {name: 'Programs'}).click();
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await detail.getByTestId('program-edit').click();
    await act.getByTestId('action-display').click();
    await expect(one.getByTestId('display-text')).toHaveValue('Fenster offen');
    await expect(one.getByTestId('display-icon')).toHaveValue('WINDOW_OPEN');
    await expect(four.getByTestId('display-invert')).toBeChecked();
    await expect(dialog.getByTestId('display-sound')).toHaveValue('EVENT');
    await four.getByTestId('display-text').fill('');
    await four.getByTestId('display-invert').uncheck();
    await dialog.getByTestId('display-sound').selectOption('');
    await dialog.getByTestId('display-save').click();
    await editor.getByTestId('editor-save').click();
    // (the save goes out after the click: asked until it is there)
    await expect.poll(async () => (await stored(key)).rules[0].then[0].display).toEqual({lines: [{text: 'Fenster offen', icon: 'WINDOW_OPEN'}, {}, {text: '{program}', align: 'LEFT'}]});

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
});

test('a CCU\'s finished text for a display is shown in words and converted into the composer\'s lines', async ({page, request}, info) => {
    const p = info.project.name;
    const made = await request.post('api/v1/programs', {headers: H, data: {program: {name: `Anzeige importiert ${p}`, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: [
        {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E11:1', param: 'COMBINED_PARAMETER'}, to: '{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=Zeile 1,DDID=1},{DDBC=BLACK,DDTC=WHITE,DDI=30,DDA=LEFT,DDS=T³r,DDID=2,DDC=true},{R=1,IN=10,ANS=6}'},
        {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E11:1', param: 'COMBINED_PARAMETER'}, to: '{DDS=XXX,DDID=1,DDC=true}'},
    ]}]}}});
    expect(made.status()).toBe(201);
    const key = (await made.json()).program.key as string;

    await page.goto('./');
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    const detail = page.getByTestId('program-detail');
    await detail.getByTestId('program-edit').click();
    const editor = page.getByTestId('program-editor');
    const acts = editor.getByTestId('ed-action');
    await expect(acts.nth(0).getByTestId('action-decoded')).toHaveText('Zeile 1 | Bell Tür · signal tone: Event');
    await expect(acts.nth(1).getByTestId('action-convert')).toHaveCount(0);
    const height = (await acts.nth(0).boundingBox())!.height;
    await acts.nth(0).getByTestId('action-convert').click();
    await expect(acts.nth(0).getByTestId('action-kind')).toHaveValue('display');
    await expect(acts.nth(0).getByTestId('action-display')).toHaveText('Zeile 1 | Bell Tür · signal tone: Event');
    expect((await acts.nth(0).boundingBox())!.height).toBe(height);
    await editor.getByTestId('editor-save').click();
    await expect(detail.getByTestId('action').nth(0)).toContainText('Zeile 1 | Bell Tür');
    const then = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program.rules[0].then;
    expect(then[0]).toEqual({kind: 'display', channel: DISPLAY, display: {lines: [{text: 'Zeile 1'}, {text: 'Tür', icon: 'BELL', align: 'LEFT', color: 'WHITE', background: 'BLACK'}], sound: 'EVENT', repetitions: 1, interval: 10}});
    expect(then[1]).toMatchObject({kind: 'set', to: '{DDS=XXX,DDID=1,DDC=true}'});
    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the composer speaks German', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-new').click();
        const editor = page.getByTestId('program-editor');
        await editor.getByTestId('action-add').click();
        const act = editor.getByTestId('ed-action').nth(0);
        await act.getByTestId('action-kind').selectOption('display');
        await expect(act.getByTestId('action-kind').locator('option:checked')).toHaveText('Anzeige');
        await expect(act.getByTestId('action-display')).toHaveText('Anzeige gestalten…');
        await act.getByTestId('action-device').click();
        await page.getByTestId('device-picker').getByTestId(`pick-${DISPLAY}`).getByTestId('pick').click();
        const dialog = page.getByTestId('display-dialog');
        await expect(dialog).toContainText('Anzeige Wandtaster Flur Anzeige');
        await expect(dialog.getByTestId('help-display')).toHaveAttribute('href', /docs\/de\/#display$/);
        await expect(dialog.getByTestId('display-preview')).toHaveAttribute('aria-label', 'Vorschau');
        await dialog.getByTestId('display-save').click();
        await expect(dialog.getByTestId('display-error')).toHaveText('Einen Text eingeben, ein Symbol oder einen Signalton wählen.');
        await dialog.getByTestId('display-line-1').getByTestId('display-icon').selectOption({label: 'Glocke'});
        await dialog.getByTestId('display-save').click();
        await expect(act.getByTestId('action-display')).toHaveText('Glocke');
        await editor.getByTestId('editor-cancel').click();
        await page.getByTestId('confirm-discard-yes').click();
    });
});
