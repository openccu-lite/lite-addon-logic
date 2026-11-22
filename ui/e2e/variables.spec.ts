import {expect, test} from '@playwright/test';

// System variables on the page: create, set, change, the alarm messages, remove - against the
// daemon on the fake system. Every project makes variables of its own name: the four projects
// share one daemon.
test.describe.configure({mode: 'serial'});

test('a number: created with limits, set, refused outside them, changed, removed', async ({page}, info) => {
    const name = `Temperatur ${info.project.name}`;
    const key = `variable-temperatur-${info.project.name}`;
    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-new').click();
    const dialog = page.getByTestId('variable-dialog');
    // a variable needs a name: the dialog says so and stays
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-name-error')).toHaveText('A variable needs a name.');
    await dialog.getByTestId('var-name').fill(name);
    await dialog.getByTestId('var-type').selectOption('number');
    await dialog.getByTestId('var-unit').fill('°C');
    await dialog.getByTestId('var-min').fill('10');
    await dialog.getByTestId('var-max').fill('5');
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-max-error')).toHaveText('The maximum is below the minimum.');
    await dialog.getByTestId('var-min').fill('-40');
    await dialog.getByTestId('var-max').fill('60');
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);

    const row = page.getByTestId(key);
    await expect(row).toContainText(name);
    await expect(row).toContainText('Number');
    await expect(row.getByTestId('value')).toHaveValue('0');
    // set a value; one outside the limits is refused before it is sent
    await row.getByTestId('value').fill('21,5');
    await row.getByTestId('value-set').click();
    await expect(row.getByTestId('value')).toHaveValue('21.5');
    // what is wrong is said in the line above the list, which is always there: the row does not grow
    const before = (await row.boundingBox())!.height;
    await row.getByTestId('value').fill('99');
    await row.getByTestId('value-set').click();
    await expect(page.getByTestId('value-error')).toHaveText(`${name}: The largest value is 60.`);
    expect((await row.boundingBox())!.height).toBe(before);
    await row.getByTestId('value').fill('warm');
    await row.getByTestId('value-set').click();
    await expect(page.getByTestId('value-error')).toHaveText(`${name}: Enter a number.`);

    // change: a description, logged
    await row.getByTestId('variable-edit').click();
    await dialog.getByTestId('var-note').fill('Wohnzimmer');
    await dialog.getByTestId('var-logged').check();
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(row).toContainText('Wohnzimmer');
    await expect(row).toContainText('logged');

    // remove, with the page's own question
    await row.getByTestId('variable-delete').click();
    await expect(page.getByTestId('confirm-delete')).toContainText(name);
    await page.getByTestId('confirm-delete-yes').click();
    await expect(row).toHaveCount(0);
});

test('a value list: entries, a renamed entry keeps the value', async ({page}, info) => {
    const name = `Modus ${info.project.name}`;
    const row = page.getByTestId(`variable-modus-${info.project.name}`);
    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-new').click();
    const dialog = page.getByTestId('variable-dialog');
    await dialog.getByTestId('var-name').fill(name);
    await dialog.getByTestId('var-type').selectOption('list');
    await dialog.getByTestId('var-save').click();
    await expect(dialog.getByTestId('var-entries-error')).toHaveText('Every entry needs a text.');
    await dialog.getByTestId('var-entry').nth(0).fill('Auto');
    await dialog.getByTestId('var-entry-add').click();
    await dialog.getByTestId('var-entry').nth(1).fill('Manuell');
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(row.getByTestId('value')).toHaveValue('Auto');
    await row.getByTestId('value').selectOption('Manuell');
    await expect(row.getByTestId('value')).toHaveValue('Manuell');

    await row.getByTestId('variable-edit').click();
    await dialog.getByTestId('var-entry').nth(1).fill('Hand');
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(row.getByTestId('value')).toHaveValue('Hand');

    await row.getByTestId('variable-delete').click();
    await page.getByTestId('confirm-delete-yes').click();
    await expect(row).toHaveCount(0);
});

