import {expect, test} from '@playwright/test';

// The interface process "Logic" end to end: the daemon started by scripts/dev-serve.sh answers
// XML-RPC on 127.0.0.1:39311 as rfd and hmipserver do, and writes its devices' names into the
// (fake) system's metadata. What lite-rpc does with it on a real system is the lab check.
const RPC = 'http://127.0.0.1:39311/';
const SYSTEM = 'http://127.0.0.1:18081';

function call(method: string, ...params: string[]): string {
    const ps = params.map(p => `<param><value><string>${p}</string></value></param>`).join('');
    return `<?xml version="1.0"?><methodCall><methodName>${method}</methodName><params>${ps}</params></methodCall>`;
}

test.describe('the Logic interface', () => {
    test.beforeEach(({}, info) => {
        test.skip(info.project.name !== 'desktop-light', 'no page involved: once is enough');
    });

    test('lists the system values as devices and describes their datapoint', async ({request}) => {
        const list = await (await request.post(RPC, {data: call('listDevices'), headers: {'Content-Type': 'text/xml'}})).text();
        for (const address of ['var-alarm-messages', 'var-alarm-messages:0', 'var-alarm-messages:1', 'var-service-messages:1', 'var-duty-cycle:1']) {
            expect(list).toContain(`<string>${address}</string>`);
        }
        expect(list).toContain('<string>LOGIC-VARIABLE</string>');
        expect(list).toContain('<name>INTERFACE</name><value><string>Logic</string>');

        const desc = await (await request.post(RPC, {data: call('getParamsetDescription', 'var-alarm-messages:1', 'VALUES'), headers: {'Content-Type': 'text/xml'}})).text();
        expect(desc).toContain('<name>VALUE</name>');
        expect(desc).toContain('<name>TYPE</name><value><string>FLOAT</string>');
        // read and event, not write: a system value
        expect(desc).toContain('<name>OPERATIONS</name><value><i4>5</i4>');

        const value = await (await request.post(RPC, {data: call('getValue', 'var-alarm-messages:1', 'VALUE'), headers: {'Content-Type': 'text/xml'}})).text();
        expect(value).toContain('<double>0</double>');

        const refused = await (await request.post(RPC, {data: call('setValue', 'var-alarm-messages:1', 'VALUE', '5'), headers: {'Content-Type': 'text/xml'}})).text();
        expect(refused).toContain('<fault>');

        const unknown = await (await request.post(RPC, {data: call('getValue', 'var-nothing:1', 'VALUE'), headers: {'Content-Type': 'text/xml'}})).text();
        expect(unknown).toContain('<name>faultCode</name><value><i4>-2</i4>');
    });

    test('writes the names of its devices into the system', async ({request}) => {
        await expect(async () => {
            const names = await (await request.get(`${SYSTEM}/dev/names`)).json();
            expect(names['Logic.var-alarm-messages']).toBe('Alarmmeldungen');
            expect(names['Logic.var-alarm-messages:1']).toBe('Alarmmeldungen');
            expect(names['Logic.var-duty-cycle']).toBe('DutyCycle');
        }).toPass({timeout: 10_000});
    });
});
