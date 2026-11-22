import {expect, test} from '@playwright/test';

// The dry run end to end: what a program would do now, and what it would do if one of its
// lines' source reported a value - with nothing written, nothing scheduled and no entry in the
// History. Every project makes a program and variables of its own name and removes them again.

const H = {'X-CCU-Addon-Request': '1'};

test('a dry run and a simulated trigger show what would happen and change nothing', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Probe ${p}`;
    const key = `probe-${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Nass ${p}`, type: 'bool', visible: true, labels: {true: 'nass', false: 'trocken'}}, value: false}})).status()).toBe(201);
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Stufe ${p}`, type: 'number', visible: true}, value: 3}})).status()).toBe(201);
    expect(
        (
            await request.post('api/v1/programs', {
                headers: H,
                // inactive: the dry run works all the same
                data: {program: {name, active: false, visible: true, operable: true, rules: [
                    {
                        when: {op: 'any', groups: [{op: 'all', lines: [
                            {variable: `nass-${p}`, op: 'eq', value: true, trigger: 'change'},
                            {variable: `stufe-${p}`, op: 'gt', value: 5, trigger: 'check'},
                        ]}]},
                        then: [
                            {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E03:2', param: 'STATE'}, to: true},
                            {kind: 'variable', variable: `stufe-${p}`, to: 9, after: '10min'},
                        ],
                    },
                    {when: {op: 'all', groups: [{op: 'all', lines: [{time: {kind: 'daily', at: '07:00'}, trigger: 'update'}]}]}, then: [{kind: 'variable', variable: `stufe-${p}`, to: 1}]},
                    {else: true, then: [{kind: 'variable', variable: `stufe-${p}`, to: 0}]},
                ]}},
            })
        ).status(),
    ).toBe(201);

    await page.goto('./');
    await page.getByTestId('program-search').fill(name);
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await expect(page.getByTestId('program-name')).toHaveText(name);
    await expect(page.getByTestId('help-dry-run').first()).toHaveAttribute('href', /docs\/en\/#dry-run$/);
    await page.getByTestId('dry-run').click();
    const dialog = page.getByTestId('dry-run-dialog');
    const result = dialog.getByTestId('sim-result');

    // now, as everything stands: neither condition holds, the Else would write
    await expect(result.getByTestId('run-outcome')).toHaveText('Else');
    await expect(result.getByTestId('run-trigger')).toContainText('by hand');
    const lines = result.getByTestId('run-line');
    await expect(lines).toHaveCount(3);
    await expect(lines.nth(0)).toContainText(`Nass ${p} is nass`);
    await expect(lines.nth(0).getByTestId('run-verdict')).toHaveText('false');
    await expect(lines.nth(1)).toContainText('3');
    await expect(result.getByTestId('run-step')).toHaveCount(1);
    await expect(result.getByTestId('run-status')).toHaveText('would be written');
    await expect(dialog.getByTestId('sim-go')).toBeDisabled();
    const box = await dialog.getByTestId('sim-result').boundingBox();

    // if line 1 reported "nass": that flips the line - and the second line is still false: Else again
    await dialog.getByTestId('sim-line').selectOption('1');
    await expect(result.getByTestId('run')).toHaveCount(0);
    await dialog.getByTestId('sim-go').click();
    await expect(result.getByTestId('run-trigger')).toContainText(`Nass ${p}: trocken → nass`);
    await expect(result.getByTestId('run-outcome')).toHaveText('Else');
    await expect(lines.nth(0).getByTestId('run-verdict')).toHaveText('true');
    // if it reported "trocken": no flip - it would not trigger, and the reason is said
    await dialog.getByTestId('sim-value').selectOption({label: 'trocken'});
    await dialog.getByTestId('sim-go').click();
    await expect(result.getByTestId('run-outcome')).toHaveText('would not trigger');
    await expect(result.getByTestId('run-detail')).toContainText('its result (true or false) is the same as before');
    // the line that only checks never triggers, whatever it reports
    await dialog.getByTestId('sim-line').selectOption('2');
    await dialog.getByTestId('sim-value').fill('7');
    await dialog.getByTestId('sim-go').click();
    await expect(result.getByTestId('run-outcome')).toHaveText('would not trigger');
    await expect(result.getByTestId('run-detail')).toContainText('"check only"');
    // a value that is no number is refused in words, the result's place stays
    await dialog.getByTestId('sim-value').fill('viel');
    await dialog.getByTestId('sim-go').click();
    await expect(dialog.getByTestId('sim-error')).toBeVisible();
    expect((await dialog.getByTestId('sim-result').boundingBox())!.y).toBe(box!.y);
    // the time module, tried as firing now: its block would run
    await dialog.getByTestId('sim-line').selectOption('3');
    await expect(dialog.getByTestId('sim-value')).toHaveCount(0);
    await dialog.getByTestId('sim-go').click();
    await expect(result.getByTestId('run-outcome')).toHaveText('Else if 1');
    await expect(result.getByTestId('run-trigger')).toContainText('time module (line 3)');
    await expect(lines.nth(2).getByTestId('run-verdict')).toHaveText('true');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('sim-close').click();
    await expect(dialog).toHaveCount(0);

    // nothing happened: the values as they were, no delay, no entry in the program's history
    const overview = await (await request.get('api/v1/overview')).json();
    expect(overview.variables.find((v: {def: {key: string}}) => v.def.key === `stufe-${p}`).value).toBe(3);
    expect((await (await request.get('api/v1/schedule')).json()).pending.filter((x: {program: string}) => x.program === key)).toHaveLength(0);
    expect((await (await request.get(`api/v1/runs?program=${key}`)).json()).runs).toHaveLength(0);
    await expect(page.getByTestId('program-history').getByTestId('run-empty')).toBeVisible();

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/nass-${p}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/stufe-${p}`, {headers: H})).status()).toBe(200);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the dry run says it in German', async ({page, request}, info) => {
        const key = `kurz-${info.project.name}`;
        expect((await request.post('api/v1/programs', {headers: H, data: {program: {name: `Kurz ${info.project.name}`, active: false, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: []}]}}})).status()).toBe(201);
        await page.goto('./');
        await page.getByTestId('program-search').fill(`Kurz ${info.project.name}`);
        await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
        await page.getByTestId('dry-run').click();
        const dialog = page.getByTestId('dry-run-dialog');
        await expect(dialog).toContainText('Probelauf: Kurz');
        await expect(dialog.getByTestId('sim-line')).toContainText('jetzt, so wie alles steht');
        await expect(dialog.getByTestId('run-outcome')).toHaveText('Dann');
        await expect(dialog.getByTestId('help-dry-run')).toHaveAttribute('href', /docs\/de\/#dry-run$/);
        expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    });
});