test('an alarm: triggered, listed as an alarm message, acknowledged', async ({page}, info) => {
    const name = `Alarm ${info.project.name}`;
    const key = `alarm-${info.project.name}`;
    const row = page.getByTestId(`variable-${key}`);
    await page.goto('./');
    await expect(page.getByTestId('alarm-badge')).toBeHidden();
    await page.getByRole('tab', {name: 'System variables'}).click();
    await page.getByTestId('variable-new').click();
    const dialog = page.getByTestId('variable-dialog');
    await dialog.getByTestId('var-name').fill(name);
    await dialog.getByTestId('var-type').selectOption('alarm');
    await dialog.getByTestId('var-label-true').fill('ausgelöst');
    await dialog.getByTestId('var-label-false').fill('ruhig');
    await dialog.getByTestId('var-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(page.getByTestId('alarm-messages')).toHaveCount(0);

    await row.getByTestId('value').selectOption({label: 'ausgelöst'});
    const messages = page.getByTestId('alarm-messages');
    await expect(messages.getByTestId(`alarm-${key}`)).toContainText(name);
    await expect(page.getByTestId('alarm-badge')).toHaveText('1 alarm');
    // the count of the alarm messages is a system value
    await expect(page.getByTestId('variable-alarm-messages').getByTestId('value')).toHaveText('1');
    await expect(page.getByTestId('variable-alarm-messages')).toContainText('system value');
    await expect(page.getByTestId('variable-alarm-messages').getByTestId('variable-edit')).toHaveCount(0);

    await messages.getByTestId(`alarm-${key}`).getByTestId('acknowledge').click();
    await expect(page.getByTestId('alarm-messages')).toHaveCount(0);
    await expect(row.getByTestId('value')).toHaveValue('false');
    // again, and "acknowledge all"
    await row.getByTestId('value').selectOption({label: 'ausgelöst'});
    await page.getByTestId('acknowledge-all').click();
    await expect(page.getByTestId('alarm-messages')).toHaveCount(0);

    await row.getByTestId('variable-delete').click();
    await page.getByTestId('confirm-delete-yes').click();
    await expect(row).toHaveCount(0);
});

test('the search, and a variable that programs use cannot be removed', async ({page}) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    // the import spec ran before (alphabetical order): its variables are there
    await page.getByTestId('variable-search').fill('regen');
    const regen = page.getByTestId('variable-regen');
    await expect(regen).toBeVisible();
    await expect(regen.getByTestId('variable-delete')).toBeDisabled();
    // the column Programs has their number, and their names at the pointer
    await expect(regen.getByTestId('variable-users')).not.toHaveText('0');
    await expect(regen.getByTestId('variable-users')).toHaveAttribute('title', /\S/);
    await page.getByTestId('variable-search').fill('gibt es nicht');
    await expect(page.getByText('No variable matches the search.')).toBeVisible();
    // the CCU's counter variables are internal: hidden until asked for
    await page.getByTestId('variable-search').fill('svenergycounter');
    await expect(page.getByTestId('variable-list').locator('tbody tr')).toHaveCount(0);
    await page.getByTestId('show-internal-variables').check();
    await expect(page.getByTestId('variable-svenergycounteroldval-4711')).toBeVisible();
});

test('every ?-icon of the page leads to a section the guide has', async ({page, request}) => {
    await page.goto('./');
    const seen = new Set<string>();
    const collect = async () => {
        for (const href of await page.locator('a.lgc-helplink').evaluateAll((as) => as.map((a) => (a as HTMLAnchorElement).href))) seen.add(href);
    };
    await collect();
    await page.getByTestId('program-search').fill('markise regen');
    await page.getByTestId('program-open').click();
    await collect();
    await page.getByRole('tab', {name: 'System variables'}).click();
    await collect();
    await page.getByTestId('variable-new').click();
    await page.getByTestId('var-type').selectOption('number');
    await collect();
    await page.keyboard.press('Escape');
    await page.getByRole('tab', {name: 'Import'}).click();
    await collect();
    expect(seen.size).toBeGreaterThan(8);
    const guide = await (await request.get('docs/en/')).text();
    for (const href of seen) {
        const anchor = href.split('#')[1];
        expect(guide, href).toContain(`id="${anchor}"`);
    }
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the variables page is German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Systemvariablen'}).click();
        await expect(page.getByTestId('variable-new')).toHaveText('Neue Variable');
        await expect(page.getByTestId('variable-alarm-messages')).toContainText('Alarmmeldungen');
        await expect(page.getByTestId('variable-alarm-messages')).toContainText('Systemwert');
        await page.getByTestId('variable-new').click();
        await expect(page.getByTestId('variable-dialog')).toContainText('Werteliste');
        await page.getByTestId('variable-dialog').getByTestId('var-save').click();
        await expect(page.getByTestId('var-name-error')).toHaveText('Eine Variable braucht einen Namen.');
    });
});
