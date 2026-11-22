import {expect, test} from '@playwright/test';

// The history's remote end to end, against the git host of the fake system (/dev/git/, git's
// HTTP protocol with the token "geheim"): set up with a wrong token and the right one, pushed
// by itself, fetched; what somebody else pushed is shown file by file and taken over, a file
// the addon cannot read stops the take-over; the form's two kinds of address; removed again.
// The four projects share the daemon: each has a repository of its own and leaves no remote.

const FAKE = 'http://127.0.0.1:18081/';
const H = {'X-CCU-Addon-Request': '1'};
test.describe.configure({mode: 'serial'});

test('a remote is set up, pushed to, fetched from and taken over', async ({page, request}, info) => {
    const p = info.project.name;
    const repo = `haus-${p}`;
    const key = `fern-${p}`;
    expect((await request.put(`${FAKE}dev/git/${repo}`)).ok()).toBeTruthy();
    await request.delete(`api/v1/remote`, {headers: H});
    await request.delete(`api/v1/variables/${key}`, {headers: H});
    // something to push, whatever ran before
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Lokal ${p}`, type: 'bool', visible: true}}})).status()).toBe(201);

    await page.goto('./');
    await page.getByRole('tab', {name: 'Settings'}).click();
    const section = page.getByTestId('settings-remote');
    await expect(section.getByTestId('help-remote')).toHaveAttribute('href', /docs\/en\/#remote$/);
    await expect(section.getByTestId('remote-state')).toHaveText('No remote is set: the history stays on this system.');
    for (const id of ['remote-push', 'remote-fetch', 'remote-remove']) await expect(section.getByTestId(id)).toBeDisabled();

    // what the form refuses before anything is sent
    await section.getByTestId('remote-save').click();
    await expect(section.getByTestId('remote-message')).toHaveText('Enter the address of the repository.');
    await section.getByTestId('remote-url').fill('http://git.example.org/x.git');
    await section.getByTestId('remote-save').click();
    await expect(section.getByTestId('remote-message')).toContainText('http:// would send the token unprotected');

    // an address over ssh shows the key's place instead of the token's - and nothing moves
    const top = async () => (await section.getByTestId('remote-save').boundingBox())!.y;
    const before = await top();
    await section.getByTestId('remote-url').fill('git@git.example.org:me/home.git');
    await expect(section.getByTestId('remote-public-key')).toHaveAttribute('placeholder', 'made when the remote is saved');
    await expect(section.getByTestId('remote-host-key')).toHaveText('remembered at the first contact');
    await expect(section.getByTestId('remote-new-key')).toBeDisabled();
    await expect(section.getByTestId('remote-forget-host')).toBeDisabled();
    await expect(section.getByTestId('remote-token')).toHaveCount(0);
    expect(Math.abs((await top()) - before)).toBeLessThanOrEqual(1);

    // a wrong token: saved, and the push says whose fault it is - here and at the top of the page
    await section.getByTestId('remote-url').fill(`${FAKE}dev/git/${repo}`);
    await section.getByTestId('remote-branch').fill('zweig');
    await section.getByTestId('remote-username').fill('ich');
    await section.getByTestId('remote-token').fill('falsch');
    await section.getByTestId('remote-save').click();
    await expect(section.getByTestId('remote-state')).toHaveText('The remote does not accept the token.', {timeout: 15_000});
    await expect(section.getByTestId('remote-token')).toHaveValue('');
    await expect(section.getByTestId('remote-token')).toHaveAttribute('placeholder', 'unchanged');
    await page.reload();
    await expect(page.getByTestId('remote-notice')).toHaveText('The history could not be pushed to its remote. The Settings tab says why.');
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(section.getByTestId('remote-url')).toHaveValue(`${FAKE}dev/git/${repo}`);
    await expect(section.getByTestId('remote-branch')).toHaveValue('zweig');
    await expect(section.getByTestId('remote-username')).toHaveValue('ich');
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-state')).toHaveText('The remote does not accept the token.');
    await expect(section.getByTestId('remote-preview')).toHaveCount(0);

    // the right one, on the branch "main": pushed by itself
    await section.getByTestId('remote-branch').fill('');
    await section.getByTestId('remote-token').fill('geheim');
    await section.getByTestId('remote-save').click();
    await expect(section.getByTestId('remote-state')).toContainText('In step with the remote; last pushed', {timeout: 15_000});
    await page.reload();
    await expect(page.getByTestId('remote-notice')).toHaveCount(0);
    await page.getByRole('tab', {name: 'Settings'}).click();
    await section.getByTestId('remote-push').click();
    await expect(section.getByTestId('remote-state')).toContainText('In step with the remote');
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-relation')).toHaveText('The remote and this system have the same history.');
    await expect(section.getByTestId('remote-pull')).toHaveCount(0);
    await section.getByTestId('remote-preview-close').click();
    await expect(section.getByTestId('remote-preview')).toHaveCount(0);

    // somebody else pushes a variable: the fetch lists the entry and the file, "Take over" brings it
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: `variables/${key}.yaml`, text: `name: Fern ${p}\ntype: bool\n`, message: 'from elsewhere'}})).ok()).toBeTruthy();
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-relation')).toHaveText('The remote has 1 entry this system lacks.');
    await expect(section.getByTestId('remote-entries')).toContainText('Somebody else · from elsewhere');
    await expect(section.getByTestId('remote-changes')).toBeVisible();
    const change = section.getByTestId('remote-change');
    await expect(change).toHaveCount(1);
    await expect(change).toContainText('new');
    await expect(change).toContainText(`System variable Fern ${p}`);
    // (the page keeps its width with the list open)
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    expect(await section.locator('button').evaluateAll((els) => els.filter((e) => e.getBoundingClientRect().right > document.documentElement.clientWidth + 1).map((e) => e.textContent))).toEqual([]);
    await section.getByTestId('remote-pull').click();
    await expect(page.getByTestId('confirm-remote')).toContainText('Nothing runs because of it, values stay.');
    await page.getByTestId('confirm-remote-yes').click();
    await expect(section.getByTestId('remote-message')).toHaveText('1 file was taken over.');
    await expect(section.getByTestId('remote-preview')).toHaveCount(0);
    await page.getByRole('tab', {name: 'System variables'}).click();
    await expect(page.getByTestId(`variable-${key}`)).toContainText(`Fern ${p}`);

    // a file the addon cannot read: marked, and not taken over
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: `programs/kaputt-${p}.yaml`, text: 'name: Kaputt\nrules: nein\n', message: 'broken'}})).ok()).toBeTruthy();
    // ... and a program that names a variable there is none of
    const orphan = `name: Verwaist ${p}\nrules:\n  - when:\n      any:\n        - all:\n            - variable: gibt-es-nicht\n              is: true\n              trigger: change\n    then: []\n`;
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: `programs/verwaist-${p}.yaml`, text: orphan, message: 'orphan'}})).ok()).toBeTruthy();
    await page.getByRole('tab', {name: 'Settings'}).click();
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-change')).toHaveCount(2);
    await expect(section.getByTestId('remote-change').first()).toContainText('not possible');
    await expect(section.getByTestId('remote-change').first()).toContainText(`kaputt-${p}.yaml`);
    await expect(section.getByTestId('remote-change').nth(1)).toContainText(`Program Verwaist ${p}`);
    await expect(section.getByTestId('remote-problem')).toContainText(`Verwaist ${p}: `);
    await expect(section.getByTestId('remote-pull')).toBeDisabled();
    await expect(section.getByTestId('remote-cannot')).toHaveText('Put right at the remote what is marked, then fetch again.');
    // put right at the remote: fetched again, taken over (nothing is left to change)
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: `programs/kaputt-${p}.yaml`, text: '', message: 'put right'}})).ok()).toBeTruthy();
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: `programs/verwaist-${p}.yaml`, text: '', message: 'put right, too'}})).ok()).toBeTruthy();
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-relation')).toHaveText('The remote has 4 entries this system lacks.');
    await expect(section.getByTestId('remote-change')).toHaveCount(0);
    await section.getByTestId('remote-pull').click();
    await page.getByTestId('confirm-remote-yes').click();
    await expect(section.getByTestId('remote-message')).toHaveText('0 files were taken over.');

    // a change here while the remote went on: the push is refused, the fetch says the two diverged
    expect((await request.post(`${FAKE}dev/git/${repo}/commit`, {data: {path: 'notes.txt', text: 'elsewhere\n', message: 'their way'}})).ok()).toBeTruthy();
    expect((await request.delete(`api/v1/variables/${key}`, {headers: H})).ok()).toBeTruthy();
    await page.reload();
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(section.getByTestId('remote-state')).toContainText('The remote did not take the push', {timeout: 15_000});
    await section.getByTestId('remote-fetch').click();
    await expect(section.getByTestId('remote-relation')).toContainText('each gone their own way');
    await expect(section.getByTestId('remote-pull')).toHaveCount(0);
    await section.getByTestId('remote-preview-close').click();

    // removed: nothing is pushed any more
    await section.getByTestId('remote-remove').click();
    await expect(page.getByTestId('confirm-remote')).toContainText('The history is no longer pushed.');
    await page.getByTestId('confirm-remote-yes').click();
    await expect(section.getByTestId('remote-message')).toHaveText('The remote was removed. The history stays on this system.');
    await expect(section.getByTestId('remote-url')).toHaveValue('');
    await expect(section.getByTestId('remote-fetch')).toBeDisabled();
    await request.delete(`api/v1/variables/lokal-${p}`, {headers: H});
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the remote\'s section is German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Einstellungen'}).click();
        const section = page.getByTestId('settings-remote');
        await expect(section).toContainText('Remote für den Verlauf');
        await expect(section.getByTestId('help-remote')).toHaveAttribute('href', /docs\/de\/#remote$/);
        await expect(section.getByTestId('remote-state')).toHaveText('Kein Remote eingerichtet: der Verlauf bleibt auf diesem System.');
        await expect(section.getByTestId('remote-fetch')).toHaveText('Abholen…');
        await section.getByTestId('remote-url').fill('ssh://git@git.example.org/me/home.git');
        await expect(section.getByTestId('remote-host-key')).toHaveText('wird beim ersten Kontakt gemerkt');
        await section.getByTestId('remote-url').fill('ftp://x/y');
        await section.getByTestId('remote-save').click();
        await expect(section.getByTestId('remote-message')).toHaveText('Die Adresse muss https://…, ssh://… oder benutzer@host:pfad sein.');
    });
});
