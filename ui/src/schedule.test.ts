import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import type {Context} from './model.js';
import {offsetNote, offText, progress, relative, scheduleRows, type Schedule} from './schedule.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const NOW = '2026-11-21T12:00:00+01:00';
const ctx: Context = {variables: {hinweis: {key: 'hinweis', name: 'Hinweis', type: 'string', visible: true}}, programs: {nacht: 'Nachtabsenkung', markise: 'Markise'}, names: {'HmIP-RF/A:1': 'Rollladen'}};

describe('how far away', () => {
    it('is said in the largest two units', () => {
        expect(relative(NOW, '2026-11-21T12:00:12+01:00')).toBe('in 12 s');
        expect(relative(NOW, '2026-11-21T12:04:59+01:00')).toBe('in 4 min');
        expect(relative(NOW, '2026-11-21T14:14:00+01:00')).toBe('in 2 h 14 min');
        expect(relative(NOW, '2026-11-21T14:00:30+01:00')).toBe('in 2 h');
        expect(relative(NOW, '2026-11-24T14:00:00+01:00')).toBe('in 3 d 2 h');
        expect(relative(NOW, '2026-11-23T12:10:00+01:00')).toBe('in 2 d');
        expect(relative(NOW, '2026-11-21T11:49:00+01:00')).toBe('11 min ago');
        expect(relative(NOW, NOW)).toBe('now');
        // the same instant in another offset is the same instant
        expect(relative(NOW, '2026-11-21T11:00:00Z')).toBe('now');
        expect(relative(NOW, undefined)).toBe('');
        expect(relative(NOW, 'soon')).toBe('');
    });
    it('as a part of the way', () => {
        expect(progress('2026-11-21T11:00:00+01:00', '2026-11-21T13:00:00+01:00', NOW)).toBe(0.5);
        expect(progress('2026-11-21T12:30:00+01:00', '2026-11-21T13:00:00+01:00', NOW)).toBe(0);
        expect(progress('2026-11-21T10:00:00+01:00', '2026-11-21T11:00:00+01:00', NOW)).toBe(1);
        expect(progress(undefined, '2026-11-21T13:00:00+01:00', NOW)).toBeNull();
        expect(progress('2026-11-21T11:00:00+01:00', undefined, NOW)).toBeNull();
        expect(progress('2026-11-21T11:00:00+01:00', '2026-11-21T13:00:00+01:00', '')).toBeNull();
        expect(progress('2026-11-21T13:00:00+01:00', '2026-11-21T13:00:00+01:00', NOW)).toBeNull();
        // a state file from before the scheduling moment was kept: the zero time
        expect(progress('0001-01-01T00:00:00Z', '2026-11-21T13:00:00+01:00', NOW)).toBeNull();
    });
});

describe('the offset of a time on the other side of a clock change', () => {
    it('is named, and only then', () => {
        expect(offsetNote('2027-03-28T03:00:00+02:00', NOW)).toBe('UTC+2');
        expect(offsetNote('2026-11-22T06:30:00+01:00', NOW)).toBe('');
        expect(offsetNote('2026-11-22T06:30:00Z', NOW)).toBe('UTC');
        expect(offsetNote('2026-11-22T06:30:00-03:30', NOW)).toBe('UTC-3:30');
        expect(offsetNote('2026-11-22T06:30:00+01:00', '2026-06-01T12:00:00+02:00')).toBe('UTC+1');
        expect(offsetNote(undefined, NOW)).toBe('');
        expect(offsetNote('2026-11-22T06:30:00', NOW)).toBe('');
    });
});

