import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, beforeEach, describe, expect, it} from 'vitest';

import {HELPER_KINDS, baseName, candidateId, candidateLabel, hasDays, helperNames, helperProblem, helperTitle, kindNoun, leadRole, type Candidate} from './helpers.js';
import {MESSAGES} from './messages.js';

const psm: Candidate = {kind: 'energy', channel: 'HmIP-RF/000A1B2C3D4E12:1', name: 'Waschmaschine', type: 'HmIP-PSM', unit: 'Wh', roles: ['total', 'today', 'yesterday']};
const wds: Candidate = {kind: 'rain-hm', channel: 'BidCos-RF/OEQ0000003:1', type: 'HM-WDS100-C6-O', roles: ['today', 'yesterday']};

describe('device helpers', () => {
    beforeAll(() => addMessages(MESSAGES));
    beforeEach(() => (i18n.language = 'de'));
    it('names a candidate by its channel, what it counts and its device', () => {
        expect(candidateLabel(psm)).toBe('Waschmaschine – Energie (HmIP-PSM)');
        expect(candidateLabel(wds)).toBe('OEQ0000003:1 – Regen (HM-WDS100-C6-O)');
        expect(candidateLabel({...wds, channel: 'x'})).toBe('x – Regen (HM-WDS100-C6-O)');
        expect(candidateId(psm)).toBe('energy@HmIP-RF/000A1B2C3D4E12:1');
        expect(baseName(psm)).toBe('Waschmaschine');
        expect(baseName(wds)).toBe('OEQ0000003:1');
        expect(baseName({...wds, channel: 'x'})).toBe('x');
    });
    it('names the variables of a new helper', () => {
        expect(helperNames(psm, ' Waschmaschine ', true)).toEqual({total: 'Energiezähler Waschmaschine', today: 'Energie heute Waschmaschine', yesterday: 'Energie gestern Waschmaschine'});
        expect(helperNames(psm, 'Waschmaschine', false)).toEqual({total: 'Energiezähler Waschmaschine'});
        // the classic rain sensor has no total: today's amount leads
        expect(helperNames(wds, 'Wetter', true)).toEqual({today: 'Regen heute Wetter', yesterday: 'Regen gestern Wetter'});
        expect(helperNames(wds, 'Wetter', false)).toEqual({today: 'Regen heute Wetter'});
        expect(leadRole(wds.roles)).toBe('today');
        expect(leadRole([])).toBe('total');
        expect(hasDays(psm)).toBe(true);
        expect(hasDays({...psm, roles: ['total']})).toBe(false);
        i18n.language = 'en';
        expect(helperNames(psm, 'Washer', true)).toEqual({total: 'Energy counter Washer', today: 'Energy today Washer', yesterday: 'Energy yesterday Washer'});
    });
    it('has words for every kind and role', () => {
        for (const kind of HELPER_KINDS) {
            expect(MESSAGES['helpernoun.' + kind], kind).toBeTruthy();
            expect(kindNoun(kind)).not.toContain('helpernoun.');
            if (kind !== 'rain-hm') expect(MESSAGES['helpertotal.' + kind], kind).toBeTruthy();
        }
        for (const role of ['total', 'today', 'yesterday']) expect(MESSAGES['role.' + role]).toBeTruthy();
    });
    it('says what is wrong with the input', () => {
        expect(helperProblem(undefined, 'x')).toBe('Ein Gerät auswählen.');
        expect(helperProblem(psm, '  ')).toBe('Der Helfer braucht einen Namen.');
        expect(helperProblem(psm, 'x'.repeat(110))).toBe('Der Name ist zu lang.');
        expect(helperProblem(psm, 'Waschmaschine')).toBe('');
    });
    it('says what a helper\'s variable is', () => {
        expect(helperTitle({kind: 'energy', channel: psm.channel, role: 'total'}, 'Waschmaschine')).toBe(
            'Von einem Gerätehelfer gepflegt: Energie, Gesamtstand, an Waschmaschine. Den Wert zu setzen setzt den Zählerstand; von dort wird weitergezählt.',
        );
        expect(helperTitle({kind: 'rain', channel: 'x', role: 'today'}, 'Wetterstation')).toBe('Von einem Gerätehelfer gepflegt: Regen, heute, an Wetterstation. Niemand sonst schreibt ihn.');
    });
});
