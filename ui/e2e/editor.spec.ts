import {expect, test, type Locator, type Page} from '@playwright/test';

// The program editor end to end, against the daemon on the fake system (which has a handful of
// devices with names, rooms and datapoints): a program made from nothing with the device
// picker, changed, its lines moved by arrows and by dragging, a time control set in its form,
// a rule copied, the program copied and removed. Every project makes programs and variables of
// its own name - the four projects share one daemon - and removes them again.
test.describe.configure({mode: 'serial'});

const H = {'X-CCU-Addon-Request': '1'};

async function noSidewaysScroll(page: Page) {
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
}

/** What the rows of a list are on, in order: the value of each row's kind select. */
async function kinds(rows: Locator, testId: string): Promise<string[]> {
    return rows.evaluateAll((els, id) => els.map((el) => (el.querySelector(`[data-testid="${id}"]`) as HTMLSelectElement).value), testId);
}

test('a program is made in the editor: device picker, values as their datapoint asks, what is wrong said while typing', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Markise ${p}`;
    const rain = `Regen ${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: rain, type: 'bool', visible: true, labels: {true: 'es regnet', false: 'trocken'}}, value: false}})).status()).toBe(201);

    await page.goto('./');
    await expect(page.getByTestId('help-editor')).toHaveAttribute('href', /docs\/en\/#editor$/);
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await expect(editor).toBeVisible();
    await expect(editor.getByTestId('help-editor-lines')).toHaveAttribute('href', /#editor-lines$/);
    await expect(editor.getByTestId('help-editor-activities')).toHaveAttribute('href', /#editor-activities$/);

    // a program needs a name: said at once, and Save counts what needs attention
    await expect(editor.getByTestId('ed-name-problem')).toHaveText('A program needs a name.');
    await editor.getByTestId('editor-save').click();
    await expect(editor.getByTestId('editor-error')).toHaveText('1 place needs attention before the program can be saved.');
    await editor.getByTestId('ed-name').fill(name);
    await editor.getByTestId('ed-note').fill('Terrasse');
    await expect(editor.getByTestId('ed-name-problem')).toHaveText('');

    // a line on a device: the picker, with its search and filters
    await editor.getByTestId('group-add').click();
    const lines = editor.getByTestId('ed-line');
    const first = lines.nth(0);
    await expect(first.getByTestId('line-problem')).toHaveText('Choose a device.');
    const height = (await first.boundingBox())!.height;
    await first.getByTestId('line-device').click();
    const picker = page.getByTestId('device-picker');
    await expect(picker.getByTestId('help-editor-devices')).toHaveAttribute('href', /#editor-devices$/);
    await expect(picker.getByTestId('picker-count')).toHaveText('31 channels');
    await picker.getByTestId('picker-search').fill('küche');
    await expect(picker.getByTestId('picker-count')).toHaveText('2 channels');
    await picker.getByTestId('picker-search').fill('');
    await picker.getByTestId('picker-room').selectOption('Kitchen');
    await expect(picker.getByTestId('picker-count')).toHaveText('1 channel');
    await picker.getByTestId('picker-room').selectOption('');
    await picker.getByTestId('picker-interface').selectOption('BidCos-RF');
    await expect(picker.getByTestId('picker-count')).toHaveText('2 channels');
    await picker.getByTestId('picker-interface').selectOption('');
    await picker.getByTestId('picker-type').selectOption('HmIP-SWDO');
    await expect(picker.getByTestId('picker-count')).toHaveText('2 channels');
    await picker.getByTestId('picker-search').fill('nothing like it');
    await expect(picker.getByTestId('picker-count')).toHaveText('No channel matches.');
    await picker.getByTestId('picker-search').fill('');
    await picker.getByTestId('pick-HmIP-RF/000A1B2C3D4E02:1').getByTestId('pick').click();
    await expect(picker).toHaveCount(0);
    // the channel by its name, its datapoint chosen, the value a choice of the enumeration's entries
    await expect(first.getByTestId('line-device')).toHaveText('Fenster Küche');
    await expect(first.getByTestId('line-param')).toHaveValue('STATE');
    await first.getByTestId('line-value').selectOption({label: 'OPEN'});
    await expect(first.getByTestId('line-problem')).toHaveText('');
    await expect(first.getByTestId('ed-line-mark')).toHaveText('⚡');
    // the row is as high as it was while something was wrong with it
    expect((await first.boundingBox())!.height).toBe(height);

    // a second line on a variable, negated, check only
    await editor.getByTestId('line-add').click();
    const second = lines.nth(1);
    await second.getByTestId('line-kind').selectOption('variable');
    await expect(second.getByTestId('line-problem')).toHaveText('Choose a variable.');
    await second.getByTestId('line-variable').selectOption({label: rain});
    await second.getByTestId('line-value').selectOption({label: 'es regnet'});
    await second.getByTestId('line-not').check();
    await second.getByTestId('line-trigger').selectOption('check');
    await expect(second.getByTestId('ed-line-mark')).toHaveText('');
    // a logic value is compared for "is" and "is not" only
    await expect(second.getByTestId('line-op').locator('option')).toHaveCount(2);
    await editor.getByTestId('group-op').selectOption('any');

    // an activity on a device: a number is asked for, and a delay
    await editor.getByTestId('action-add').click();
    const actions = editor.getByTestId('ed-action');
    const act = actions.nth(0);
    await act.getByTestId('action-device').click();
    await picker.getByTestId('picker-search').fill('rollladen');
    await picker.getByTestId('pick-HmIP-RF/000A1B2C3D4E01:2').getByTestId('pick').click();
    await expect(act.getByTestId('action-param')).toHaveValue('LEVEL');
    await act.getByTestId('action-value').fill('abc');
    await expect(act.getByTestId('action-problem')).toHaveText('Enter a number.');
    await act.getByTestId('action-value').fill('0,5');
    await expect(act.getByTestId('action-problem')).toHaveText('');
    await act.getByTestId('action-timing').selectOption('delayed');
    await expect(act.getByTestId('action-problem')).toHaveText('Enter the delay as a whole number.');
    await act.getByTestId('action-delay').fill('90');
    await act.getByTestId('action-delay-unit').selectOption('min');
    await editor.getByTestId('rule-cancel-pending').check();

    // "Else…" with an activity on the variable; the editor says what that means
    await editor.getByTestId('else-add').click();
    await expect(editor.getByTestId('else-add')).toBeDisabled();
    await editor.getByTestId('ed-rule').nth(1).getByTestId('action-add').click();
    const other = actions.nth(1);
    await other.getByTestId('action-kind').selectOption('variable');
    await other.getByTestId('action-variable').selectOption({label: rain});
    await other.getByTestId('action-value').selectOption({label: 'trocken'});
    await expect(editor.getByTestId('ed-hints')).toContainText('"Trigger on change" also runs the program when a line stops being true');
    await noSidewaysScroll(page);

    await editor.getByTestId('editor-save-foot').click();
    // saved: the program's view, in the CCU's rows
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('program-name')).toHaveText(name);
    await expect(detail).toContainText('Fenster Küche');
    await expect(detail).toContainText('delayed by 1 h 30 min');
    await expect(detail.getByTestId('cancel-pending')).toBeVisible();
    await expect(detail.getByTestId('line').nth(1)).toContainText('not');
    // and as it is stored: the enumeration's index, the number, the delay
    const overview = await (await request.get('api/v1/overview')).json();
    const stored = overview.programs.find((x: {program: {name: string}}) => x.program.name === name).program;
    expect(stored.active).toBe(true);
    expect(stored.rules[0].when).toEqual({op: 'any', groups: [{op: 'any', lines: [
        {device: {interface: 'HmIP-RF', address: '000A1B2C3D4E02:1', param: 'STATE'}, op: 'eq', value: 1, trigger: 'change'},
        {variable: overview.variables.find((v: {def: {name: string}}) => v.def.name === rain).def.key, op: 'eq', value: true, not: true, trigger: 'check'},
    ]}]});
    expect(stored.rules[0].then).toEqual([{kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E01:2', param: 'LEVEL'}, to: 0.5, after: '1h30min'}]);
    expect(stored.rules[0].cancel_pending).toBe(true);
    expect(stored.rules[1].else).toBe(true);
});

test('a program is changed: lines moved by arrows and by dragging, a time control in its form, a rule copied, unsaved changes asked for', async ({page}, info) => {
    const p = info.project.name;
    const name = `Markise ${p}`;
    await page.goto('./');
    await page.getByTestId(`program-markise-${p}`).getByTestId('program-open').click();
    await page.getByTestId('program-edit').click();
    const editor = page.getByTestId('program-editor');
    const lines = editor.getByTestId('ed-line');
    await expect(lines).toHaveCount(2);
    expect(await kinds(lines, 'line-kind')).toEqual(['device', 'variable']);
    // by the arrows
    await lines.nth(0).getByTestId('line-down').click();
    expect(await kinds(lines, 'line-kind')).toEqual(['variable', 'device']);
    // by dragging the handle onto another line
    await lines.nth(1).getByTestId('line-handle').dragTo(lines.nth(0).getByTestId('line-kind'));
    expect(await kinds(lines, 'line-kind')).toEqual(['device', 'variable']);

    // a time control: the form opens, says what is missing, and lists the next times
    await editor.getByTestId('line-add').click();
    await lines.nth(2).getByTestId('line-kind').selectOption('time');
    const dialog = page.getByTestId('time-dialog');
    await expect(dialog.getByTestId('help-editor-time')).toHaveAttribute('href', /#editor-time$/);
    await dialog.getByTestId('tm-kind').selectOption('weekly');
    await dialog.getByTestId('tm-save').click();
    await expect(dialog.getByTestId('tm-error')).toHaveText('Choose at least one weekday.');
    await dialog.getByTestId('tm-day-mon').check();
    await dialog.getByTestId('tm-day-sat').check();
    await dialog.getByTestId('tm-at').fill('07:30');
    await expect(dialog.getByTestId('tm-words')).toHaveText('weekly Mon, Sat, at 07:30');
    await expect(dialog.getByTestId('tm-next').locator('div')).toHaveCount(5);
    await expect(dialog.getByTestId('tm-next').locator('div').first()).toContainText('07:30');
    await dialog.getByTestId('tm-ranged').check();
    await dialog.getByTestId('tm-duration').fill('2');
    await expect(dialog.getByTestId('tm-next').locator('div').first()).toContainText('(begin)');
    await dialog.getByTestId('tm-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(lines.nth(2).getByTestId('line-time')).toHaveText('weekly Mon, Sat, from 07:30, for 2 h');
    await expect(lines.nth(2).getByTestId('line-trigger').locator('option')).toHaveCount(2);
    // the form again, on what was set
    await lines.nth(2).getByTestId('line-time').click();
    await expect(dialog.getByTestId('tm-day-sat')).toBeChecked();
    await dialog.getByRole('button', {name: 'Cancel'}).click();

    // a rule copied: the copy is the "Else if…" after it; removed again
    const rules = editor.getByTestId('ed-rule');
    await expect(rules).toHaveCount(2);
    await rules.nth(0).getByTestId('rule-copy').click();
    await expect(rules).toHaveCount(3);
    await expect(rules.nth(1).locator('h3')).toContainText('Else if…');
    await expect(rules.nth(1).getByTestId('ed-line')).toHaveCount(3);
    await rules.nth(1).getByTestId('rule-up').click();
    await rules.nth(1).getByTestId('rule-remove').click();
    await expect(rules).toHaveCount(2);
    // a line removed with its ✕
    await lines.nth(1).getByTestId('line-remove').click();
    await expect(lines).toHaveCount(2);
    await noSidewaysScroll(page);
    await editor.getByTestId('editor-save').click();
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('line')).toHaveCount(2);
    await expect(detail).toContainText('weekly Mon, Sat, from 07:30, for 2 h');

    // changes that are not saved: another tab and "Cancel" both ask first
    await page.getByTestId('program-edit').click();
    await editor.getByTestId('ed-name').fill(name + ' anders');
    await page.getByRole('tab', {name: 'System variables'}).click();
    const leave = page.getByTestId('confirm-leave');
    await expect(leave).toContainText('not saved yet');
    await leave.getByRole('button', {name: 'Cancel'}).click();
    await expect(editor.getByTestId('ed-name')).toHaveValue(name + ' anders');
    await editor.getByTestId('editor-cancel').click();
    await page.getByTestId('confirm-discard-yes').click();
    await expect(detail.getByTestId('program-name')).toHaveText(name);
    // without changes "Cancel" just leaves
    await page.getByTestId('program-edit').click();
    await editor.getByTestId('editor-cancel-foot').click();
    await expect(detail.getByTestId('program-name')).toHaveText(name);
});

test('a program is copied and removed; one that another program runs stays', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Markise ${p}`;
    await page.goto('./');
    await page.getByTestId(`program-markise-${p}`).getByTestId('program-open').click();
    await page.getByTestId('program-copy').click();
    const editor = page.getByTestId('program-editor');
    await expect(editor.getByTestId('ed-name')).toHaveValue(`${name} (copy)`);
    await expect(editor.getByTestId('ed-active')).not.toBeChecked();
    // the copy gets an activity that runs the original
    await editor.getByTestId('ed-rule').nth(0).getByTestId('action-add').click();
    const act = editor.getByTestId('ed-action').nth(1);
    await act.getByTestId('action-kind').selectOption('program');
    await expect(act.getByTestId('action-problem')).toHaveText('Choose a program.');
    await act.getByTestId('action-program').selectOption({label: name});
    await act.getByTestId('action-what').selectOption('run-then');
    // and it comes first: by its arrow
    await act.getByTestId('action-up').click();
    expect(await kinds(editor.getByTestId('ed-rule').nth(0).getByTestId('ed-action'), 'action-kind')).toEqual(['program', 'set']);
    await editor.getByTestId('editor-save').click();
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('program-name')).toHaveText(`${name} (copy)`);
    await expect(detail.getByTestId('action').nth(0)).toContainText('run the first "Then"');

    // the original cannot go while the copy runs it
    await page.getByTestId('program-back').click();
    await page.getByTestId(`program-markise-${p}`).getByTestId('program-open').click();
    await page.getByTestId('program-delete').click();
    await expect(page.getByTestId('confirm-program-delete')).toContainText(name);
    await page.getByTestId('confirm-program-delete-yes').click();
    await expect(page.getByText('cannot be removed while other programs run or switch it')).toBeVisible();
    await expect(detail.getByTestId('program-name')).toHaveText(name);
    // the copy goes, then the original
    await page.getByTestId('program-back').click();
    await page.getByTestId(`program-markise-${p}-copy`).getByTestId('program-open').click();
    await page.getByTestId('program-delete').click();
    await page.getByTestId('confirm-program-delete-yes').click();
    await expect(page.getByTestId(`program-markise-${p}-copy`)).toHaveCount(0);
    await page.getByTestId(`program-markise-${p}`).getByTestId('program-open').click();
    await page.getByTestId('program-delete').click();
    await page.getByTestId('confirm-program-delete-yes').click();
    await expect(page.getByTestId(`program-markise-${p}`)).toHaveCount(0);
    expect((await request.delete(`api/v1/variables/regen-${p}`, {headers: H})).status()).toBe(200);
});
