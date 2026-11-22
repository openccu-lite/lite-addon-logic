import {expect, test} from '@playwright/test';

// What the page says when the data is not as it should be - on a daemon of its own
// (playwright.config.ts), whose seed (e2e/state-seed) holds a file that is no program and a
// program that names a variable there is none of: the banner for the file with the way to the
// guide, and a program marked faulty that cannot be switched on or run.

const BASE = 'http://127.0.0.1:18304/addons/logic/';
test.use({baseURL: BASE});

test('a file that cannot be read is named in a banner; a program with a broken reference is faulty and does not run', async ({page}) => {
    await page.goto('./');
    const banner = page.getByTestId('bad-file');
    await expect(banner).toHaveCount(1);
    await expect(banner).toContainText('programs/kaputt.yaml');
    await expect(banner.getByTestId('help-file-rules')).toHaveAttribute('href', /docs\/en\/#file-rules$/);

    const row = page.getByTestId('program-verwaist');
    await expect(row.getByTestId('program-state')).toHaveText('faulty');
    await expect(row.getByTestId('program-active')).toBeDisabled();
    await row.getByTestId('program-open').click();
    await expect(page.getByTestId('program-problem')).toContainText('fehlt');
    await expect(page.getByTestId('detail-active')).toBeDisabled();
    await expect(page.getByTestId('run-check')).toBeDisabled();
    await expect(page.getByTestId('run-then')).toBeDisabled();
    await expect(page.getByTestId('dry-run')).toBeDisabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});
