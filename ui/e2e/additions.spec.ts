import {expect, test} from '@playwright/test';

// What goes beyond the CCU, end to end: a line that must have held for a while, "on for", a
// random delay, an astro point with a twilight. They are written in the program's file (here:
// sent through the API); the page says them in words, the editor keeps them when the program
// is saved from it, and the engine does them - the fake system sees one command with the
// on-time and the value. Every project has its own program and variable.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const SWITCH = '000A1B2C3D4E03:2';

test('holds for, on for, a random delay and an astro point: shown, kept by the editor, and done', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Beschattung ${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Helligkeit ${p}`, type: 'number', visible: true}, value: 0}})).status()).toBe(201);
    const variable = `helligkeit-${p}`;
    const program = {
        name, active: true, visible: true, operable: true,
        rules: [{
            when: {op: 'any', groups: [{op: 'all', lines: [
                {variable, op: 'gt', value: 5, for: '1s', trigger: 'change'},
                {time: {kind: 'daily', astro: 'before_sunset', offset: '20min', point: true, twilight: 'civil'}, trigger: 'check'},
            ]}, {op: 'all', lines: [{variable, op: 'gt', value: 5, for: '1s', trigger: 'change'}]}]},
            then: [
                {kind: 'set', device: {interface: 'HmIP-RF', address: SWITCH, param: 'STATE'}, to: true, on_for: '5min'},
                {kind: 'variable', variable, to: 3, after: '1h', after_up_to: '2h'},
            ],
        }],
    };
    const made = await request.post('api/v1/programs', {headers: H, data: {program}});
    expect(made.status()).toBe(201);
    const key = (await made.json()).program.key as string;

    // the page says them in words
    await page.goto('./');
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('line').nth(0)).toContainText('greater than 5, for at least 1 s');
    await expect(detail.getByTestId('line').nth(1)).toContainText('before sunset (20 min), as a point in time, civil twilight');
    await expect(detail.getByTestId('action').nth(0)).toContainText('to true for 5 min');
    await expect(detail.getByTestId('action').nth(1)).toContainText('delayed at random by 1 h to 2 h');

    // the editor says them below their rows and keeps them when only the description changes
    await detail.getByTestId('program-edit').click();
    const editor = page.getByTestId('program-editor');
    await expect(editor.getByTestId('ed-line').nth(0).getByTestId('line-extras')).toHaveText('for at least 1 s');
    await expect(editor.getByTestId('ed-action').nth(0).getByTestId('action-extras')).toHaveText('on for 5 min');
    await expect(editor.getByTestId('ed-action').nth(1).getByTestId('action-extras')).toHaveText('delayed at random, by 2 h at most');
    await expect(editor.getByTestId('ed-action').nth(1).getByTestId('action-timing')).toHaveValue('random');
    await editor.getByTestId('ed-note').fill('Südseite');
    await editor.getByTestId('editor-save').click();
    await expect(page.getByTestId('program-detail').getByTestId('program-note')).toHaveText('Südseite');
    const stored = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(stored.rules[0].when.groups[0].lines[0].for).toBe('1s');
    expect(stored.rules[0].when.groups[0].lines[1].time).toEqual({kind: 'daily', astro: 'before_sunset', offset: '20min', point: true, twilight: 'civil'});
    expect(stored.rules[0].then[0].on_for).toBe('5min');
    expect(stored.rules[0].then[1]).toMatchObject({after: '1h', after_up_to: '2h'});

    // the comparison holds from now: a second later the program runs, without another write
    const before = ((await (await request.get(FAKE + 'dev/writes')).json()) ?? []).length;
    expect((await request.post(`api/v1/variables/${variable}/value`, {headers: H, data: {value: 9}})).status()).toBe(200);
    await expect(page.getByTestId('program-detail').getByTestId('detail-last-trigger')).toHaveText('a condition held long enough');
    const writes: {address: string; values?: Record<string, unknown>}[] = (await (await request.get(FAKE + 'dev/writes')).json()).slice(before);
    expect(writes.filter((w) => w.address === SWITCH && w.values)).toContainEqual(expect.objectContaining({interface: 'HmIP-RF', address: SWITCH, values: {ON_TIME: 300, STATE: true}}));
    // the random delay is somewhere between one and two hours from now
    const pending = (await (await request.get('api/v1/schedule')).json()).pending.filter((x: {program: string}) => x.program === key);
    expect(pending).toHaveLength(1);
    const wait = new Date(pending[0].due).getTime() - Date.now();
    expect(wait).toBeGreaterThan(59 * 60_000);
    expect(wait).toBeLessThan(121 * 60_000);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/${variable}`, {headers: H})).status()).toBe(200);
});

