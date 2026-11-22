import {expect, test, type Page} from '@playwright/test';

// Every page at 360 px - the narrowest phone - in light and dark and in both languages: the
// page never scrolls sideways and nothing sticks out of the window, on the daemon whose names
// and values are far too long for their columns (e2e/table-seed). The two phone projects run
// it (light and dark); the desktop projects have the same pages at their width in the other
// specs.

const BASE = 'http://127.0.0.1:18303/addons/logic/';
test.use({baseURL: BASE, viewport: {width: 360, height: 780}});

/** Nothing scrolls sideways, and no element that can be seen reaches past the window - unless it lies in a box that scrolls or cuts by itself. */
async function fits(page: Page, where: string) {
    const out = await page.evaluate(() => {
        const width = document.documentElement.clientWidth;
        const bad: string[] = [];
        if (document.documentElement.scrollWidth > width) bad.push(`the page is ${document.documentElement.scrollWidth} px wide`);
        const clipped = (el: Element): boolean => {
            for (let p = el.parentElement; p; p = p.parentElement) {
                const s = getComputedStyle(p);
                if (s.overflowX !== 'visible' || s.overflow !== 'visible') return true;
            }
            return false;
        };
        for (const el of Array.from(document.querySelectorAll('[data-testid], button, input, select, h2, h3, p'))) {
            const r = el.getBoundingClientRect();
            if (r.width === 0 || r.height === 0) continue;
            if ((r.right > width + 1 || r.left < -1) && !clipped(el)) bad.push(`${el.tagName.toLowerCase()}[${el.getAttribute('data-testid') ?? el.textContent?.trim().slice(0, 30)}] reaches from ${Math.round(r.left)} to ${Math.round(r.right)}`);
        }
        return bad.slice(0, 8);
    });
    expect(out, where).toEqual([]);
}

