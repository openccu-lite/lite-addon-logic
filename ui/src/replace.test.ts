import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, beforeEach, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import {candidates, deviceLabel, idOf, replacedText, replacementText, usedLabel, type SystemDevice, type UsedDevice} from './replace.js';
import {messageText} from './versions.js';

const used = (more: Partial<UsedDevice>): UsedDevice => ({interface: 'HmIP-RF', address: 'ALT1', present: false, datapoints: ['1/STATE'], programs: [{key: 'markise', name: 'Markise'}], variables: [], ...more});
const devices: SystemDevice[] = [
    {interface: 'HmIP-RF', address: 'B2', name: 'Zweite', type: 'HmIP-PS'},
    {interface: 'HmIP-RF', address: 'A1', name: 'Erste', type: 'HmIP-PS'},
    {interface: 'HmIP-RF', address: 'C3', type: 'HmIP-SWDO'},
    {interface: 'BidCos-RF', address: 'A1', name: 'Alt', type: 'HM-LC-Sw1-Pl'},
];

describe('replacing a device', () => {
    beforeAll(() => addMessages(MESSAGES));
    beforeEach(() => {
        i18n.language = 'en';
    });

    it('names a device in one line', () => {
        expect(idOf(devices[0]!)).toBe('HmIP-RF/B2');
        expect(deviceLabel(devices[0]!)).toBe('Zweite (HmIP-RF/B2) · HmIP-PS');
        expect(deviceLabel(devices[2]!)).toBe('HmIP-RF/C3 · HmIP-SWDO');
        expect(deviceLabel({interface: 'HmIP-RF', address: 'X'})).toBe('HmIP-RF/X');
        expect(usedLabel(used({name: 'Markise Terrasse', type: 'HmIP-BROLL'}), true)).toBe('Markise Terrasse (HmIP-RF/ALT1) · HmIP-BROLL · not on this system · named by 1 program or variable');
        expect(usedLabel(used({present: true, variables: [{key: 'v', name: 'V'}]}), true)).toBe('HmIP-RF/ALT1 · named by 2 programs and variables');
        // when the system could not be asked nothing is called missing
        expect(usedLabel(used({}), false)).toBe('HmIP-RF/ALT1 · named by 1 program or variable');
        i18n.language = 'de';
        expect(usedLabel(used({}), true)).toBe('HmIP-RF/ALT1 · nicht auf diesem System · von 1 Programm oder Variable genannt');
    });

    it('offers the other devices of the type, or every other one when the type is not known', () => {
        expect(candidates(undefined, devices)).toEqual([]);
        expect(candidates(used({type: 'HmIP-PS'}), devices).map(idOf)).toEqual(['HmIP-RF/A1', 'HmIP-RF/B2']);
        expect(candidates(used({address: 'A1', type: 'HmIP-PS', present: true}), devices).map(idOf)).toEqual(['HmIP-RF/B2']);
        expect(candidates(used({type: 'HmIP-BROLL'}), devices)).toEqual([]);
        expect(candidates(used({}), devices).map(idOf)).toEqual(['BidCos-RF/A1', 'HmIP-RF/A1', 'HmIP-RF/C3', 'HmIP-RF/B2']);
    });

    it('says what a replacement would do and did', () => {
        const r = {programs: [{key: 'markise', name: 'Markise'}], variables: [{key: 'mittel', name: ''}], references: 3, problems: [], can_apply: true};
        expect(replacementText(r)).toBe('3 places would name the new device, in: Markise, mittel');
        expect(replacementText({...r, references: 1, variables: []})).toBe('1 place would name the new device, in: Markise');
        expect(replacedText(r)).toBe('3 places now name the new device.');
        expect(replacedText({...r, references: 1})).toBe('1 place now names the new device.');
        expect(messageText('Device HmIP-RF/ALT1 replaced by HmIP-RF/A1')).toBe('Device HmIP-RF/ALT1 replaced by HmIP-RF/A1');
        i18n.language = 'de';
        expect(messageText('Device HmIP-RF/ALT1 replaced by HmIP-RF/A1')).toBe('Gerät HmIP-RF/ALT1 ersetzt durch HmIP-RF/A1');
    });
});
