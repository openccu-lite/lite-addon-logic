import {expect, test} from '@playwright/test';

// What the other specs leave out, so that every control of the page is used by a test: the
// editor's program properties, a second group with its junctions, "Else if…" and "Else…" and
// their order, the range comparisons, an activity that takes its value from a datapoint; every
// field of the time control's form; a variable's properties and the removal of a list entry;
// "Evaluate now"; the History's older pages. Every project makes what it needs under its own
// name and removes it again.

const H = {'X-CCU-Addon-Request': '1'};

test('the editor: properties, groups and their junctions, Else if and Else, ranges, a value taken from a datapoint', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Rest ${p}`;
    const key = `rest-${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Mass ${p}`, type: 'number', visible: true}, value: 5}})).status()).toBe(201);

    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    // the properties
    await editor.getByTestId('ed-visible').uncheck();
    await editor.getByTestId('ed-operable').uncheck();
    await editor.getByTestId('ed-internal').check();
    await editor.getByTestId('ed-at-start').check();

    // the first group: a number in a range
    await editor.getByTestId('group-add').click();
    const lines = editor.getByTestId('ed-line');
    await lines.nth(0).getByTestId('line-kind').selectOption('variable');
    await lines.nth(0).getByTestId('line-variable').selectOption(`mass-${p}`);
    await lines.nth(0).getByTestId('line-op').selectOption('range');
    await lines.nth(0).getByTestId('line-from').fill('2');
    await lines.nth(0).getByTestId('line-below').fill('8');
    // a second line in it, outside a range, joined with OR
    await editor.getByTestId('line-add').first().click();
    await lines.nth(1).getByTestId('line-kind').selectOption('variable');
    await lines.nth(1).getByTestId('line-variable').selectOption(`mass-${p}`);
    await lines.nth(1).getByTestId('line-op').selectOption('outside');
    await lines.nth(1).getByTestId('line-from').fill('0');
    await lines.nth(1).getByTestId('line-to').fill('100');
    await editor.getByTestId('group-op').selectOption('any');
    // a second group, joined with AND
    await editor.getByTestId('group-add').first().click();
    await expect(editor.getByTestId('ed-group')).toHaveCount(2);
    await lines.nth(2).getByTestId('line-kind').selectOption('variable');
    await lines.nth(2).getByTestId('line-variable').selectOption(`mass-${p}`);
    await lines.nth(2).getByTestId('line-value').fill('5');
    await editor.getByTestId('rule-op').selectOption('all');

    // an activity that writes the variable with a datapoint's value
    const rules = editor.getByTestId('ed-rule');
    await rules.nth(0).getByTestId('action-add').click();
    const action = rules.nth(0).getByTestId('ed-action').nth(0);
    await expect(action.getByTestId('action-handle')).toBeVisible();
    await action.getByTestId('action-kind').selectOption('variable');
    await action.getByTestId('action-variable').selectOption(`mass-${p}`);
    await action.getByTestId('action-source').selectOption('device');
    await action.getByTestId('action-from-device').click();
    const picker = page.getByTestId('device-picker');
    await expect(picker.getByTestId('picker-list')).toBeVisible();
    await picker.getByTestId('pick-HmIP-RF/000A1B2C3D4E04:1').getByTestId('pick').click();
    await action.getByTestId('action-from-param').selectOption('ACTUAL_TEMPERATURE');

    // "Else if…" and "Else…": added, and the "Else if…" moved before the first rule and back
    await editor.getByTestId('rule-add').click();
    await editor.getByTestId('else-add').click();
    await expect(rules).toHaveCount(3);
    await expect(editor.getByTestId('else-add')).toBeDisabled();
    await rules.nth(0).getByTestId('rule-down').click();
    await expect(rules.nth(1).getByTestId('ed-line')).toHaveCount(3);
    await rules.nth(1).getByTestId('rule-up').click();
    await expect(rules.nth(0).getByTestId('ed-line')).toHaveCount(3);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await editor.getByTestId('editor-save').click();

    // saved as built: the view says it, the file has it
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('program-name')).toHaveText(name);
    await expect(detail).toContainText('not visible');
    await expect(detail).toContainText('not operable');
    await expect(detail).toContainText('internal');
    await expect(detail.getByTestId('detail-at-start')).toBeChecked();
    await expect(detail).toContainText('in the range from 2 to less than 8');
    await expect(detail).toContainText('outside the range from 0 to 100');
    await expect(detail).toContainText('to the value of Thermostat Bad');
    const saved = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(saved.rules).toHaveLength(3);
    expect(saved.rules[0].when.op).toBe('all');
    expect(saved.rules[0].when.groups.map((g: {op: string; lines: unknown[]}) => [g.op, g.lines.length])).toEqual([['any', 2], ['all', 1]]);
    expect(saved.rules[0].then[0].from_device.param).toBe('ACTUAL_TEMPERATURE');
    expect(saved.rules[2].else).toBe(true);
    expect([saved.visible, saved.operable, saved.internal, saved.at_start]).toEqual([false, false, true, true]);

    // "Evaluate now" runs it with its conditions: the History has the run, by hand
    await detail.getByTestId('run-check').click();
    const own = page.getByTestId('program-history');
    // (the run writes the variable its own lines look at: a note or a run of that may follow it - the oldest entry is the one by hand)
    await expect(own.getByTestId('run-trigger').last()).toContainText('by hand');
    await expect(own.getByTestId('run-time').last()).not.toHaveText('–');
    // an internal program is not in the list until asked for
    await page.getByTestId('program-back').click();
    await page.getByTestId('program-search').fill(name);
    await expect(page.getByTestId(`program-${key}`)).toHaveCount(0);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/mass-${p}`, {headers: H})).status()).toBe(200);
});

