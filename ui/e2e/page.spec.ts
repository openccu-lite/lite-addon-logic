import {expect, test} from '@playwright/test';

import {TOPICS} from '../src/help.js';

// The page against the daemon on the fake system. Opened on its own the page takes the
// browser's language (inside the system's shell it follows the shell).

test('the page loads', async ({page}) => {
    await page.goto('./');
    await expect(page.getByRole('tab', {name: 'System variables'})).toBeVisible();
    await expect(page.getByTestId('version')).toContainText('Version');
});

test('nothing scrolls sideways', async ({page}) => {
    await page.goto('./');
    await expect(page.getByTestId('tabs')).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
});

test('the ?-icon opens the guide\'s section', async ({page, context}) => {
    await page.goto('./');
    const [guide] = await Promise.all([context.waitForEvent('page'), page.getByTestId('help-overview').click()]);
    await guide.waitForLoadState();
    expect(guide.url()).toContain('/addons/logic/docs/en/#overview');
    await expect(guide.locator('#overview')).toHaveText('Overview');
});

test('the guide has the chapter for those who come from a CCU, the quirks, and every section a ?-icon names', async ({page}) => {
    for (const [lang, chapter, quirks, kept] of [['en', 'Coming from a CCU', 'The CCU\'s quirks in plain words', 'A trigger says when the program looks, not what must be true.'], ['de', 'Umstieg von einer CCU', 'Die Eigenheiten der CCU in einfachen Worten', 'Ein Auslöser sagt, wann das Programm hinsieht, nicht, was wahr sein muss.']] as const) {
        await page.goto(`./docs/${lang}/#from-ccu`);
        await expect(page.locator('#from-ccu')).toHaveText(chapter);
        await expect(page.locator('#quirks')).toHaveText(quirks);
        await expect(page.locator('ol > li').filter({hasText: kept})).toHaveCount(1);
        // the steps' table of reasons is a table inside the list
        await expect(page.locator('ol table')).toHaveCount(1);
        for (const topic of TOPICS) await expect(page.locator(`[id="${topic}"]`), `${lang} #${topic}`).toHaveCount(1);
        // every link inside the guide leads somewhere
        const dead = await page.evaluate(() => [...document.querySelectorAll('a[href^="#"]')].map((a) => a.getAttribute('href')!.slice(1)).filter((id) => !document.getElementById(id)));
        expect(dead).toEqual([]);
        // the chapters are listed before the first one, and the list leads to them
        await expect(page.locator('#contents a').nth(1)).toHaveText(chapter);
        await page.locator('#contents a').nth(2).click();
        await expect(page).toHaveURL(/#quirks$/);
        // the guide keeps the window's width, with its tables (at phone width they scroll in themselves)
        expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    }
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the page and the guide are German', async ({page, context}) => {
        await page.goto('./');
        await expect(page.getByRole('tab', {name: 'Systemvariablen'})).toBeVisible();
        const [guide] = await Promise.all([context.waitForEvent('page'), page.getByTestId('help-overview').click()]);
        await guide.waitForLoadState();
        expect(guide.url()).toContain('/addons/logic/docs/de/#overview');
        await expect(guide.locator('#overview')).toHaveText('Überblick');
    });
});

// occulited B-184: openccu-lite links to the import as /addons/logic/#/import (the manifest's
// ui.regadom_import_url) - the page opens on that tab, and the address follows the tabs
test('the address names the tab: #/import opens the import, a tab change keeps the hash in step', async ({page}) => {
    await page.goto('./#/import');
    await expect(page.getByTestId('page-import')).toBeVisible();
    await expect(page.getByRole('tab', {name: 'Import'})).toHaveAttribute('aria-selected', 'true');
    await page.getByRole('tab', {name: 'System variables'}).click();
    await expect(page.getByTestId('page-variables')).toBeVisible();
    await expect(page).toHaveURL(/#\/variables$/);
    await page.reload();
    await expect(page.getByTestId('page-variables')).toBeVisible();
    // a hash change (the link clicked again in the same tab) moves the page too
    await page.evaluate(() => (location.hash = '#/import'));
    await expect(page.getByTestId('page-import')).toBeVisible();
    // a hash the page does not know: the programs; no hash: none is added
    await page.goto('about:blank');
    await page.goto('./#/nothing');
    await expect(page.getByTestId('page-programs')).toBeVisible();
    await page.goto('./');
    await expect(page.getByTestId('page-programs')).toBeVisible();
    await page.getByRole('tab', {name: 'Import'}).click();
    await expect(page.getByTestId('page-import')).toBeVisible();
    expect(new URL(page.url()).hash).toBe('');
});
