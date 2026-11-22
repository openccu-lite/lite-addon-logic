import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import {context, type Overview, type Program} from './model.js';
import {kindText, roughly, triggerGroups, type Sources} from './triggers.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const temp = {interface: 'HmIP-RF', address: 'T:1', param: 'ACTUAL_TEMPERATURE'};
const gone = {interface: 'HmIP-RF', address: 'GONE:1', param: 'STATE'};
const prog = (key: string, name: string, active: boolean, lines: Program['rules'][0]['when']): Overview['programs'][0] => ({
    program: {key, name, active, visible: true, operable: true, rules: [{when: lines, then: []}, {else: true, then: []}]},
    problems: [],
    triggers: [],
    script: false,
    time: false,
    state: {key},
});
const o: Overview = {
    programs: [
        prog('luefter', 'Lüfter Bad', true, {op: 'any', groups: [{op: 'all', lines: [
            {device: temp, op: 'gt', value: 21, trigger: 'update'},
            {device: temp, op: 'lt', value: 30, trigger: 'update'},
            {variable: 'regen', op: 'eq', value: true, trigger: 'check'},
            {time: {kind: 'daily', at: '07:00'}, trigger: 'check'},
        ]}]}),
        prog('markise', 'Markise', false, {op: 'any', groups: [{op: 'all', lines: [
            {variable: 'regen', op: 'eq', value: true, trigger: 'change'},
            {variable: 'weg', op: 'eq', value: 1, trigger: 'change'},
            {device: gone, op: 'eq', value: true, trigger: 'change'},
            {device: temp, op: 'gt', value: 25, trigger: 'check'},
        ]}]}),
        {...prog('leer', 'Leer', true, undefined)},
    ],
    variables: [{def: {key: 'regen', name: 'Regen', type: 'bool', visible: true, labels: {true: 'es regnet', false: 'trocken'}}, value: true, time: '2026-11-21T14:02:00+01:00', users: []}],
    alarms: [],
    names: {'HmIP-RF/T:1': 'Thermostat Bad'},
    notices: {},
    settings: {},
    now: '2026-11-21T15:00:00+01:00',
};
const src: Sources = {now: o.now, devices: {'HmIP-RF/T:1/ACTUAL_TEMPERATURE': {value: 21.4, time: '2026-11-21T14:59:00+01:00', every_seconds: 180}, 'HmIP-RF/GONE:1/STATE': {value: false}}};
const all = {query: '', kind: '', program: ''};

describe('what triggers what', () => {
    it('lists every source with the programs that listen and how', () => {
        const groups = triggerGroups(o, src, context(o), all);
        expect(groups.map((g) => g.id)).toEqual(['v:regen', 'v:weg', 'd:HmIP-RF/GONE:1/STATE', 'd:HmIP-RF/T:1/ACTUAL_TEMPERATURE', 't:luefter:4']);
        expect(groups[0]).toMatchObject({kind: 'variable', title: 'Regen', last: '2026-11-21T14:02:00+01:00', value: 'es regnet', every: ''});
        // who triggers comes first
        expect(groups[0]!.listeners).toEqual([
            {programKey: 'markise', program: 'Markise', label: 'trigger when changed', triggers: true, active: false, warn: ''},
            {programKey: 'luefter', program: 'Lüfter Bad', label: 'check only', triggers: false, active: true, warn: ''},
        ]);
        expect(groups[1]).toMatchObject({title: 'weg', last: '', value: ''});
        expect(groups[2]).toMatchObject({kind: 'device', title: 'GONE:1 (HmIP-RF)', detail: 'STATE · GONE:1', last: '', value: 'false', every: ''});
        expect(groups[3]).toMatchObject({title: 'Thermostat Bad', detail: 'ACTUAL_TEMPERATURE · T:1', last: '2026-11-21T14:59:00+01:00', value: '21.4', every: 'reports about every 3 min'});
        // two lines of one program in one mode are one listener; an update on a device is warned about
        expect(groups[3]!.listeners.map((l) => [l.program, l.label, l.warn])).toEqual([['Lüfter Bad', 'trigger when updated', 'runs at every report'], ['Markise', 'check only', '']]);
        // a time module triggers whatever its mode says
        expect(groups[4]).toMatchObject({kind: 'time', title: 'daily, at 07:00', last: ''});
        expect(groups[4]!.listeners[0]).toMatchObject({label: 'check only', triggers: true});
    });
    it('without the engine\'s device states the devices have no last event', () => {
        const g = triggerGroups(o, null, context(o), {...all, kind: 'device'});
        expect(g.map((x) => [x.last, x.value, x.every])).toEqual([['', '', ''], ['', '', '']]);
    });
    it('is filtered by kind, by a search and to one program', () => {
        const ctx = context(o);
        expect(triggerGroups(o, src, ctx, {...all, kind: 'variable'}).map((g) => g.id)).toEqual(['v:regen', 'v:weg']);
        expect(triggerGroups(o, src, ctx, {...all, kind: 'time'}).map((g) => g.id)).toEqual(['t:luefter:4']);
        expect(triggerGroups(o, src, ctx, {...all, query: 'thermostat'}).map((g) => g.id)).toEqual(['d:HmIP-RF/T:1/ACTUAL_TEMPERATURE']);
        expect(triggerGroups(o, src, ctx, {...all, query: 'lüfter'}).map((g) => g.id)).toEqual(['v:regen', 'd:HmIP-RF/T:1/ACTUAL_TEMPERATURE', 't:luefter:4']);
        const own = triggerGroups(o, src, ctx, {...all, program: 'markise'});
        expect(own.map((g) => g.id)).toEqual(['v:regen', 'v:weg', 'd:HmIP-RF/GONE:1/STATE', 'd:HmIP-RF/T:1/ACTUAL_TEMPERATURE']);
        expect(own.every((g) => g.listeners.length === 1 && g.listeners[0]!.programKey === 'markise')).toBe(true);
        expect(triggerGroups(o, src, ctx, {...all, program: 'leer'})).toEqual([]);
    });
    it('says lengths roughly and kinds in a word', () => {
        expect(roughly(45)).toBe('45 s');
        expect(roughly(180)).toBe('3 min');
        expect(roughly(7200)).toBe('2 h');
        expect(['variable', 'device', 'time'].map(kindText)).toEqual(['variable', 'device', 'time module']);
    });
});