test('the time control\'s form: every way to repeat, with its fields, said in words', async ({page}, info) => {
    const p = info.project.name;
    await page.goto('./');
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(`Zeit ${p}`);
    await editor.getByTestId('group-add').click();
    await editor.getByTestId('ed-line').nth(0).getByTestId('line-kind').selectOption('time');
    const dialog = page.getByTestId('time-dialog');
    const words = dialog.getByTestId('tm-words');

    await dialog.getByTestId('tm-kind').selectOption('once');
    await dialog.getByTestId('tm-date').fill('2027-12-24');
    await dialog.getByTestId('tm-at').fill('18:00');
    await expect(words).toHaveText('once on 2027-12-24, at 18:00');

    await dialog.getByTestId('tm-kind').selectOption('interval');
    await dialog.getByTestId('tm-interval').fill('15');
    await dialog.getByTestId('tm-interval-unit').selectOption('min');
    await dialog.getByTestId('tm-at').fill('06:00');
    await expect(words).toHaveText('every 15 min, from 06:00');

    await dialog.getByTestId('tm-kind').selectOption('daily');
    await dialog.getByTestId('tm-days').selectOption('workdays');
    await dialog.getByTestId('tm-at').fill('07:00');
    await dialog.getByTestId('tm-ranged').check();
    await dialog.getByTestId('tm-duration').fill('90');
    await dialog.getByTestId('tm-duration-unit').selectOption('min');
    await expect(words).toHaveText('on workdays, from 07:00, for 90 min');
    await dialog.getByTestId('tm-ranged').uncheck();
    await dialog.getByTestId('tm-days').selectOption('all');
    // "every three days" is counted from a day: without "valid from" the form says so
    await dialog.getByTestId('tm-every').fill('3');
    await expect(words).toHaveText('–');
    await dialog.getByTestId('tm-save').click();
    await expect(dialog.getByTestId('tm-error')).toContainText('counted from "valid from"');
    await dialog.getByTestId('tm-from').fill('2027-01-01');
    await expect(words).toHaveText('every 3 days, at 07:00, valid from 2027-01-01');
    await dialog.getByTestId('tm-every').fill('');
    await dialog.getByTestId('tm-from').fill('');

    await dialog.getByTestId('tm-kind').selectOption('monthly');
    await dialog.getByTestId('tm-by').selectOption('day');
    await dialog.getByTestId('tm-day').fill('31');
    await expect(words).toHaveText('monthly on day 31, at 07:00');
    await dialog.getByTestId('tm-by').selectOption('weekday');
    await dialog.getByTestId('tm-nth').selectOption('2');
    await dialog.getByTestId('tm-weekday').selectOption('tue');
    await expect(words).toHaveText('monthly on the 2. Tue, at 07:00');

    await dialog.getByTestId('tm-kind').selectOption('yearly');
    await dialog.getByTestId('tm-by').selectOption('day');
    await dialog.getByTestId('tm-day').fill('24');
    await dialog.getByTestId('tm-month').selectOption('12');
    await expect(words).toHaveText('yearly on 24/12, at 07:00');

    // the validity and the number of times
    await dialog.getByTestId('tm-from').fill('2027-01-01');
    await dialog.getByTestId('tm-until').fill('2030-12-31');
    await dialog.getByTestId('tm-count').fill('4');
    await expect(words).toHaveText('yearly on 24/12, at 07:00, valid from 2027-01-01, until 2030-12-31, 4 times');
    await expect(dialog.getByTestId('tm-next').locator('div').first()).toContainText('24/12/2027');
    // a date that is none is said, and not applied
    await dialog.getByTestId('tm-until').fill('irgendwann');
    await dialog.getByTestId('tm-save').click();
    await expect(dialog.getByTestId('tm-error')).not.toHaveText('');
    await dialog.getByTestId('tm-until').fill('2030-12-31');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('tm-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(editor.getByTestId('line-time')).toHaveText('yearly on 24/12, at 07:00, valid from 2027-01-01, until 2030-12-31, 4 times');
    // (not saved: the editor is left)
    await editor.getByTestId('editor-cancel').click();
    await page.getByTestId('confirm-discard-yes').click();
});

test('a variable\'s properties and a list entry removed; a search that finds nothing', async ({page, request}, info) => {
    const p = info.project.name;
    const key = `wahl-${p}`;
    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-new').click();
    const dialog = page.getByTestId('variable-dialog');
    // without a name: said in the dialog
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-name-error')).not.toHaveText('');
    await dialog.getByTestId('var-name').fill(`Wahl ${p}`);
    await dialog.getByTestId('var-type').selectOption('list');
    await dialog.getByTestId('var-entry').nth(0).fill('eins');
    await dialog.getByTestId('var-entry-add').click();
    await dialog.getByTestId('var-entry').nth(1).fill('zwei');
    await dialog.getByTestId('var-entry-add').click();
    await dialog.getByTestId('var-entry').nth(2).fill('drei');
    await dialog.getByTestId('var-entry-remove').nth(1).click();
    await expect(dialog.getByTestId('var-entry')).toHaveCount(2);
    await dialog.getByTestId('var-visible').uncheck();
    await dialog.getByTestId('var-internal').check();
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    const def = (await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === key).def;
    expect([def.entries, def.visible, def.internal]).toEqual([['eins', 'drei'], false, true]);
    // an internal variable shows when asked for
    await page.getByTestId('variable-search').fill(`Wahl ${p}`);
    await expect(page.getByTestId(`variable-${key}`)).toHaveCount(0);
    await page.getByTestId('show-internal-variables').check();
    await expect(page.getByTestId(`variable-${key}`)).toBeVisible();
    // what the addon refuses is said in the dialog, which stays: an entry a program uses cannot be removed
    expect((await request.post('api/v1/programs', {headers: H, data: {program: {name: `Nutzt ${p}`, active: false, visible: true, operable: true, rules: [{when: {op: 'all', groups: [{op: 'all', lines: [{variable: key, op: 'eq', value: 'eins', trigger: 'change'}]}]}, then: []}]}}})).status()).toBe(201);
    await page.getByTestId(`variable-${key}`).getByTestId('variable-edit').click();
    await dialog.getByTestId('var-entry-remove').nth(0).click();
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-error')).toContainText('cannot be removed while programs use it');
    await dialog.getByRole('button', {name: 'Cancel'}).click();
    expect((await request.delete(`api/v1/programs/nutzt-${p}`, {headers: H})).status()).toBe(200);
    // a search that finds nothing says so, on both lists
    await page.getByTestId('variable-search').fill('gibt es nicht xyz');
    await expect(page.getByTestId('page-variables')).toContainText('No variable matches the search.');
    expect((await request.post('api/v1/programs', {headers: H, data: {program: {name: `Da ${p}`, active: false, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: []}]}}})).status()).toBe(201);
    await page.getByRole('tab', {name: 'Programs'}).click();
    await page.getByTestId('program-search').fill('gibt es nicht xyz');
    await expect(page.getByTestId('no-match')).toBeVisible();
    expect((await request.delete(`api/v1/programs/da-${p}`, {headers: H})).status()).toBe(200);

    expect((await request.delete(`api/v1/variables/${key}`, {headers: H})).status()).toBe(200);
});

