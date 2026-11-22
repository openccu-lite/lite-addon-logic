import {expect, test} from '@playwright/test';

// Versions end to end: every save of a program and of a variable is listed, a version is
// compared line by line with the one before it and with now, and brought back - as a new
// version. Every project makes a program and a variable of its own name and removes them again.

const H = {'X-CCU-Addon-Request': '1'};

test('a program\'s versions: the list, the comparison line by line, a version brought back', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Fassung ${p}`;
    const key = `fassung-${p}`;
    const program = (note: string, active: boolean) => ({name, note, active, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: []}]});
    expect((await request.post('api/v1/programs', {headers: H, data: {program: program('erste Notiz', false)}})).status()).toBe(201);
    // (the history is written in the background: each save gets its entry once the one before is listed)
    const count = async () => (await (await request.get(`api/v1/programs/${key}/versions`)).json()).versions.length;
    expect(await count()).toBe(1);
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: program('zweite Notiz', false)}})).status()).toBe(200);
    expect(await count()).toBe(2);
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: program('zweite Notiz', true)}})).status()).toBe(200);
    expect(await count()).toBe(3);

    await page.goto('./');
    await page.getByTestId('program-search').fill(name);
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await expect(page.getByTestId('program-note')).toHaveText('zweite Notiz');
    await page.getByTestId('program-versions').click();
    const dialog = page.getByTestId('versions-dialog');
    await expect(dialog.getByTestId('help-versions')).toHaveAttribute('href', /docs\/en\/#versions$/);
    const rows = dialog.getByTestId('version');
    await expect(rows).toHaveCount(3);
    await expect(rows.nth(0).getByTestId('version-message')).toHaveText(`Program ${name} changed`);
    await expect(rows.nth(0).getByTestId('version-time')).toContainText('now');
    await expect(rows.nth(2).getByTestId('version-message')).toHaveText(`Program ${name} created`);
    const heights = await rows.evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height));
    expect(new Set(heights).size).toBe(1);

    // the newest version, against the one before it: the program was switched on
    const diff = dialog.getByTestId('version-diff');
    await expect(dialog.getByTestId('version-stats')).toHaveText('+1 −0 lines');
    await expect(diff.locator('.lgc-diff-add')).toHaveText('+active: true');
    await expect(diff.locator('.lgc-diff-del')).toHaveCount(0);
    // it is what is there now: nothing to restore
    await expect(dialog.getByTestId('version-restore')).toBeDisabled();
    // (and against now it does not differ)
    await dialog.getByTestId('version-mode').selectOption('now');
    await expect(dialog.getByTestId('version-same')).toHaveText('No difference.');
    await dialog.getByTestId('version-mode').selectOption('change');
    await expect(dialog.getByTestId('version-list')).toBeVisible();
    expect(await diff.getByTestId('diff-line').count()).toBeGreaterThan(3);
    const box = await diff.boundingBox();

    // the first version: everything in it came with it; against now, the note and the switch differ
    await rows.nth(2).click();
    await expect(diff.locator('.lgc-diff-del')).toHaveCount(0);
    await expect(diff.locator('.lgc-diff-add').first()).toHaveText(`+name: ${name}`);
    await dialog.getByTestId('version-mode').selectOption('now');
    await expect(dialog.getByTestId('version-stats')).toHaveText('+2 −1 lines');
    await expect(diff.locator('.lgc-diff-del').first()).toHaveText('-note: erste Notiz');
    await expect(diff.locator('.lgc-diff-add').first()).toHaveText('+note: zweite Notiz');
    expect((await diff.boundingBox())!.height).toBe(box!.height);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // bring it back: asked first, then a fourth version that says so - and the program is the first one again
    await dialog.getByTestId('version-restore').click();
    await page.getByTestId('confirm-restore-yes').click();
    await expect(rows).toHaveCount(4);
    await expect(rows.nth(0).getByTestId('version-message')).toContainText(`Program ${name} restored to its version`);
    await expect(dialog.getByTestId('version-error')).toHaveText('');
    await dialog.getByTestId('versions-close').click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId('program-note')).toHaveText('erste Notiz');
    await expect(page.getByTestId('detail-active')).not.toBeChecked();

    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
});

test('a variable\'s versions, from its edit dialog', async ({page, request}, info) => {
    const p = info.project.name;
    const key = `stand-${p}`;
    const variable = (unit: string) => ({name: `Stand ${p}`, type: 'number', visible: true, unit});
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: variable('l'), value: 1}})).status()).toBe(201);
    expect((await (await request.get(`api/v1/variables/${key}/versions`)).json()).versions).toHaveLength(1);
    expect((await request.put(`api/v1/variables/${key}`, {headers: H, data: {variable: variable('m³'), renames: {}}})).status()).toBe(200);

    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-search').fill(`Stand ${p}`);
    await page.getByTestId(`variable-${key}`).getByTestId('variable-edit').click();
    await page.getByTestId('var-versions').click();
    const dialog = page.getByTestId('versions-dialog');
    await expect(dialog.getByTestId('version')).toHaveCount(2);
    await expect(dialog.getByTestId('version-diff').locator('.lgc-diff-add')).toHaveText('+unit: m³');
    await dialog.getByTestId('version').nth(1).click();
    await dialog.getByTestId('version-restore').click();
    await page.getByTestId('confirm-restore-yes').click();
    await expect(dialog.getByTestId('version')).toHaveCount(3);
    await dialog.getByTestId('versions-close').click();
    // the variable has its first unit again
    expect((await (await request.get('api/v1/overview')).json()).variables.find((v: {def: {key: string}}) => v.def.key === key).def.unit).toBe('l');

    expect((await request.delete(`api/v1/variables/${key}`, {headers: H})).status()).toBe(200);
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the versions say it in German', async ({page, request}, info) => {
        const name = `Wort ${info.project.name}`;
        const key = `wort-${info.project.name}`;
        expect((await request.post('api/v1/programs', {headers: H, data: {program: {name, active: false, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: []}]}}})).status()).toBe(201);
        await page.goto('./');
        await page.getByTestId('program-search').fill(name);
        await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
        await page.getByTestId('program-versions').click();
        const dialog = page.getByTestId('versions-dialog');
        await expect(dialog).toContainText(`Versionen: ${name}`);
        await expect(dialog.getByTestId('version-message')).toHaveText(`Programm ${name} angelegt`);
        await expect(dialog.getByTestId('version-mode')).toContainText('was diese Version geändert hat');
        await expect(dialog.getByTestId('help-versions')).toHaveAttribute('href', /docs\/de\/#versions$/);
        expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).status()).toBe(200);
    });
});
