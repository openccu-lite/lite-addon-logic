import {expect, test} from '@playwright/test';

// A webhook and a Wake-on-LAN, end to end: a secret entered in the settings, a program made in
// the editor with the webhook's form and the wake's two fields, run - the fake system receives
// the request with its placeholders filled in and the three magic packets -, the history says
// the status and the time, a failed call is said, a dry run calls nothing. Every project makes
// its own secret, variable and program and removes them again.
const H = {'X-CCU-Addon-Request': '1'};
const FAKE = 'http://127.0.0.1:18081/';
const PROJECTS = ['desktop-light', 'desktop-dark', 'phone-light', 'phone-dark'];

interface Hook {
    method: string;
    url: string;
    header: Record<string, string>;
    body: string;
}

test('a webhook with a secret and a Wake-on-LAN: made in the editor, sent, and said in the history', async ({page, request}, info) => {
    const p = info.project.name;
    const name = `Melden ${p}`;
    const key = `melden-${p}`;
    const secret = `token-${p}`;
    const mac = `AA:BB:CC:00:11:0${PROJECTS.indexOf(p) + 1}`;
    expect((await request.post('api/v1/variables', {headers: H, data: {variable: {name: `Temp ${p}`, type: 'number', visible: true}, value: 21.5}})).status()).toBe(201);
    const hooks = async () => ((await (await request.get(FAKE + 'dev/hooks')).json()) as Hook[]).filter((h) => h.url.startsWith(`/dev/hook/${p}?`));
    const wakes = async () => ((await (await request.get(FAKE + 'dev/wakes')).json()) as string[]).filter((m) => m === mac.toLowerCase());

    // the secret: entered once, never shown again
    await page.goto('./');
    await page.getByRole('tab', {name: 'Settings'}).click();
    const secrets = page.getByTestId('settings-secrets');
    await expect(secrets.getByTestId('help-secrets')).toHaveAttribute('href', /docs\/en\/#secrets$/);
    const messageBox = (await secrets.getByTestId('secrets-message').boundingBox())!;
    await secrets.getByTestId('secret-name').fill('Not A Name');
    await secrets.getByTestId('secret-value').fill('x');
    await secrets.getByTestId('secret-save').click();
    await expect(secrets.getByTestId('secrets-message')).toHaveText('A secret\'s name is made of lower-case letters, digits and hyphens.');
    await secrets.getByTestId('secret-name').fill(secret);
    await secrets.getByTestId('secret-value').fill('');
    await secrets.getByTestId('secret-save').click();
    await expect(secrets.getByTestId('secrets-message')).toHaveText('Enter the secret\'s value.');
    await secrets.getByTestId('secret-value').fill(`geheim & ${p}`);
    await secrets.getByTestId('secret-save').click();
    await expect(secrets.getByTestId('secrets-message')).toHaveText(`The secret ${secret} was saved.`);
    // (the message's line is there whether it says something or not)
    expect((await secrets.getByTestId('secrets-message').boundingBox())!.height).toBe(messageBox.height);
    await expect(secrets.getByTestId(`secret-${secret}`).getByTestId('secret-users')).toHaveText('not used');
    await expect(secrets.getByTestId('secret-value')).toHaveValue('');
    await expect(secrets.getByTestId('secret-value')).toHaveAttribute('type', 'password');
    expect(JSON.stringify(await (await request.get('api/v1/secrets')).json())).not.toContain('geheim');

    // the program: a webhook in its form, a Wake-on-LAN in its row
    await page.getByRole('tab', {name: 'Programs'}).click();
    await page.getByTestId('program-new').click();
    const editor = page.getByTestId('program-editor');
    await editor.getByTestId('ed-name').fill(name);
    await editor.getByTestId('action-add').click();
    const actions = editor.getByTestId('ed-action');
    const first = actions.nth(0);
    await first.getByTestId('action-kind').selectOption('webhook');
    // (the row with something wrong in it is as high as it is afterwards)
    const height = (await first.boundingBox())!.height;
    await expect(first.getByTestId('action-problem')).toHaveText('Enter the address.');
    const dialog = page.getByTestId('webhook-dialog');
    await expect(dialog.getByTestId('help-webhook')).toHaveAttribute('href', /docs\/en\/#webhook$/);
    await expect(dialog.getByTestId('help-webhook-placeholders')).toHaveAttribute('href', /#webhook-placeholders$/);
    await expect(dialog.getByTestId('hook-placeholders')).toContainText('{secret:…}');
    await dialog.getByTestId('hook-save').click();
    await expect(dialog.getByTestId('hook-error')).toHaveText('Enter the address.');
    await dialog.getByTestId('hook-url').fill('ftp://example.org/');
    await expect(dialog.getByTestId('hook-error')).toContainText('The address must be http://… or https://…');
    await dialog.getByTestId('hook-url').fill(`${FAKE}dev/hook/${p}?v={var:temp-${p}}&k={secret:nope}&status=201`);
    await expect(dialog.getByTestId('hook-error')).toHaveText('There is no secret nope: enter it in the settings first.');
    await dialog.getByTestId('hook-url').fill(`${FAKE}dev/hook/${p}?v={var:temp-${p}}&status=201`);
    await dialog.getByTestId('hook-header-add').click();
    await dialog.getByTestId('hook-header-name').fill('X-Token');
    await dialog.getByTestId('hook-header-value').fill(`{secret:${secret}}`);
    // a GET has no body; a POST has one
    await dialog.getByTestId('hook-method').selectOption('GET');
    await expect(dialog.getByTestId('hook-body')).toBeDisabled();
    await dialog.getByTestId('hook-method').selectOption('POST');
    await dialog.getByTestId('hook-body').fill('{"program": "{program}", "by": "{trigger}", "keep": {"x": 1}}');
    await expect(dialog.getByTestId('hook-error')).toHaveText('');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await dialog.getByTestId('hook-save').click();
    await expect(dialog).toHaveCount(0);
    await expect(first.getByTestId('action-webhook')).toContainText(`POST ${FAKE}dev/hook/${p}`);
    await expect(first.getByTestId('action-problem')).toHaveText('');
    expect((await first.boundingBox())!.height).toBe(height);

    await editor.getByTestId('action-add').click();
    const second = actions.nth(1);
    await second.getByTestId('action-kind').selectOption('wake');
    await expect(second.getByTestId('action-problem')).toHaveText('Enter the hardware address: AA:BB:CC:DD:EE:FF.');
    const wakeHeight = (await second.boundingBox())!.height;
    await second.getByTestId('action-mac').fill(mac.toLowerCase().replaceAll(':', '-'));
    await second.getByTestId('action-broadcast').fill('127.0.0');
    await expect(second.getByTestId('action-problem')).toContainText('The broadcast address is an IPv4 address');
    await second.getByTestId('action-broadcast').fill('127.0.0.1');
    await expect(second.getByTestId('action-problem')).toHaveText('');
    expect((await second.boundingBox())!.height).toBe(wakeHeight);
    await editor.getByTestId('editor-save').click();

    // the program says both in words
    const detail = page.getByTestId('program-detail');
    await expect(detail.getByTestId('action').nth(0)).toContainText(`POST ${FAKE}dev/hook/${p}`);
    await expect(detail.getByTestId('action').nth(0)).toContainText('call with a body (JSON)');
    await expect(detail.getByTestId('action').nth(1)).toContainText(`${mac} wake up, through 127.0.0.1`);
    const stored = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(stored.rules[0].then[1]).toEqual({kind: 'wake', wake: {mac, broadcast: '127.0.0.1'}});

    // a dry run calls nothing
    const dry = await (await request.post(`api/v1/programs/${key}/dry-run`, {headers: H, data: {}})).json();
    expect(dry.steps.map((s: {what: string}) => s.what)).toEqual(['called', 'called']);
    expect(await hooks()).toHaveLength(0);

    // run: the request arrives with its placeholders filled in, and three magic packets
    await detail.getByTestId('run-then').click();
    await expect.poll(async () => (await hooks()).length).toBe(1);
    const got = (await hooks())[0]!;
    expect(got.method).toBe('POST');
    expect(got.url).toBe(`/dev/hook/${p}?v=21.5&status=201`);
    expect(got.header['X-Token']).toBe(`geheim & ${p}`);
    expect(got.header['Content-Type']).toBe('application/json');
    expect(got.header['User-Agent']).toMatch(/^lite-addon-logic\//);
    expect(JSON.parse(got.body)).toEqual({program: name, by: 'user', keep: {x: 1}});
    await expect.poll(async () => (await wakes()).length).toBe(3);

    // the history: the status and the time of the call, the packets sent
    await page.getByRole('tab', {name: 'History'}).click();
    await page.getByTestId('run-search').fill(name);
    const runs = page.getByTestId('page-history').getByTestId('run-list').getByTestId('run');
    await expect(runs).toHaveCount(1);
    await runs.nth(0).getByTestId('run-head').click();
    const steps = runs.nth(0).getByTestId('run-step');
    await expect(steps.nth(0).getByTestId('run-status')).toContainText(/answered 201 after \d+ ms/);
    await expect(steps.nth(1).getByTestId('run-status')).toContainText('sent ');

    // a server that says no: the call failed, with its status, and the program counts it
    stored.rules[0].then[0].webhook.url = `${FAKE}dev/hook/${p}?status=500`;
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: stored}})).ok()).toBeTruthy();
    expect((await request.post(`api/v1/programs/${key}/run`, {headers: H, data: {mode: 'then'}})).ok()).toBeTruthy();
    await expect(runs).toHaveCount(2);
    await runs.nth(0).getByTestId('run-head').click();
    await expect(runs.nth(0).getByTestId('run-step').nth(0).getByTestId('run-status')).toContainText('failed: HTTP 500');
    // the secret never reached the trace
    expect(JSON.stringify(await (await request.get(`api/v1/runs?program=${key}`)).json())).not.toContain('geheim');

    // the secret is in use: said, and it cannot be removed; a secret a program names that is not entered is said
    stored.rules[0].then[0].webhook.headers.push({name: 'X-Other', value: `{secret:fehlt-${p}}`});
    expect((await request.put(`api/v1/programs/${key}`, {headers: H, data: {program: stored}})).ok()).toBeTruthy();
    await page.getByRole('tab', {name: 'Settings'}).click();
    const row = secrets.getByTestId(`secret-${secret}`);
    await expect(row.getByTestId('secret-users')).toHaveText(`used by ${name}`);
    await expect(row.getByTestId('secret-remove')).toBeDisabled();
    await expect(secrets.getByTestId('secrets-message')).toContainText(`fehlt-${p}`);
    expect((await request.delete(`api/v1/secrets/${secret}`, {headers: H})).status()).toBe(409);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);

    // gone again: the program, then the secret (with a question), the variable
    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(row.getByTestId('secret-users')).toHaveText('not used');
    const rowHeight = (await row.boundingBox())!.height;
    await row.getByTestId('secret-remove').click();
    await expect(page.getByTestId('confirm-secret')).toContainText(secret);
    await page.getByTestId('confirm-secret-yes').click();
    await expect(row).toHaveCount(0);
    await expect(secrets.getByTestId('secrets-message')).toHaveText(`The secret ${secret} was removed.`);
    if (await secrets.getByTestId('secrets-none').count()) expect((await secrets.getByTestId('secrets-none').boundingBox())!.height).toBe(rowHeight);
    expect((await request.delete(`api/v1/variables/temp-${p}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the secrets and the activities speak German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Einstellungen'}).click();
        const secrets = page.getByTestId('settings-secrets');
        await expect(secrets.getByRole('heading')).toHaveText('Geheimnisse');
        await expect(secrets.getByTestId('help-secrets')).toHaveAttribute('href', /docs\/de\/#secrets$/);
        await expect(secrets.getByTestId('secret-save')).toHaveText('Geheimnis speichern');
        await page.getByRole('tab', {name: 'Programme'}).click();
        await page.getByTestId('program-new').click();
        const editor = page.getByTestId('program-editor');
        await editor.getByTestId('action-add').click();
        await editor.getByTestId('ed-action').nth(0).getByTestId('action-kind').selectOption('webhook');
        const dialog = page.getByTestId('webhook-dialog');
        await expect(dialog.getByTestId('hook-header-add')).toHaveText('Kopfzeile hinzufügen');
        // header lines come and go, the body's kind is the POST's, the certificate check is a tick
        await dialog.getByTestId('hook-header-add').click();
        await dialog.getByTestId('hook-header-add').click();
        await expect(dialog.getByTestId('hook-header')).toHaveCount(2);
        await dialog.getByTestId('hook-header-name').nth(1).fill('X-Zwei');
        await dialog.getByTestId('hook-header-remove').nth(0).click();
        await expect(dialog.getByTestId('hook-header')).toHaveCount(1);
        await expect(dialog.getByTestId('hook-header-name')).toHaveValue('X-Zwei');
        await dialog.getByTestId('hook-header-remove').click();
        await expect(dialog.getByTestId('hook-header')).toHaveCount(0);
        await dialog.getByTestId('hook-method').selectOption('GET');
        await expect(dialog.getByTestId('hook-body-type')).toBeDisabled();
        await dialog.getByTestId('hook-method').selectOption('POST');
        await dialog.getByTestId('hook-body-type').selectOption('text');
        await expect(dialog.getByTestId('hook-body-type')).toHaveValue('text');
        await expect(dialog.getByTestId('hook-insecure')).not.toBeChecked();
        await dialog.getByTestId('hook-insecure').check();
        await expect(dialog.getByTestId('hook-insecure')).toBeChecked();
        await dialog.getByTestId('hook-save').click();
        await expect(dialog.getByTestId('hook-error')).toHaveText('Die Adresse eingeben.');
        await dialog.getByRole('button', {name: 'Abbrechen'}).click();
        await expect(editor.getByTestId('ed-action').nth(0).getByTestId('action-webhook')).toHaveText('Adresse festlegen…');
        await editor.getByTestId('editor-cancel').click();
        await page.getByTestId('confirm-discard-yes').click();
    });
});