test('the History\'s older entries come by pages', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Viel ${p}`;
    const key = `viel-${p}`;
    expect((await request.post('api/v1/programs', {headers: H, data: {program: {name, active: false, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: []}]}}})).status()).toBe(201);
    // (a run by hand every 600 ms: below the loop guard's twenty runs in ten seconds... too slow for 105 - so the guard is widened for the test and put back)
    const settings = await (await request.get('api/v1/settings')).json();
    expect((await request.put('api/v1/settings', {headers: H, data: {...settings, loop_guard: {runs: 1000, window_seconds: 10, pause_seconds: 60}}})).ok()).toBeTruthy();
    for (let i = 0; i < 105; i++) expect((await request.post(`api/v1/programs/${key}/run`, {headers: H, data: {mode: 'then'}})).ok()).toBeTruthy();
    expect((await request.put('api/v1/settings', {headers: H, data: settings})).ok()).toBeTruthy();

    await page.goto('./');
    await page.getByRole('tab', {name: 'History'}).click();
    await page.getByTestId('run-program-filter').selectOption(key);
    await expect(page.getByTestId('run-own-note')).toBeVisible();
    const runs = page.getByTestId('page-history').getByTestId('run');
    await expect(runs).toHaveCount(100);
    await page.getByTestId('run-older').click();
    await expect(runs).toHaveCount(105);
    await expect(page.getByTestId('run-older')).toHaveCount(0);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
});