test('the editor sets them: holds for and the counting line on the schedule, on for, a random delay, an astro point with a twilight', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Lüften ${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Feuchte ${p}`, type: 'number', visible: true}, value: 80}})).status()).toBe(201);
    const variable = `feuchte-${p}`;

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);

    // a line on the variable that must have held for two hours
    await editor.getByTestId('group-add').click();
    const line = editor.getByTestId('ed-line').nth(0);
    await line.getByTestId('line-kind').selectOption('variable');
    await line.getByTestId('line-variable').selectOption({label: `Feuchte ${p}`});
    await line.getByTestId('line-op').selectOption('gt');
    await line.getByTestId('line-value').fill('70');
    const height = (await line.boundingBox())!.height;
    await expect(line.getByTestId('line-more')).toHaveAttribute('title', 'More options');
    await line.getByTestId('line-more').click();
    const options = page.getByTestId('options-dialog');
    await expect(options.getByTestId('help-additions')).toHaveAttribute('href', /docs\/en\/#additions$/);
    await options.getByTestId('opt-held').check();
    await options.getByTestId('opt-save').click();
    await expect(options.getByTestId('opt-error')).toHaveText('Enter how long the comparison must have held: a whole number, seven days at most.');
    await options.getByTestId('opt-hold').fill('2');
    await options.getByTestId('opt-hold-unit').selectOption('h');
    await options.getByTestId('opt-save').click();
    await expect(options).toHaveCount(0);
    await expect(line.getByTestId('line-extras')).toHaveText('for at least 2 h');
    await expect(line.getByTestId('line-more')).toHaveAttribute('title', 'for at least 2 h');
    // the row is as high as before
    expect((await line.boundingBox())!.height).toBe(height);
    // cancelled, the options stay as they were
    await line.getByTestId('line-more').click();
    await options.getByTestId('opt-hold').fill('9');
    await options.getByRole('button', {name: 'Cancel'}).click();
    await expect(line.getByTestId('line-extras')).toHaveText('for at least 2 h');

    // a time line: twenty minutes before the civil dusk, as a point
    await editor.getByTestId('line-add').click();
    const timeLine = editor.getByTestId('ed-line').nth(1);
    await timeLine.getByTestId('line-kind').selectOption('time');
    const dialog = page.getByTestId('time-dialog');
    await dialog.getByTestId('tm-when').selectOption('before_sunset');
    await dialog.getByTestId('tm-offset').fill('20');
    await expect(dialog.getByTestId('help-additions')).toHaveAttribute('href', /#additions$/);
    await dialog.getByTestId('tm-shape').selectOption('point');
    await expect(dialog.getByTestId('tm-ranged')).toBeDisabled();
    await dialog.getByTestId('tm-twilight').selectOption('civil');
    await expect(dialog.getByTestId('tm-words')).toHaveText('daily, before sunset (20 min), as a point in time, civil twilight');
    await dialog.getByTestId('tm-save').click();
    await expect(timeLine.getByTestId('line-time')).toHaveText('daily, before sunset (20 min), as a point in time, civil twilight');
    await expect(timeLine.getByTestId('line-more')).toBeDisabled();
    await timeLine.getByTestId('line-trigger').selectOption('check');
    await editor.getByTestId('group-op').selectOption('any');

    // the switch on for five minutes: the channel has an on-time
    await editor.getByTestId('action-add').click();
    const act = editor.getByTestId('ed-action').nth(0);
    await act.getByTestId('action-device').click();
    await page.getByTestId('device-picker').getByTestId(`pick-HmIP-RF/${SWITCH}`).getByTestId('pick').click();
    await expect(act.getByTestId('action-param')).toHaveValue('STATE');
    await act.getByTestId('action-more').click();
    await options.getByTestId('opt-timed').check();
    await options.getByTestId('opt-on-for').fill('5');
    await options.getByTestId('opt-save').click();
    await expect(act.getByTestId('action-extras')).toHaveText('on for 5 min');
    // the value of another datapoint cannot be "on for": the option is off, and the program loses it
    await act.getByTestId('action-source').selectOption('variable');
    await expect(act.getByTestId('action-extras')).toHaveText('');
    await act.getByTestId('action-more').click();
    await expect(options.getByTestId('opt-timed')).toBeDisabled();
    await options.getByRole('button', {name: 'Cancel'}).click();
    await act.getByTestId('action-source').selectOption('value');
    await expect(act.getByTestId('action-extras')).toHaveText('on for 5 min');

    // the variable at random between one and two hours later: chosen in the row, the end in the options
    await editor.getByTestId('action-add').click();
    const later = editor.getByTestId('ed-action').nth(1);
    await later.getByTestId('action-kind').selectOption('variable');
    await later.getByTestId('action-variable').selectOption({label: `Feuchte ${p}`});
    await later.getByTestId('action-value').fill('60');
    await later.getByTestId('action-timing').selectOption('random');
    await expect(options.getByTestId('opt-random')).toBeChecked();
    await options.getByTestId('opt-from').fill('2');
    await options.getByTestId('opt-from-unit').selectOption('h');
    await options.getByTestId('opt-up-to').fill('60');
    await options.getByTestId('opt-save').click();
    await expect(options.getByTestId('opt-error')).toHaveText('The longest delay must be longer than the shortest.');
    await options.getByTestId('opt-from').fill('1');
    await options.getByTestId('opt-up-to').fill('2');
    await options.getByTestId('opt-up-to-unit').selectOption('h');
    await options.getByTestId('opt-save').click();
    await expect(later.getByTestId('action-extras')).toHaveText('delayed at random, by 2 h at most');
    await expect(later.getByTestId('action-delay')).toHaveValue('1');
    await expect(later.getByTestId('action-delay-unit')).toHaveValue('h');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    await editor.getByTestId('editor-save').click();
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('line').nth(0)).toContainText('greater than 70, for at least 2 h');
    await expect(detail.getByTestId('action').nth(0)).toContainText('to true for 5 min');
    await expect(detail.getByTestId('action').nth(1)).toContainText('delayed at random by 1 h to 2 h');
    const key = `lueften-${p}`;
    const stored = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(stored.rules[0].when.groups[0].lines[0]).toMatchObject({variable, op: 'gt', value: 70, for: '2h'});
    expect(stored.rules[0].when.groups[0].lines[1].time).toEqual({kind: 'daily', astro: 'before_sunset', offset: '20min', point: true, twilight: 'civil'});
    expect(stored.rules[0].then[0]).toMatchObject({kind: 'set', to: true, on_for: '5min'});
    expect(stored.rules[0].then[1]).toMatchObject({kind: 'variable', to: 60, after: '1h', after_up_to: '2h'});

    // the value is above 70 since the program was saved: the schedule lists the line as counting
    await page.getByRole('tab', {name: 'Schedule'}).click();
    await page.getByTestId('schedule-filter').selectOption('hold');
    const row = page.getByTestId(`schedule-h-${key}-1`);
    await expect(row.getByTestId('schedule-kind')).toHaveText('counting');
    await expect(row).toContainText(`Feuchte ${p} greater than 70, for at least 2 h`);
    await expect(row.getByTestId('schedule-program')).toHaveText(name);
    await expect(row.getByTestId('schedule-relative')).toHaveText(/^in 1 h 59 min$|^in 2 h$/);
    // the comparison ends: the line leaves the list
    expect((await request.post(`api/v1/variables/${variable}/value`, {headers: H, data: {value: 50}})).status()).toBe(200);
    await expect(row).toHaveCount(0);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/${variable}`, {headers: H})).status()).toBe(200);
});
