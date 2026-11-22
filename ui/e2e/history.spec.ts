import {expect, test} from '@playwright/test';

// The History page end to end: every run with what triggered it, each condition line as the
// run saw it, what every activity did and what became of it - and, for one program, the notes
// that it was not triggered. Every project makes a program and variables of its own name and
// removes them again.

const H = {'X-CCU-Addon-Request': '1'};

test('why a program ran and why it did not: the entries, their lines and steps, filter, search, the program\'s own history', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Markise ${p}`;
    const key = `markise-${p}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Regen ${p}`, type: 'bool', visible: true, labels: {true: 'es regnet', false: 'trocken'}}, value: false}})).status()).toBe(201);
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Hinweis ${p}`, type: 'string', visible: true}, value: ''}})).status()).toBe(201);
    expect(
        (
            await request.post('api/v1/programs', {
                headers: H,
                data: {program: {name, active: true, visible: true, operable: true, rules: [{
                    when: {op: 'any', groups: [{op: 'all', lines: [
                        {variable: `regen-${p}`, op: 'eq', value: true, trigger: 'change'},
                        {variable: `hinweis-${p}`, op: 'ne', value: 'nie', trigger: 'check'},
                    ]}]},
                    then: [
                        {kind: 'set', device: {interface: 'HmIP-RF', address: '000A1B2C3D4E03:2', param: 'STATE'}, to: true},
                        {kind: 'variable', variable: `hinweis-${p}`, to: 'wegen Regen', after: '1h'},
                    ],
                }]}},
            })
        ).status(),
    ).toBe(201);
    const regen = async (value: boolean) => expect((await request.post(`api/v1/variables/regen-${p}/value`, {headers: H, data: {value}})).ok()).toBeTruthy();
    await regen(true); // the flip: Then
    await regen(true); // no change: not triggered
    await regen(false); // the flip back: evaluated, nothing runs

    await page.goto('./');
    await page.getByRole('tab', {name: 'History'}).click();
    await expect(page.getByTestId('help-trace')).toHaveAttribute('href', /docs\/en\/#trace$/);
    await page.getByTestId('run-search').fill(name);
    const list = page.getByTestId('page-history').getByTestId('run-list');
    const runs = list.getByTestId('run');
    // the global log: the two evaluations, the newest first
    await expect(runs).toHaveCount(2);
    await expect(runs.nth(0).getByTestId('run-outcome')).toHaveText('nothing ran');
    await expect(runs.nth(0).getByTestId('run-trigger')).toContainText(`Regen ${p}: es regnet → trocken`);
    await expect(runs.nth(1).getByTestId('run-outcome')).toHaveText('Then');
    await expect(runs.nth(1).getByTestId('run-trigger')).toContainText(`Regen ${p}: trocken → es regnet`);
    await expect(runs.nth(1).getByTestId('run-program')).toHaveText(name);
    // every row has the same height, open or not, and nothing is wider than the page
    const heights = await runs.getByTestId('run-head').evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height));
    expect(new Set(heights).size).toBe(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // the run that did something: its lines with value and truth, its steps with what became of them
    await runs.nth(1).getByTestId('run-head').click();
    const body = runs.nth(1).getByTestId('run-body');
    const lines = body.getByTestId('run-line');
    await expect(lines).toHaveCount(2);
    await expect(lines.nth(0)).toContainText(`Regen ${p} is es regnet`);
    await expect(lines.nth(0).getByTestId('run-verdict')).toHaveText('true');
    await expect(lines.nth(0).locator('.lgc-line-mark')).toHaveText('⚡');
    await expect(lines.nth(1)).toContainText(`Hinweis ${p} is not nie`);
    await expect(lines.nth(1).locator('.lgc-line-mark')).toHaveText('');
    const steps = body.getByTestId('run-step');
    await expect(steps).toHaveCount(2);
    await expect(steps.nth(0)).toContainText('Steckdose');
    await expect(steps.nth(0).getByTestId('run-status')).toContainText('sent, confirmed');
    await expect(steps.nth(1)).toContainText(`Hinweis ${p} to wegen Regen`);
    await expect(steps.nth(1).getByTestId('run-status')).toContainText('scheduled for');
    expect((await runs.nth(1).getByTestId('run-head').boundingBox())!.height).toBe(heights[0]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    // the delay is ended by hand: the step that scheduled it says so (the page asks again within seconds)
    const pending = (await (await request.get('api/v1/schedule')).json()).pending.find((x: {program: string}) => x.program === key);
    expect((await request.delete(`api/v1/pending/${pending.id}`, {headers: H})).ok()).toBeTruthy();
    await expect(steps.nth(1).getByTestId('run-status')).toContainText('(by hand)');
    // the run in which nothing ran says so
    await runs.nth(0).getByTestId('run-head').click();
    await expect(runs.nth(0).getByTestId('run-detail')).toContainText('no condition was true');
    await expect(runs.nth(0).getByTestId('run-verdict').nth(0)).toHaveText('false');

    // the filters
    await page.getByTestId('run-filter').selectOption('acted');
    await expect(runs).toHaveCount(1);
    await page.getByTestId('run-filter').selectOption('problems');
    await expect(page.getByTestId('run-empty')).toHaveText('Nothing matches.');
    await page.getByTestId('run-filter').selectOption('');

    // one program: the note that it was not triggered stands between its runs, with the reason
    await page.getByTestId('run-search').fill('');
    await page.getByTestId('run-program-filter').selectOption(key);
    await expect(page.getByTestId('help-trace-not-triggered')).toHaveAttribute('href', /docs\/en\/#trace-not-triggered$/);
    await expect(runs).toHaveCount(3);
    await expect(runs.nth(1).getByTestId('run-outcome')).toHaveText('not triggered');
    await runs.nth(1).getByTestId('run-head').click();
    await expect(runs.nth(1).getByTestId('run-detail')).toContainText('its result (true or false) is the same as before');

    // the program's own view has the same history; from the global log a click opens the program
    await page.getByTestId('run-program-filter').selectOption('');
    await page.getByTestId('run-search').fill(name);
    // (the entries opened above are still open)
    await runs.nth(0).getByTestId('run-open-program').click();
    await expect(page.getByTestId('program-name')).toHaveText(name);
    const own = page.getByTestId('program-history');
    await expect(own.getByTestId('run')).toHaveCount(3);
    await expect(own.getByTestId('run-program')).toHaveCount(0);
    await expect(own.getByTestId('run-outcome').nth(1)).toHaveText('not triggered');
    // by hand: a new entry at the top, run without a look at the conditions
    await page.getByTestId('run-then').click();
    await expect(own.getByTestId('run')).toHaveCount(4);
    await expect(own.getByTestId('run-trigger').nth(0)).toContainText('by hand');
    await own.getByTestId('run-head').nth(0).click();
    await expect(own.getByTestId('run-detail').nth(0)).toHaveText('Run without a look at the conditions.');
    // the search is kept when the tab is come back to
    await page.getByRole('tab', {name: 'History'}).click();
    await expect(page.getByTestId('run-search')).toHaveValue(name);

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/regen-${p}`, {headers: H})).status()).toBe(200);
    expect((await request.delete(`api/v1/variables/hinweis-${p}`, {headers: H})).status()).toBe(200);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the History page says it in German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Verlauf'}).click();
        await expect(page.getByRole('heading', {name: 'Verlauf'})).toBeVisible();
        await expect(page.getByTestId('help-trace')).toHaveAttribute('href', /docs\/de\/#trace$/);
        await expect(page.getByTestId('run-filter')).toContainText('nur Probleme');
    });
});
