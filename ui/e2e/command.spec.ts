import {expect, test} from '@playwright/test';

// A command, end to end: made in the editor in its form - the program and its arguments, a
// text with a placeholder, the value of a variable and of a datapoint each as one argument,
// what becomes of its output and its exit code -, run for real by the daemon (programs every
// Linux has), the block waiting for it, the history saying the line as it was run, its output
// and a failure; a line for the shell with its values as "$1"; a dry run runs nothing. Every
// project makes its own variables and program and removes them again.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const THERMOSTAT = '000A1B2C3D4E04:1';

test('a command: made in its form, run with its values as single arguments, its output and exit code in variables', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Befehl ${p}`;
    const key = `befehl-${p}`;
    const make = async (variable: object, value: unknown) => expect((await request.post('api/v1/variables', {headers: H, data: {variable: {visible: true, ...variable}, value}})).status()).toBe(201);
    await make({name: `Wert ${p}`, type: 'string'}, 'a b; touch pwned "$(id)"');
    await make({name: `Antwort ${p}`, type: 'string'}, '');
    await make({name: `Code ${p}`, type: 'number'}, 7);
    await make({name: `Nach ${p}`, type: 'number'}, 0);
    await make({name: `Zahl ${p}`, type: 'number'}, 0);
    const value = async (k: string) => (await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === `${k}-${p}`).value;

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    await editor.getByTestId('action-add').click();
    const actions = editor.getByTestId('ed-action');
    const first = actions.nth(0);
    await first.getByTestId('action-kind').selectOption('command');
    const height = (await first.boundingBox())!.height;
    await expect(first.getByTestId('action-problem')).toHaveText('Enter the program to run.');

    // the form: the program, then one argument per row
    const dialog = page.getByTestId('command-dialog');
    await expect(dialog.getByTestId('help-command')).toHaveAttribute('href', /docs\/en\/#command$/);
    await expect(dialog.getByTestId('help-command-values')).toHaveAttribute('href', /#command-values$/);
    await expect(dialog.getByTestId('command-notes')).toContainText('never as root');
    await expect(dialog.getByTestId('command-shell-mark')).toHaveText('no shell: every argument is handed over as it is');
    const mark = (await dialog.getByTestId('command-shell-mark').boundingBox())!;
    await dialog.getByTestId('command-save').click();
    await expect(dialog.getByTestId('command-error')).toHaveText('Enter the program to run.');
    await dialog.getByTestId('command-program').fill('/bin/{value}');
    await expect(dialog.getByTestId('command-error')).toHaveText('The program is written out: a placeholder cannot stand in it.');
    await dialog.getByTestId('command-program').fill('/bin/sh');
    const args = dialog.getByTestId('command-arg');
    const add = async (text: string) => {
        await dialog.getByTestId('command-arg-add').click();
        await args.last().getByTestId('command-arg-text').fill(text);
    };
    await add('-c');
    await add('printf "%s|" "$#" "$@"');
    await add('sh');
    await add('von {program} {secret:nope}');
    await expect(dialog.getByTestId('command-error')).toHaveText('There is no secret nope: enter it in the settings first.');
    await args.last().getByTestId('command-arg-text').fill('von {program}');
    // a variable's value
    await dialog.getByTestId('command-arg-add').click();
    await args.last().getByTestId('command-arg-kind').selectOption('variable');
    await expect(dialog.getByTestId('command-error')).toHaveText('Argument 5: choose a variable.');
    await args.last().getByTestId('command-arg-variable').selectOption(`wert-${p}`);
    await expect(args.last().getByTestId('command-arg-format').locator('option')).toHaveCount(1);
    // a datapoint's value, with one decimal
    await dialog.getByTestId('command-arg-add').click();
    await args.last().getByTestId('command-arg-kind').selectOption('device');
    await expect(dialog.getByTestId('command-error')).toHaveText('Argument 6: choose a device.');
    await args.last().getByTestId('command-arg-device').click();
    const picker = page.getByTestId('device-picker');
    await picker.getByTestId('picker-search').fill(THERMOSTAT);
    await picker.getByTestId('pick').first().click();
    await expect(args.last().getByTestId('command-arg-param')).not.toHaveValue('');
    await args.last().getByTestId('command-arg-param').selectOption('ACTUAL_TEMPERATURE');
    await args.last().getByTestId('command-arg-format').selectOption('.1');
    // an argument too many goes again
    await add('weg');
    await expect(args).toHaveCount(8);
    await args.last().getByTestId('command-arg-remove').click();
    await expect(args).toHaveCount(7);
    // what becomes of it
    await dialog.getByTestId('command-timeout').fill('0');
    await expect(dialog.getByTestId('command-error')).toHaveText('The timeout is a whole number of seconds from 1 to 600.');
    await dialog.getByTestId('command-timeout').fill('5');
    await dialog.getByTestId('command-wait').check();
    await dialog.getByTestId('command-output').selectOption(`antwort-${p}`);
    await dialog.getByTestId('command-exit-code').selectOption(`code-${p}`);
    await expect(dialog.getByTestId('command-output').locator(`option[value="code-${p}"]`)).toHaveCount(1);
    await expect(dialog.getByTestId('command-exit-code').locator(`option[value="antwort-${p}"]`)).toHaveCount(0);
    await expect(dialog.getByTestId('command-error')).toHaveText('');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('command-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(first.getByTestId('action-command')).toContainText('/bin/sh -c');
    await expect(first.getByTestId('action-problem')).toHaveText('');
    expect((await first.boundingBox())!.height).toBe(height);

    // after it, in the same block: a variable is written only when the command has ended
    await editor.getByTestId('action-add').click();
    const second = actions.nth(1);
    await second.getByTestId('action-kind').selectOption('variable');
    await second.getByTestId('action-variable').selectOption(`nach-${p}`);
    await second.getByTestId('action-value').fill('1');
    await editor.getByTestId('editor-save').click();

    // the program says it in words, and is stored as a list
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('action').nth(0)).toContainText('Command');
    await expect(detail.getByTestId('action').nth(0)).toContainText(`<Wert ${p}>`);
    await expect(detail.getByTestId('action').nth(0)).toContainText(`the block waits for it, at most 5 s, output into Antwort ${p}, exit code into Code ${p}`);
    const stored = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(stored.rules[0].then[0]).toEqual({kind: 'command', command: {
        args: [{text: '/bin/sh'}, {text: '-c'}, {text: 'printf "%s|" "$#" "$@"'}, {text: 'sh'}, {text: 'von {program}'}, {variable: `wert-${p}`},
            {device: {interface: 'HmIP-RF', address: THERMOSTAT, param: 'ACTUAL_TEMPERATURE'}, format: '.1'}],
        timeout: '5s', wait: true, output: `antwort-${p}`, exit_code: `code-${p}`}});

    // a dry run runs nothing
    // (a datapoint is followed once a program names it: until its value is known the command is skipped)
    const dry = async () => {
        expect((await request.post(FAKE + 'dev/event', {data: {interface: 'HmIP-RF', address: THERMOSTAT, param: 'ACTUAL_TEMPERATURE', value: 21.26}})).ok()).toBeTruthy();
        return ((await (await request.post(`api/v1/programs/${key}/dry-run`, {headers: H, data: {}})).json()).steps as {what: string}[]).map((s) => s.what);
    };
    await expect.poll(dry).toEqual(['called', 'written']);
    expect(await value('code')).toBe(7);

    // run: three arguments arrive, each as one - whatever the variable holds
    await detail.getByTestId('run-then').click();
    const want = `3|von ${name}|a b; touch pwned "$(id)"|21.3|`;
    await expect.poll(() => value('antwort')).toBe(want);
    expect(await value('code')).toBe(0);
    await expect.poll(() => value('nach')).toBe(1);

    // the history: the line as it was run, how long it took, its output
    await page.getByRole('tab', {name: 'History'}).click();
    await page.getByTestId('run-search').fill(name);
    const runs = page.getByTestId('page-history').getByTestId('run-list').getByTestId('run');
    await expect(runs).toHaveCount(1);
    await runs.nth(0).getByTestId('run-head').click();
    const steps = runs.nth(0).getByTestId('run-step');
    await expect(steps.nth(0)).toContainText(`Command /bin/sh -c 'printf "%s|" "$#" "$@"' sh 'von ${name}'`);
    await expect(steps.nth(0).getByTestId('run-status')).toContainText(/ended after \d+ ms · output: 3\|von /);
    await expect(steps.nth(1).getByTestId('run-status')).toContainText('written');

    // a line for the shell: marked, its value is "$1", the placeholder one quoted word; it fails with 3
    stored.rules[0].then[0].command = {shell: true, line: 'printf "%s|" {var:wert-' + p + '} "$1"; echo kaputt >&2; exit 3', args: [{variable: `wert-${p}`}], wait: true, output: `zahl-${p}`, exit_code: `code-${p}`};
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: stored}})).ok()).toBeTruthy();
    expect((await request.post(`api/v1/variables/nach-${p}/value`, {headers: H, data: {value: 0}})).ok()).toBeTruthy();
    expect((await request.post(`api/v1/programs/${key}/run`, {headers: H, data: {mode: 'then'}})).ok()).toBeTruthy();
    await expect.poll(() => value('code')).toBe(3);
    await expect.poll(() => value('nach')).toBe(1);
    expect(await value('zahl')).toBe(0);
    await expect(runs).toHaveCount(2);
    await runs.nth(0).getByTestId('run-head').click();
    const failed = runs.nth(0).getByTestId('run-step').nth(0);
    await expect(failed.getByTestId('run-status')).toContainText('failed: exit code 3: kaputt');
    await expect(failed.getByTestId('run-status')).toContainText('output: a b; touch pwned "$(id)"|a b; touch pwned "$(id)"|');

    // a program there is none of, and an output that is no number
    stored.rules[0].then[0].command = {args: [{text: '/nowhere/tool'}], exit_code: `code-${p}`};
    stored.rules[0].then[1] = {kind: 'command', command: {args: [{text: 'echo'}, {text: 'viel'}], output: `zahl-${p}`}};
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: stored}})).ok()).toBeTruthy();
    expect((await request.post(`api/v1/programs/${key}/run`, {headers: H, data: {mode: 'then'}})).ok()).toBeTruthy();
    await expect.poll(() => value('code')).toBe(-1);
    await expect(runs).toHaveCount(3);
    await runs.nth(0).getByTestId('run-head').click();
    await expect(runs.nth(0).getByTestId('run-step').nth(0).getByTestId('run-status')).toContainText('failed: there is no program /nowhere/tool');
    await expect(runs.nth(0).getByTestId('run-step').nth(1).getByTestId('run-status')).toContainText(`zahl-${p} does not take the output "viel"`);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // the editor opens the shell's line in its form, marked - in a box of the same size
    await page.getByRole('tab', {name: 'Programs'}).click();
    stored.rules[0].then = [{kind: 'command', command: {shell: true, line: 'date | tr a b', args: [{variable: `wert-${p}`}]}}];
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: stored}})).ok()).toBeTruthy();
    await page.reload();
    await page.getByTestId('program-search').fill(name);
    await page.getByTestId('program-open').click();
    await page.getByTestId('program-detail').getByTestId('program-edit').click();
    await editor.getByTestId('ed-action').nth(0).getByTestId('action-command').click();
    await expect(dialog.getByTestId('command-shell')).toBeChecked();
    await expect(dialog.getByTestId('command-shell-mark')).toHaveText('Shell: the line is interpreted');
    expect((await dialog.getByTestId('command-shell-mark').boundingBox())!.height).toBe(mark.height);
    await expect(dialog.getByTestId('command-line')).toHaveValue('date | tr a b');
    await expect(dialog.getByTestId('command-arg')).toHaveCount(1);
    await expect(dialog.getByTestId('command-arg-add')).toHaveText('Add a value');
    // without the shell the line's place is the program's row, and back
    await dialog.getByTestId('command-shell').uncheck();
    await expect(dialog.getByTestId('command-program')).toHaveValue('');
    await expect(dialog.getByTestId('command-line')).toHaveCount(0);
    await dialog.getByTestId('command-shell').check();
    await expect(dialog.getByTestId('command-line')).toHaveValue('date | tr a b');
    await dialog.getByTestId('command-line').fill('');
    await dialog.getByTestId('command-save').click();
    await expect(dialog.getByTestId('command-error')).toHaveText('Enter the line the shell runs.');
    await dialog.getByRole('button', {name: 'Cancel'}).click();
    await editor.getByTestId('editor-cancel').click();

    // gone again
    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
    for (const k of ['wert', 'antwort', 'code', 'nach', 'zahl']) expect((await request.delete(`api/v1/variables/${k}-${p}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the command speaks German', async ({page}) => {
        await page.goto('./');
        await page.getByTestId('program-new').click();
        const editor = page.getByTestId('program-editor');
        await editor.getByTestId('action-add').click();
        await editor.getByTestId('ed-action').nth(0).getByTestId('action-kind').selectOption('command');
        const dialog = page.getByTestId('command-dialog');
        await expect(dialog.getByTestId('help-command')).toHaveAttribute('href', /docs\/de\/#command$/);
        await expect(dialog.getByTestId('command-arg-add')).toHaveText('Argument hinzufügen');
        await expect(dialog.getByTestId('command-notes')).toContainText('nie als root');
        await dialog.getByTestId('command-shell').check();
        await expect(dialog.getByTestId('command-shell-mark')).toHaveText('Shell: Die Zeile wird interpretiert');
        await expect(dialog.getByTestId('command-arg-add')).toHaveText('Wert hinzufügen');
        await dialog.getByTestId('command-save').click();
        await expect(dialog.getByTestId('command-error')).toHaveText('Bitte die Zeile eingeben, die die Shell ausführt.');
        await dialog.getByRole('button', {name: 'Abbrechen'}).click();
        await expect(editor.getByTestId('ed-action').nth(0).getByTestId('action-command')).toHaveText('Befehl festlegen…');
        await editor.getByTestId('editor-cancel').click();
        await page.getByTestId('confirm-discard-yes').click();
    });
});