for (const [locale, words] of [
    ['en-GB', {programs: 'Programs', variables: 'System variables', schedule: 'Schedule', triggers: 'Triggers', history: 'History', importTab: 'Import', settings: 'Settings'}],
    ['de-DE', {programs: 'Programme', variables: 'Systemvariablen', schedule: 'Zeitplan', triggers: 'Auslöser', history: 'Verlauf', importTab: 'Import', settings: 'Einstellungen'}],
] as const) {
    test.describe(locale, () => {
        test.use({locale});
        test('every page and dialog fits a window of 360 px', async ({page}, info) => {
            test.skip(!info.project.name.startsWith('phone'), 'the narrow window is the phone projects\' (light and dark)');
            await page.goto('./');
            await expect(page.getByTestId('program-list')).toBeVisible();
            await fits(page, 'the program list');

            // a program with the longest name: its view, the dry run, the versions, the editor
            await page.getByTestId('program-lang').getByTestId('program-open').click();
            await expect(page.getByTestId('program-detail')).toBeVisible();
            await fits(page, 'a program');
            await page.getByTestId('dry-run').click();
            await expect(page.getByTestId('dry-run-dialog').getByTestId('run')).toBeVisible();
            await fits(page, 'the dry run');
            await page.getByTestId('sim-close').click();
            await page.getByTestId('program-versions').click();
            await expect(page.getByTestId('versions-dialog').getByTestId('version').first()).toBeVisible();
            await fits(page, 'the versions');
            await page.getByTestId('versions-close').click();
            await page.getByTestId('program-edit').click();
            await expect(page.getByTestId('program-editor')).toBeVisible();
            await fits(page, 'the editor');
            // a channel's form in the editor (a coloured light has the most fields), and the plain list behind it
            const rule = page.getByTestId('ed-rule').first();
            await rule.getByTestId('action-add').click();
            const added = rule.getByTestId('ed-action').last();
            await added.getByTestId('action-kind').selectOption('channel');
            await added.getByTestId('action-device').click();
            await page.getByTestId('device-picker').getByTestId('picker-search').fill('lichtband');
            await page.getByTestId('device-picker').getByTestId('pick-HmIP-RF/000A1B2C3D4E09:1').getByTestId('pick').click();
            await expect(page.getByTestId('form-dialog').getByTestId('field-hue')).toBeVisible();
            await fits(page, 'a channel\'s form');
            await page.getByTestId('form-dialog').getByTestId('form-all').click();
            await expect(page.getByTestId('values-dialog').getByTestId('value-HUE')).toBeVisible();
            await fits(page, 'the plain list of a channel with a form');
            await page.getByTestId('values-dialog').getByTestId('values-form').click();
            await page.getByTestId('form-dialog').getByTestId('form-save').click();
            // the display composer
            await rule.getByTestId('action-add').click();
            const shows = rule.getByTestId('ed-action').last();
            await shows.getByTestId('action-kind').selectOption('display');
            await shows.getByTestId('action-device').click();
            await page.getByTestId('device-picker').getByTestId('picker-search').fill('wandtaster');
            await page.getByTestId('device-picker').getByTestId('pick-HmIP-RF/000A1B2C3D4E11:1').getByTestId('pick').click();
            await expect(page.getByTestId('display-dialog').getByTestId('display-preview')).toBeVisible();
            await page.getByTestId('display-dialog').getByTestId('display-text').first().fill('Fenster offen');
            await fits(page, 'the display composer');
            await page.getByTestId('display-dialog').getByTestId('display-save').click();
            // the command's form: an argument of each kind, then the shell's line
            await rule.getByTestId('action-add').click();
            await rule.getByTestId('ed-action').last().getByTestId('action-kind').selectOption('command');
            const command = page.getByTestId('command-dialog');
            await command.getByTestId('command-program').fill('/usr/bin/curl');
            for (const kind of ['text', 'variable', 'device']) {
                await command.getByTestId('command-arg-add').click();
                await command.getByTestId('command-arg').last().getByTestId('command-arg-kind').selectOption(kind);
            }
            await fits(page, 'the command\'s form');
            await command.getByTestId('command-shell').check();
            await command.getByTestId('command-line').fill('date');
            await fits(page, 'the command\'s form with a shell');
            await command.locator('button', {hasText: /^(Cancel|Abbrechen)$/}).click();
            await page.getByTestId('editor-cancel').click();
            await page.getByTestId('confirm-discard-yes').click();
            await page.getByTestId('program-back').click();

            await page.getByRole('tab', {name: words.variables}).click();
            await expect(page.getByTestId('page-variables')).toBeVisible();
            await fits(page, 'the variables');
            await page.getByTestId('helper-new').click();
            await expect(page.getByTestId('helper-dialog').getByTestId('helper-device').locator('option')).toHaveCount(3);
            await fits(page, 'the device helper\'s dialog');
            await page.getByTestId('helper-cancel').click();
            await page.getByTestId('variable-lange-liste').getByTestId('variable-edit').click();
            await expect(page.getByTestId('variable-dialog')).toBeVisible();
            await fits(page, 'a variable\'s dialog');
            await page.getByTestId('var-versions').click();
            await expect(page.getByTestId('versions-dialog')).toBeVisible();
            await fits(page, 'a variable\'s versions');
            await page.getByTestId('versions-close').click();

            for (const [tab, id] of [[words.schedule, 'page-schedule'], [words.triggers, 'page-triggers'], [words.history, 'page-history'], [words.importTab, 'page-import'], [words.settings, 'page-settings']] as const) {
                await page.getByRole('tab', {name: tab, exact: true}).click();
                await expect(page.getByTestId(id)).toBeVisible();
                await fits(page, tab);
            }
            // the History with an entry open
            await page.getByRole('tab', {name: words.history, exact: true}).click();
            if (await page.getByTestId('run-head').count()) {
                await page.getByTestId('run-head').first().click();
                await fits(page, 'an entry of the History');
            }
        });
    });
}
