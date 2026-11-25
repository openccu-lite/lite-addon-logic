import {expect, test, type Locator, type Page, type TestInfo} from '@playwright/test';

// The lists keep their shape whatever is in them: a third daemon (playwright.config.ts) starts
// with the tree of e2e/table-seed - names, descriptions, units and values far too long for
// their columns beside short ones. Every row of a table has the same height, at desktop width
// as rows and at phone width as cards, in light and dark; a cut text is whole in its tooltip.

const BASE = 'http://127.0.0.1:18303/addons/logic/';
test.use({baseURL: BASE});

const LONG = 'Markise auf der Terrasse einfahren, sobald es regnet oder der Wind zu stark wird, aber nur wenn sie nicht schon eingefahren ist';

/**
 * A picture of the page for a look by eye - only when LOGIC_SHOTS is set. Two CI runs failed in
 * this file alone, with the test's 30 seconds used up and no check failed; the full-page
 * screenshots are the suspect (no other spec takes any, and on the CI's host these tests took
 * seconds where they take a fifth of one elsewhere).
 */
async function shot(page: Page, info: TestInfo, name: string) {
    if (!process.env.LOGIC_SHOTS) return;
    await page.screenshot({path: info.outputPath(name + '.png'), fullPage: true});
    await info.attach(name, {path: info.outputPath(name + '.png'), contentType: 'image/png'});
}

async function heights(rows: Locator): Promise<number[]> {
    return rows.evaluateAll((trs) => trs.map((tr) => Math.round(tr.getBoundingClientRect().height * 100) / 100));
}

/** Every row as high as the first, and nothing in a row wraps or sticks out of its cell. */
async function expectEven(page: Page, table: Locator, count: number) {
    const rows = table.locator('tbody tr');
    await expect(rows).toHaveCount(count);
    const hs = await heights(rows);
    expect(new Set(hs).size, `row heights ${hs.join(', ')}`).toBe(1);
    // the cells of one column are of one height and at one place in every row
    const boxes = await rows.evaluateAll((trs) =>
        trs.map((tr) => {
            const top = tr.getBoundingClientRect().top;
            // (a column that has no room at this width is not there at all)
            return [...tr.children].filter((td) => td.getBoundingClientRect().width > 0).map((td) => {
                const r = td.getBoundingClientRect();
                return [Math.round(r.left), Math.round(r.width), Math.round(r.top - top), Math.round(r.height)].join('/');
            });
        }),
    );
    for (const b of boxes) expect(b).toEqual(boxes[0]);
    // nothing sticks out of its cell: buttons, fields and pills end inside theirs
    const out = await rows.locator('td').evaluateAll((tds) =>
        tds.filter((td) => [...td.children].some((c) => c.getBoundingClientRect().right > td.getBoundingClientRect().right + 0.5)).map((td) => td.textContent?.trim()),
    );
    expect(out).toEqual([]);
    // the table does not make the page scroll sideways
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
}

test('the program list: rows of one height, pills in their own columns, cut texts whole in the tooltip', async ({page}, info) => {
    await page.goto('./');
    await page.getByTestId('show-internal').check();
    const table = page.getByTestId('program-list');
    await expectEven(page, table, 5);

    const long = page.getByTestId('program-lang');
    const name = long.getByTestId('program-open');
    await expect(name).toHaveAttribute('title', new RegExp('^' + LONG + '\\nDiese Beschreibung ist absichtlich so lang'));
    expect(await name.evaluate((el) => el.scrollWidth > el.clientWidth)).toBe(true);
    await expect(long.locator('td').nth(2).locator('.lgc-cut')).toHaveAttribute('title', /^Diese Beschreibung ist absichtlich so lang/);
    await expect(long.locator('td').nth(4).locator('.lgc-cut')).toHaveAttribute('title', /Regen, .*Zeitsteuerung|Regen, .*Time control/);

    // the state and the marks are pills in columns of their own, each of one width
    const state = await table.getByTestId('program-state').evaluateAll((els) => els.map((el) => Math.round(el.getBoundingClientRect().width)));
    expect(new Set(state).size).toBe(1);
    const script = page.getByTestId('program-skript');
    await expect(script.getByTestId('flag-script')).toBeVisible();
    await expect(script.getByTestId('marks-more')).toHaveText('+1');
    await expect(script.getByTestId('marks')).toHaveAttribute('title', /script: Holds a script: scripts do not run here\.\ninternal/);
    await expect(long.getByTestId('flag-no-location')).toBeVisible();
    await expect(page.getByTestId('program-kurz').getByTestId('marks').locator('.lgc-pill')).toHaveCount(0);

    if (info.project.name.startsWith('desktop')) {
        // headers stand over their cells
        const head = await table.locator('thead th').evaluateAll((ths) => ths.filter((th) => th.getBoundingClientRect().width > 0).map((th) => Math.round(th.getBoundingClientRect().left)));
        const cells = await long.locator('td').evaluateAll((tds) => tds.filter((td) => td.getBoundingClientRect().width > 0).map((td) => Math.round(td.getBoundingClientRect().left)));
        expect(head).toEqual(cells);
    }
    await shot(page, info, 'programs');

});

