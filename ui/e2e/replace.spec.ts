import {expect, test} from '@playwright/test';

// "Gerät ersetzen" end to end: a program names a device this system does not have; the
// Settings tab lists it, offers the system's devices in its place, says what stands against a
// device that lacks what the program names, and moves the program to the right one in one
// step. A device that is here is only replaced by one of its type. Every project makes its own
// program and removes it again.

const H = {'X-CCU-Addon-Request': '1'};
const PROJECTS = ['desktop-light', 'desktop-dark', 'phone-light', 'phone-dark'];

test('a device programs name is replaced by another one, after a look at what would change', async ({page, request}, info) => {
    const p = info.project.name;
    const gone = `000FEHLT00000${PROJECTS.indexOf(p) + 1}`;
    const name = `Ersatz ${p}`;
    const key = `ersatz-${p}`;
    await request.delete(`api/v1/programs/${key}`, {headers: H});
    const program = {
        name,
        visible: true,
        operable: true,
        rules: [
            {
                when: {op: 'any', groups: [{op: 'all', lines: [{device: {interface: 'HmIP-RF', address: `${gone}:1`, param: 'STATE'}, op: 'eq', value: true, trigger: 'change'}]}]},
                then: [{kind: 'set', device: {interface: 'HmIP-RF', address: `${gone}:2`, param: 'STATE'}, to: true}],
            },
        ],
    };
    expect((await request.post('api/v1/programs', {headers: H, data: {program}})).status()).toBe(201);

    await page.goto('./');
    await page.getByRole('tab', {name: 'Settings'}).click();
    const section = page.getByTestId('settings-replace');
    await expect(section.getByTestId('help-replace-device')).toHaveAttribute('href', /docs\/en\/#replace-device$/);
    await expect(section.getByTestId('replace-apply')).toBeDisabled();
    await expect(section.getByTestId('replace-to')).toBeDisabled();
    const from = section.getByTestId('replace-from');
    await expect(from.locator(`option[value="HmIP-RF/${gone}"]`)).toHaveText(`HmIP-RF/${gone} · not on this system · named by 1 program or variable`);
    const height = (await section.boundingBox())!.height;
    await from.selectOption(`HmIP-RF/${gone}`);

    // its type is not known: every device of the system is offered - one that lacks what the program names is refused with the reason
    const to = section.getByTestId('replace-to');
    await expect(to.locator('option[value="HmIP-RF/000A1B2C3D4E03"]')).toHaveText('Steckdose Flur (HmIP-RF/000A1B2C3D4E03) · HmIP-PS');
    await to.selectOption('HmIP-RF/000A1B2C3D4E02');
    await expect(section.getByTestId('replace-problem')).toHaveText('HmIP-RF/000A1B2C3D4E02 lacks what programs name: channel 2');
    await expect(section.getByTestId('replace-apply')).toBeDisabled();
    await to.selectOption('HmIP-RF/000A1B2C3D4E01');
    await expect(section.getByTestId('replace-problem')).toHaveText('HmIP-RF/000A1B2C3D4E01 lacks what programs name: 1/STATE, 2/STATE');
    // the right one: what would change, then the question, then done
    await to.selectOption('HmIP-RF/000A1B2C3D4E03');
    await expect(section.getByTestId('replace-would')).toHaveText(`2 places would name the new device, in: ${name}`);
    await expect(section.getByTestId('replace-problem')).toHaveCount(0);
    // (the section has not changed its height through all of this, and keeps the window's width)
    expect(Math.abs((await section.boundingBox())!.height - height)).toBeLessThanOrEqual(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
    await section.getByTestId('replace-apply').click();
    await expect(page.getByTestId('confirm-replace')).toContainText(`Everything that names HmIP-RF/${gone} will name Steckdose Flur (HmIP-RF/000A1B2C3D4E03) · HmIP-PS.`);
    await page.getByTestId('confirm-replace-yes').click();
    await expect(section.getByTestId('replace-preview')).toHaveText('2 places now name the new device.');
    await expect(from.locator(`option[value="HmIP-RF/${gone}"]`)).toHaveCount(0);

    // the program names the new device, and its history says what happened
    const stored = (await (await request.get('api/v1/overview')).json()).programs.find((x: {program: {key: string}}) => x.program.key === key).program;
    expect(stored.rules[0].when.groups[0].lines[0].device.address).toBe('000A1B2C3D4E03:1');
    expect(stored.rules[0].then[0].device.address).toBe('000A1B2C3D4E03:2');
    await page.getByRole('tab', {name: 'Programs'}).click();
    await page.getByTestId('program-search').fill(name);
    await page.getByTestId(`program-${key}`).getByTestId('program-open').click();
    await expect(page.getByTestId('program-detail')).toContainText('Steckdose Flur');
    await page.getByTestId('program-versions').click();
    await expect(page.getByTestId('versions-dialog')).toContainText(`Device HmIP-RF/${gone} replaced by HmIP-RF/000A1B2C3D4E03`);
    await page.keyboard.press('Escape');

    // a device that is here is replaced only by one of its type - and the fake system has no second socket
    await page.getByRole('tab', {name: 'Settings'}).click();
    await expect(from.locator('option[value="HmIP-RF/000A1B2C3D4E03"]')).toContainText('Steckdose Flur (HmIP-RF/000A1B2C3D4E03) · HmIP-PS · named by');
    await from.selectOption('HmIP-RF/000A1B2C3D4E03');
    await expect(to).toBeDisabled();
    await expect(to.locator('option').first()).toHaveText('The system has no other device of the type HmIP-PS.');
    expect((await request.delete(`api/v1/programs/${key}`, {headers: H})).ok()).toBeTruthy();
});

test.describe('in German', () => {
    test.use({locale: 'de-DE'});
    test('the section is German', async ({page}) => {
        await page.goto('./');
        await page.getByRole('tab', {name: 'Einstellungen'}).click();
        const section = page.getByTestId('settings-replace');
        await expect(section).toContainText('Gerät ersetzen');
        await expect(section).toContainText('Das Gerät in den Programmen');
        await expect(section.getByTestId('help-replace-device')).toHaveAttribute('href', /docs\/de\/#replace-device$/);
        await expect(section.getByTestId('replace-apply')).toHaveText('Ersetzen');
    });
});