describe('the rows', () => {
    const s: Schedule = {
        now: NOW,
        timers: [
            {program: 'nacht', line: 1, time: {kind: 'daily', at: '22:00', duration: '8h'}, armed: true, active: false, next: {at: '2026-11-21T22:00:00+01:00', kind: 'begin'}, last: {at: '2026-11-21T06:00:00+01:00', kind: 'end'}},
            {program: 'weg', line: 2, time: {kind: 'daily', astro: 'sunset'}, armed: false, off: 'no-location', active: false},
            {program: 'markise', line: 3, time: {kind: 'daily', at: '12:30'}, armed: true, active: true, next: {at: '2026-11-21T12:30:00+01:00', kind: 'point'}},
            {program: 'markise', line: 1, time: {kind: 'daily', at: '12:30'}, armed: true, active: false, next: {at: '2026-11-21T12:30:00+01:00', kind: 'point'}},
            {program: 'markise', line: 4, time: {kind: 'daily', at: '09:00'}, armed: false, off: 'inactive', active: false},
        ],
        pending: [
            {id: 7, due: '2026-11-21T12:00:30+01:00', set: '2026-11-21T11:59:30+01:00', program: 'markise', rule: 0, by: 'user', action: {kind: 'variable', variable: 'hinweis', to: 'x', after: '1min'}},
            {id: 8, due: '2026-11-21T13:00:00+01:00', set: '2026-11-21T12:00:00+01:00', program: 'fort', rule: 0, by: 'user', action: {kind: 'set', device: {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'}, to: 0, after: '1h'}},
        ],
    };
    it('are what fires next first, what is not armed last', () => {
        const rows = scheduleRows(s, ctx, 'all', '');
        expect(rows.map((r) => r.id)).toEqual(['p-7', 't-markise-1', 't-markise-3', 'p-8', 't-nacht-1', 't-markise-4', 't-weg-2']);
        expect(rows[0]).toMatchObject({kind: 'delay', what: 'delay of 1 min', program: 'Markise', target: 'Hinweis', next: '2026-11-21T12:00:30+01:00', progress: 0.5, pending: 7, off: ''});
        expect(rows[3]).toMatchObject({program: 'fort', target: 'Rollladen LEVEL', progress: 0});
        expect(rows[4]).toMatchObject({kind: 'time', what: 'daily, from 22:00, for 8 h', program: 'Nachtabsenkung', nextNote: 'begin', lastNote: 'end', last: '2026-11-21T06:00:00+01:00', progress: 0.375});
        expect(rows[2]).toMatchObject({active: true, nextNote: '', progress: null});
        expect(rows[5]).toMatchObject({off: 'the program is inactive', next: undefined});
        expect(rows[6]).toMatchObject({program: 'weg', off: 'the location of the system is not set'});
    });
    it('list the lines that count how long their comparison has held', () => {
        const of = {variable: 'hinweis', op: 'eq', value: 'x', for: '10min', trigger: 'change'};
        const dev = {device: {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'}, op: 'gt', value: 0.5, for: '1h', trigger: 'check'};
        const withHolds: Schedule = {...s, timers: [], pending: [], holds: [
            {program: 'markise', line: 2, of, for: '10min', since: '2026-11-21T11:56:00+01:00', due: '2026-11-21T12:06:00+01:00', true: false},
            {program: 'nacht', line: 1, of: dev, for: '1h', since: '2026-11-21T10:00:00+01:00', due: '2026-11-21T11:00:00+01:00', true: true, inactive: true},
        ]};
        const rows = scheduleRows(withHolds, ctx, 'all', '');
        expect(rows.map((r) => r.id)).toEqual(['h-markise-2', 'h-nacht-1']);
        expect(rows[0]).toMatchObject({kind: 'hold', what: 'Hinweis is x, for at least 10 min', program: 'Markise', next: '2026-11-21T12:06:00+01:00', last: undefined, active: false, off: '', progress: 0.4});
        expect(rows[1]).toMatchObject({kind: 'hold', what: 'Rollladen LEVEL greater than 0.5, for at least 1 h', next: undefined, last: '2026-11-21T11:00:00+01:00', active: true, off: 'the program is inactive', progress: null});
        expect(scheduleRows(withHolds, ctx, 'hold', '')).toHaveLength(2);
        expect(scheduleRows(withHolds, ctx, 'time', '')).toEqual([]);
        expect(scheduleRows(withHolds, ctx, 'delay', '')).toEqual([]);
        expect(scheduleRows(s, ctx, 'hold', '')).toEqual([]);
    });
    it('are filtered and searched', () => {
        expect(scheduleRows(s, ctx, 'time', '').map((r) => r.kind)).toEqual(['time', 'time', 'time', 'time', 'time']);
        expect(scheduleRows(s, ctx, 'delay', '').map((r) => r.id)).toEqual(['p-7', 'p-8']);
        expect(scheduleRows(s, ctx, 'all', 'rollladen').map((r) => r.id)).toEqual(['p-8']);
        expect(scheduleRows(s, ctx, 'all', 'nacht 22:00').map((r) => r.id)).toEqual(['t-nacht-1']);
        expect(scheduleRows({now: NOW, timers: [], pending: []}, ctx, 'all', '')).toEqual([]);
    });
    it('say why a time module is not armed', () => {
        expect(offText('inactive')).toBe('the program is inactive');
        expect(offText('faulty')).toBe('the program is faulty');
        expect(offText('no-clock')).toBe('the clock of the system is not set');
        expect(offText('no-location')).toBe('the location of the system is not set');
        expect(offText(undefined)).toBe('');
    });
});