test('the variable list and the alarm messages: rows of one height, values at the right, an error without a taller row', async ({page}, info) => {
    await page.goto('./');
    await page.getByRole('tab', {name: 'System variables'}).click();
    // an alarm that is triggered shows in the alarm messages
    await page.getByTestId('variable-alarm').getByTestId('value').selectOption('true');
    const table = page.getByTestId('variable-list');
    await expect(table.locator('tbody tr').first()).toBeVisible();
    const count = await table.locator('tbody tr').count();
    expect(count).toBeGreaterThanOrEqual(8); // five of the seed and the system values
    await expectEven(page, table, count);
    await expectEven(page, page.getByTestId('alarm-messages').locator('table'), 1);

    const long = page.getByTestId('variable-lange-zahl');
    // the description has its column only where there is room for it, and is in the name's tooltip always
    await expect(long.locator('td').first().locator('.lgc-cut')).toHaveAttribute('title', /^Außentemperatur auf der Nordseite.*\nDiese Beschreibung ist absichtlich so lang/);
    await expect(long.getByTestId('variable-type')).toHaveText('Number');
    await expect(long.getByTestId('flag-logged')).toBeVisible();
    await expect(long.getByTestId('variable-users')).toHaveText('1');
    await expect(long.getByTestId('variable-users')).toHaveAttribute('title', LONG);
    await expect(page.getByTestId('variable-regen').getByTestId('variable-users')).toHaveText('3');
    const types = await table.getByTestId('variable-type').evaluateAll((els) => els.map((el) => Math.round(el.getBoundingClientRect().width)));
    expect(new Set(types).size).toBe(1);

    // a refused value says why in the line above the list; no row changes
    const before = await heights(table.locator('tbody tr'));
    const top = (await table.boundingBox())!.y;
    await expect(page.getByTestId('value-error')).toHaveText('');
    await long.getByTestId('value').fill('99');
    await long.getByTestId('value-set').click();
    await expect(page.getByTestId('value-error')).toHaveText(/^Außentemperatur .*: The largest value is 60\.$/);
    await expect(long.getByTestId('value')).toHaveAttribute('aria-invalid', 'true');
    expect(await heights(table.locator('tbody tr'))).toEqual(before);
    expect((await table.boundingBox())!.y).toBe(top);
    await shot(page, info, 'variables');
    await long.getByTestId('value').fill('21');
    await long.getByTestId('value-set').click();
    await expect(page.getByTestId('value-error')).toHaveText('');
    await expect(long.getByTestId('value')).toHaveAttribute('aria-invalid', 'false');

    if (info.project.name.startsWith('desktop')) {
        // numbers and values end at the right edge of their columns
        const right = async (l: Locator) => l.evaluate((el) => Math.round(el.getBoundingClientRect().right));
        expect(await right(long.getByTestId('variable-users'))).toBe(await right(page.getByTestId('variable-regen').getByTestId('variable-users')));
        expect(await long.getByTestId('value').evaluate((el) => getComputedStyle(el).textAlign)).toBe('right');
    }
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the longer words change nothing: both lists stay even', async ({page}, info) => {
        await page.goto('./');
        await page.getByTestId('show-internal').check();
        await expectEven(page, page.getByTestId('program-list'), 5);
        await expect(page.getByTestId('program-skript').getByTestId('marks')).toHaveAttribute('title', /Script: Enthält ein Script/);
        await page.getByRole('tab', {name: 'Systemvariablen'}).click();
        const table = page.getByTestId('variable-list');
        await expect(table.locator('tbody tr').first()).toBeVisible();
        await expectEven(page, table, await table.locator('tbody tr').count());
        await shot(page, info, 'variables-de');
    });
});
