import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {
    actionExtras,
    namedValue,
    valuesKind,
    valuesProblem,
    valuesText,
    canonicalMAC,
    hookOf,
    hookPlaceholders,
    hookProblem,
    newHook,
    wakeProblem,
    addLine,
    addRule,
    astroWithOffset,
    channelNames,
    copyOf,
    copyRule,
    draftChannels,
    draftHints,
    draftOf,
    draftProblems,
    filterRows,
    firstValue,
    joinDuration,
    lineExtras,
    lineKind,
    moveAction,
    moveLine,
    newAction,
    newDraft,
    newLine,
    opsFor,
    paramKind,
    paramOf,
    parseTyped,
    pickerOptions,
    pickerRows,
    programOf,
    removeAction,
    removeLine,
    removeRule,
    sameProgram,
    splitDuration,
    stepAction,
    stepLine,
    stepRule,
    timeFormOf,
    timeOf,
    timeProblem,
    typedText,
    valueProblem,
    variableKind,
    wholeNumber,
    type Device,
    type Draft,
    type Env,
} from './editor.js';
import {MESSAGES} from './messages.js';
import type {Program, TimeModule} from './model.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const env: Env = {
    ctx: {
        variables: {
            regen: {key: 'regen', name: 'Regen', type: 'bool', visible: true, labels: {true: 'es regnet', false: 'trocken'}},
            modus: {key: 'modus', name: 'Modus', type: 'list', visible: true, entries: ['aus', 'an']},
            temp: {key: 'temp', name: 'Temp', type: 'number', visible: true, unit: '°C', min: 0, max: 40},
            text: {key: 'text', name: 'Text', type: 'string', visible: true},
            alarm: {key: 'alarm', name: 'Alarm', type: 'alarm', visible: true},
        },
        programs: {anderes: 'Anderes'},
        names: {},
    },
    params: {
        'HmIP-RF/A:1': [
            {id: 'LEVEL', type: 'FLOAT', read: true, write: true, min: 0, max: 1, unit: '100%'},
            {id: 'STATE', type: 'BOOL', read: true, write: false},
            {id: 'MODE', type: 'ENUM', read: true, write: true, values: ['OFF', '', 'AUTO']},
            {id: 'STOP', type: 'ACTION', read: false, write: true},
            {id: 'INFO', type: 'STRING', read: true, write: false},
            {id: 'COUNT', type: 'INTEGER', read: true, write: false},
        ],
    },
};

const program: Program = {
    key: 'markise',
    name: 'Markise',
    note: 'Terrasse',
    active: true,
    visible: true,
    operable: false,
    internal: true,
    at_start: true,
    rules: [
        {
            when: {
                op: 'any',
                groups: [
                    {
                        op: 'all',
                        lines: [
                            {variable: 'regen', op: 'eq', value: true, trigger: 'change', note: 'n'},
                            {device: {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'}, op: 'range', from: 0.2, below: 0.8, not: true, trigger: 'check'},
                        ],
                    },
                    {op: 'any', lines: [{time: {kind: 'daily', at: '07:00'}, trigger: 'update'}, {device: {interface: 'HmIP-RF', address: 'A:1', param: 'COUNT'}, op: 'outside', from: 1, to: 2, trigger: 'update'}]},
                ],
            },
            cancel_pending: true,
            note: 'r',
            then: [
                {kind: 'set', device: {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'}, to: 0, after: '1h30min', note: 'a'},
                {kind: 'variable', variable: 'temp', from_variable: 'temp'},
                {kind: 'variable', variable: 'text', from_device: {interface: 'HmIP-RF', address: 'A:1', param: 'INFO'}},
                {kind: 'set', device: {interface: 'HmIP-RF', address: 'B:1', param: 'X'}, special: 'last', after: '45s'},
                {kind: 'run', program: 'anderes', mode: 'then'},
                {kind: 'run', program: 'anderes', mode: 'check', after: '2h'},
                {kind: 'activate', program: 'anderes'},
                {kind: 'deactivate', program: 'anderes'},
                {kind: 'script', script: 'WriteLine(1);'},
            ],
        },
        {else: true, then: []},
    ],
};

const lines = (d: Draft, rule = 0) => d.rules[rule]!.groups.map((g) => g.lines.map((l) => l.variable || l.param || 'time').join(',')).join(' | ');
const acts = (d: Draft, rule = 0) => d.rules[rule]!.then.map((a) => a.kind + (a.what && a.kind === 'program' ? ':' + a.what : '')).join(',');

describe('durations and numbers', () => {
    it('splits a duration into the largest whole unit and joins it again', () => {
        expect(splitDuration('1h30min')).toEqual({n: '90', unit: 'min'});
        expect(splitDuration('2h')).toEqual({n: '2', unit: 'h'});
        expect(splitDuration('45s')).toEqual({n: '45', unit: 's'});
        expect(splitDuration('1min1s')).toEqual({n: '61', unit: 's'});
        expect(splitDuration('0s')).toEqual({n: '0', unit: 's'});
        expect(splitDuration(undefined)).toEqual({n: '', unit: 'min'});
        expect(splitDuration('soon')).toEqual({n: '', unit: 'min'});
        expect(joinDuration('90', 'min')).toBe('90min');
        expect(joinDuration('x', 'min')).toBe('');
        expect(wholeNumber(' 12 ')).toBe(12);
        expect(wholeNumber('0')).toBeNull();
        expect(wholeNumber('0', 0)).toBe(0);
        expect(wholeNumber('1.5')).toBeNull();
        expect(wholeNumber('')).toBeNull();
    });
});

describe('the draft', () => {
    it('is the program, and gives the same program back', () => {
        const d = draftOf(program);
        expect(programOf(d)).toEqual({...program, rules: [{...program.rules[0], then: program.rules[0]!.then.map((a) => (a.after === '1h30min' ? {...a, after: '90min'} : a))}, program.rules[1]]});
        expect(d.rules[0]!.then[0]).toMatchObject({delayed: true, delayN: '90', delayUnit: 'min', channel: 'HmIP-RF/A:1', param: 'LEVEL'});
        expect(d.rules[0]!.then[1]).toMatchObject({source: 'variable', fromVariable: 'temp', delayed: false, delayUnit: 's'});
        expect(d.rules[0]!.then[2]).toMatchObject({source: 'device', fromChannel: 'HmIP-RF/A:1', fromParam: 'INFO'});
        expect(d.rules[0]!.then[3]).toMatchObject({source: 'special', special: 'last'});
        expect(acts(d)).toBe('set,variable,variable,set,program:run-then,program:run-check,program:activate,program:deactivate,script');
        expect(sameProgram(d, draftOf(program))).toBe(true);
        d.name = 'Anders';
        expect(sameProgram(d, draftOf(program))).toBe(false);
    });
    it('a minimal program, a new one and a copy', () => {
        const d = draftOf({key: 'x', name: 'X', active: false, visible: true, operable: true, rules: [{when: undefined, then: []}]});
        expect(d).toMatchObject({note: '', internal: false, atStart: false});
        expect(d.rules[0]).toMatchObject({op: 'any', groups: [], cancelPending: false, note: ''});
        const fresh = newDraft();
        expect(programOf(fresh)).toEqual({key: '', name: '', active: true, visible: true, operable: true, rules: [{when: {op: 'any', groups: []}, then: []}]});
        const copy = copyOf(program);
        expect(copy).toMatchObject({key: '', name: 'Markise (copy)', active: false});
        expect(copy.rules).toHaveLength(2);
    });
    it('a time line always triggers at its times or only checks; a line without a time module gets a daily one', () => {
        const d = newDraft();
        const l = addLine(d.rules[0]!);
        Object.assign(l, {kind: 'time', trigger: 'change'});
        expect(programOf(d).rules[0]!.when!.groups[0]!.lines[0]).toEqual({time: {kind: 'daily'}, trigger: 'update'});
        l.trigger = 'check';
        expect(programOf(d).rules[0]!.when!.groups[0]!.lines[0]!.trigger).toBe('check');
        expect(newLine('time').trigger).toBe('update');
        const a = newAction();
        Object.assign(a, {delayed: true, delayN: 'x'});
        d.rules[0]!.then.push(a);
        expect(programOf(d).rules[0]!.then[0]!.after).toBeUndefined();
    });
    it('carries holds for, on for, a random delay, an astro point and a twilight', () => {
        const p = {key: 'x', name: 'X', active: true, visible: true, operable: true, rules: [{
            when: {op: 'any', groups: [{op: 'all', lines: [
                {variable: 'temp', op: 'gt', value: 5, for: '10min', trigger: 'change'},
                {time: {kind: 'daily', astro: 'before_sunset', offset: '20min', point: true, twilight: 'civil'}, trigger: 'update'},
            ]}]},
            then: [
                {kind: 'set', device: {interface: 'BidCos-RF', address: 'A:1', param: 'STATE'}, to: true, on_for: '5min', after_up_to: '30min'},
                {kind: 'variable', variable: 'temp', to: 1, after: '5min', after_up_to: '1h'},
            ],
        }]};
        const d = draftOf(p);
        expect(programOf(d)).toEqual(p);
        // the time dialog keeps them too
        const tm = p.rules[0]!.when.groups[0]!.lines[1]!.time!;
        expect(timeOf(timeFormOf(tm))).toEqual(tm);
        // a point that gets a fixed length is a range: the mark goes; a twilight stays
        expect(timeOf({...timeFormOf(tm), ranged: true, durationN: '1', durationUnit: 'h'})).toEqual({kind: 'daily', astro: 'before_sunset', offset: '20min', duration: '1h', twilight: 'civil'});
        expect(d.rules[0]!.groups[0]!.lines[0]).toMatchObject({held: true, holdN: '10', holdUnit: 'min'});
        expect(d.rules[0]!.then[0]).toMatchObject({timed: true, onForN: '5', onForUnit: 'min', delayed: true, random: true, delayN: '', upToN: '30', upToUnit: 'min'});
        expect(d.rules[0]!.then[1]).toMatchObject({timed: false, delayed: true, random: true, delayN: '5', upToN: '1', upToUnit: 'h'});
        expect(lineExtras(d.rules[0]!.groups[0]!.lines[0]!)).toBe('for at least 10 min');
        expect(lineExtras(d.rules[0]!.groups[0]!.lines[1]!)).toBe('');
        expect(actionExtras(d.rules[0]!.then[0]!)).toBe('on for 5 min · delayed at random, by 30 min at most');
        expect(actionExtras(d.rules[0]!.then[1]!)).toBe('delayed at random, by 1 h at most');
        expect(actionExtras(newAction())).toBe('');
        // switched off, the numbers stay in the form and leave the program
        const off = draftOf(p);
        off.rules[0]!.groups[0]!.lines[0]!.held = false;
        off.rules[0]!.then[0]!.timed = false;
        off.rules[0]!.then[1]!.random = false;
        const plain = programOf(off).rules[0]!;
        expect(plain.when!.groups[0]!.lines[0]!.for).toBeUndefined();
        expect(plain.then[0]!.on_for).toBeUndefined();
        expect(plain.then[1]).toEqual({kind: 'variable', variable: 'temp', to: 1, after: '5min'});
        // "on for" belongs to a fixed value written to a device; a script has no delay at all
        const a = d.rules[0]!.then[0]!;
        a.source = 'variable';
        a.fromVariable = 'temp';
        expect(programOf(d).rules[0]!.then[0]!.on_for).toBeUndefined();
        Object.assign(a, {kind: 'script', script: 'x'});
        expect(programOf(d).rules[0]!.then[0]).toEqual({kind: 'script', script: 'x'});
        // a time line has no "for"
        Object.assign(d.rules[0]!.groups[0]!.lines[0]!, {kind: 'time', time: null});
        expect(programOf(d).rules[0]!.when!.groups[0]!.lines[0]).toEqual({time: {kind: 'daily'}, trigger: 'update'});
    });
    it('names its channels once each', () => {
        expect(draftChannels(draftOf(program))).toEqual(['HmIP-RF/A:1', 'HmIP-RF/B:1']);
    });
});

describe('values', () => {
    it('are entered as their variable or datapoint asks', () => {
        expect(variableKind(env.ctx.variables.regen)).toEqual({kind: 'bool', options: [{value: true, label: 'es regnet'}, {value: false, label: 'trocken'}]});
        expect(variableKind(env.ctx.variables.alarm).options?.map((o) => o.label)).toEqual(['true', 'false']);
        expect(variableKind(env.ctx.variables.modus).options?.map((o) => o.value)).toEqual(['aus', 'an']);
        expect(variableKind({key: 'l', name: 'l', type: 'list', visible: true}).options).toEqual([]);
        expect(variableKind(env.ctx.variables.temp)).toEqual({kind: 'number', unit: '°C', min: 0, max: 40});
        expect(variableKind(env.ctx.variables.text).kind).toBe('text');
        expect(variableKind(undefined).kind).toBe('free');
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'LEVEL'))).toEqual({kind: 'number', unit: '100%', min: 0, max: 1});
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'STATE')).kind).toBe('bool');
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'STOP')).kind).toBe('bool');
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'MODE')).options).toEqual([{value: 0, label: 'OFF'}, {value: 1, label: '1'}, {value: 2, label: 'AUTO'}]);
        expect(paramKind({id: 'E', type: 'ENUM', read: true, write: false}).options).toEqual([]);
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'INFO')).kind).toBe('text');
        expect(paramKind(paramOf(env, 'HmIP-RF/A:1', 'COUNT')).kind).toBe('number');
        expect(paramKind(paramOf(env, 'HmIP-RF/Z:1', 'X')).kind).toBe('free');
        expect(lineKind({kind: 'variable', channel: '', param: '', variable: 'temp'}, env).kind).toBe('number');
        expect(lineKind({kind: 'device', channel: 'HmIP-RF/A:1', param: 'STATE', variable: ''}, env).kind).toBe('bool');
    });
    it('offer the comparisons the value can do', () => {
        expect(opsFor({kind: 'bool'})).toEqual(['eq', 'ne']);
        expect(opsFor({kind: 'text'})).toEqual(['eq', 'ne']);
        expect(opsFor(variableKind(env.ctx.variables.modus))).toEqual(['eq', 'ne']);
        expect(opsFor({kind: 'choice'})).toEqual(['eq', 'ne']);
        expect(opsFor(paramKind(paramOf(env, 'HmIP-RF/A:1', 'MODE')))).toHaveLength(8);
        expect(opsFor({kind: 'number'})).toHaveLength(8);
        expect(opsFor({kind: 'free'})).toHaveLength(8);
    });
    it('are read from what was typed', () => {
        expect(parseTyped('number', '21,5')).toBe(21.5);
        expect(parseTyped('number', ' 21. ')).toBe(21);
        expect(parseTyped('number', 'warm')).toBe('warm');
        expect(parseTyped('number', '')).toBe('');
        expect(parseTyped('text', ' 5 ')).toBe(' 5 ');
        expect(parseTyped('free', 'true')).toBe(true);
        expect(parseTyped('free', 'false')).toBe(false);
        expect(parseTyped('free', '3')).toBe(3);
        expect(parseTyped('free', 'OPEN')).toBe('OPEN');
        expect(typedText(undefined)).toBe('');
        expect(typedText(null)).toBe('');
        expect(typedText(1.5)).toBe('1.5');
        expect(firstValue({kind: 'bool'})).toBe(true);
        expect(firstValue({kind: 'choice', options: [{value: 'aus', label: 'aus'}]})).toBe('aus');
        expect(firstValue({kind: 'choice'})).toBeUndefined();
        expect(firstValue({kind: 'text'})).toBe('');
        expect(firstValue({kind: 'number'})).toBeUndefined();
    });
    it('say what is wrong with them', () => {
        expect(valueProblem({kind: 'bool'}, true)).toBe('');
        expect(valueProblem({kind: 'bool'}, 1)).toBe('Choose a value.');
        expect(valueProblem({kind: 'choice', options: [{value: 'a', label: 'a'}]}, 'a')).toBe('');
        expect(valueProblem({kind: 'choice', options: [{value: 'a', label: 'a'}]}, 'b')).toBe('Choose one of the entries.');
        expect(valueProblem({kind: 'choice'}, 'b')).toBe('Choose one of the entries.');
        expect(valueProblem({kind: 'number'}, 1)).toBe('');
        expect(valueProblem({kind: 'number'}, 'x')).toBe('Enter a number.');
        expect(valueProblem({kind: 'text'}, '')).toBe('');
        expect(valueProblem({kind: 'text'}, 1)).toBe('Enter a text.');
        expect(valueProblem({kind: 'free'}, 'x', true)).toBe('Enter a number.');
        expect(valueProblem({kind: 'free'}, 2, true)).toBe('');
        expect(valueProblem({kind: 'free'}, '')).toBe('Enter a value.');
        expect(valueProblem({kind: 'free'}, undefined)).toBe('Enter a value.');
        expect(valueProblem({kind: 'free'}, null)).toBe('Enter a value.');
        expect(valueProblem({kind: 'free'}, false)).toBe('');
    });
});

describe('what is wrong with a draft', () => {
    const one = (change: (d: Draft) => void): string[] => {
        const d = newDraft();
        d.name = 'P';
        change(d);
        return Object.values(draftProblems(d, env));
    };
    const withLine = (fields: object) => one((d) => Object.assign(addLine(d.rules[0]!), fields));
    const withAction = (fields: object, active = true) =>
        one((d) => {
            d.active = active;
            d.rules[0]!.then.push(Object.assign(newAction(), fields));
        });

    it('a sound program has nothing', () => {
        const d = draftOf(program);
        d.active = false;
        expect(draftProblems(d, env)).toEqual({});
        // an active one with a script says so at the script
        expect(Object.values(draftProblems(draftOf(program), env))).toEqual(['A program with a script cannot be active.']);
    });
    it('the name', () => {
        expect(draftProblems(newDraft(), env)).toEqual({name: 'A program needs a name.'});
        expect(one((d) => (d.name = 'x'.repeat(201)))).toEqual(['The name is too long: 200 characters at most.']);
        expect(one((d) => (d.name = 'ä'.repeat(200)))).toEqual([]);
    });
    it('condition lines', () => {
        expect(withLine({})).toEqual(['Choose a device.']);
        expect(withLine({channel: 'HmIP-RF/A:1'})).toEqual(['Choose a datapoint.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'STOP'})).toEqual(['This channel has no datapoint STOP to look at.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'NOPE'})).toEqual(['This channel has no datapoint NOPE to look at.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'gt', value: 0.5})).toEqual([]);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'gt', value: 'x'})).toEqual(['Enter a number.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'STATE', op: 'gt', value: true})).toEqual(['This can only be compared for "equal" and "not equal".']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'MODE', op: 'ge', value: 2})).toEqual([]);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'range', from: 1, below: 'x'})).toEqual(['Enter a number.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'range', from: undefined, below: 2})).toEqual(['Enter a number.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'range', from: 1, below: 2})).toEqual([]);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'outside', from: 1, to: undefined})).toEqual(['Enter a number.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'outside', from: 'x', to: 3})).toEqual(['Enter a number.']);
        expect(withLine({channel: 'HmIP-RF/A:1', param: 'LEVEL', op: 'outside', from: 1, to: 3})).toEqual([]);
        // a device the system does not know: anything goes that has a value
        expect(withLine({channel: 'HmIP-RF/Z:1', param: 'ANY', value: 'x'})).toEqual([]);
        expect(withLine({channel: 'HmIP-RF/Z:1', param: 'ANY'})).toEqual(['Enter a value.']);
        expect(withLine({kind: 'variable'})).toEqual(['Choose a variable.']);
        expect(withLine({kind: 'variable', variable: 'weg'})).toEqual(['The variable weg does not exist.']);
        expect(withLine({kind: 'variable', variable: 'modus', value: 'an'})).toEqual([]);
        expect(withLine({kind: 'variable', variable: 'modus', value: 'halb'})).toEqual(['Choose one of the entries.']);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3})).toEqual([]);
        // holds for: a whole number of seconds, minutes or hours, seven days at most
        const hold = 'Enter how long the comparison must have held: a whole number, seven days at most.';
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: true})).toEqual([hold]);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: true, holdN: '0'})).toEqual([hold]);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: true, holdN: '169', holdUnit: 'h'})).toEqual([hold]);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: true, holdN: '168', holdUnit: 'h'})).toEqual([]);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: true, holdN: '90', holdUnit: 'x'})).toEqual([]);
        expect(withLine({kind: 'variable', variable: 'temp', op: 'lt', value: 3, held: false, holdN: 'x'})).toEqual([]);
        expect(withLine({kind: 'time', time: {kind: 'daily', at: '07:00'}, held: true})).toEqual([]);
        expect(withLine({kind: 'time'})).toEqual(['Set the time.']);
        expect(withLine({kind: 'time', time: {kind: 'weekly', at: '07:00'}})).toEqual(['Choose at least one weekday.']);
        expect(withLine({kind: 'time', time: {kind: 'daily', at: '07:00'}})).toEqual([]);
    });
    it('activities', () => {
        expect(withAction({})).toEqual(['Choose a device.']);
        expect(withAction({channel: 'HmIP-RF/A:1'})).toEqual(['Choose a datapoint.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'STATE'})).toEqual(['This channel has no datapoint STATE to write.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 0.5})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 'x'})).toEqual(['Enter a number.']);
        expect(withAction({channel: 'HmIP-RF/Z:1', param: 'X', to: 1})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 1, delayed: true, delayN: ''})).toEqual(['Enter the delay as a whole number.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 1, delayed: true, delayN: '5'})).toEqual([]);
        // a random delay: from nothing or a number, to something longer
        const lvl = {channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 1, delayed: true, random: true};
        expect(withAction({...lvl})).toEqual(['Enter the longest delay as a whole number.']);
        expect(withAction({...lvl, delayN: 'x', upToN: '5'})).toEqual(['Enter the delay as a whole number.']);
        expect(withAction({...lvl, upToN: '5'})).toEqual([]);
        expect(withAction({...lvl, delayN: '0', upToN: '5'})).toEqual([]);
        expect(withAction({...lvl, delayN: '5', delayUnit: 'min', upToN: '5', upToUnit: 'min'})).toEqual(['The longest delay must be longer than the shortest.']);
        expect(withAction({...lvl, delayN: '50', delayUnit: 'min', upToN: '1', upToUnit: 'h'})).toEqual([]);
        expect(withAction({...lvl, delayed: false, upToN: 'x'})).toEqual([]);
        // on for: the channel needs an on-time
        const onFor = 'Enter how long it stays on: a whole number, seven days at most.';
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', to: 1, timed: true, onForN: '5'})).toEqual(['This channel has no on-time (ON_TIME): "on for" is not possible here.']);
        expect(withAction({channel: 'HmIP-RF/B:1', param: 'STATE', to: true, timed: true})).toEqual([onFor]);
        expect(withAction({channel: 'HmIP-RF/B:1', param: 'STATE', to: true, timed: true, onForN: '8', onForUnit: 'h'})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/Z:1', param: 'X', to: 1, timed: true, onForN: '5'})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/B:1', param: 'STATE', source: 'variable', fromVariable: 'temp', timed: true})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'special', special: 'last'})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'variable'})).toEqual(['Choose a variable.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'variable', fromVariable: 'weg'})).toEqual(['The variable weg does not exist.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'variable', fromVariable: 'temp'})).toEqual([]);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'device', fromChannel: 'HmIP-RF/A:1'})).toEqual(['Choose a datapoint.']);
        expect(withAction({channel: 'HmIP-RF/A:1', param: 'LEVEL', source: 'device', fromChannel: 'HmIP-RF/A:1', fromParam: 'LEVEL'})).toEqual([]);
        expect(withAction({kind: 'variable'})).toEqual(['Choose a variable.']);
        expect(withAction({kind: 'variable', variable: 'duty-cycle'})).toEqual(['A system value cannot be written.']);
        expect(withAction({kind: 'variable', variable: 'weg'})).toEqual(['The variable weg does not exist.']);
        expect(withAction({kind: 'variable', variable: 'regen', to: false})).toEqual([]);
        expect(withAction({kind: 'variable', variable: 'regen', to: 'nass'})).toEqual(['Choose a value.']);
        expect(withAction({kind: 'program'})).toEqual(['Choose a program.']);
        expect(withAction({kind: 'program', program: 'weg'})).toEqual(['The program weg does not exist.']);
        expect(withAction({kind: 'program', program: 'anderes'})).toEqual([]);
        expect(withAction({kind: 'script', script: 'x'})).toEqual(['A program with a script cannot be active.']);
        expect(withAction({kind: 'script', script: 'x'}, false)).toEqual([]);
    });
    it('hints', () => {
        const d = newDraft();
        expect(draftHints(d)).toEqual([]);
        const l = addLine(d.rules[0]!);
        l.trigger = 'check';
        expect(draftHints(d)).toEqual(['No line triggers this program: it only runs when it is started by hand, by another program or at a system start.']);
        l.kind = 'time';
        expect(draftHints(d)).toEqual([]);
        Object.assign(l, {kind: 'device', trigger: 'update'});
        addRule(d, true);
        expect(draftHints(d)).toEqual([]);
        d.rules[1]!.then.push(Object.assign(newAction(), {source: 'special'}));
        expect(draftHints(d)).toHaveLength(2);
        expect(draftHints(d)[0]).toContain('"Trigger on update"');
        expect(draftHints(d)[1]).toContain('special value');
        l.trigger = 'change';
        expect(draftHints(d)[0]).toContain('"Trigger on change"');
        l.trigger = 'value';
        expect(draftHints(d)).toHaveLength(1);
    });
});

describe('moving, copying, removing', () => {
    const ids = (d: Draft) => d.rules[0]!.groups.map((g) => g.lines.map((l) => l.id));
    it('lines', () => {
        const d = draftOf(program);
        expect(lines(d)).toBe('regen,LEVEL | time,COUNT');
        const [[a, b], [c, e]] = ids(d) as [[number, number], [number, number]];
        moveLine(d, a, b); // down in its group: after the one it was dropped on
        expect(lines(d)).toBe('LEVEL,regen | time,COUNT');
        moveLine(d, a, b);
        expect(lines(d)).toBe('regen,LEVEL | time,COUNT');
        moveLine(d, e, a); // into another group: before the one it was dropped on
        expect(lines(d)).toBe('COUNT,regen,LEVEL | time');
        moveLine(d, c, a); // the last line of a group: the group goes
        expect(lines(d)).toBe('COUNT,time,regen,LEVEL');
        moveLine(d, a, a);
        moveLine(d, a, 999999);
        moveLine(d, 999999, a);
        expect(lines(d)).toBe('COUNT,time,regen,LEVEL');

        const s = draftOf(program);
        const [[a2, b2], [c2]] = ids(s) as [[number, number], [number, number]];
        stepLine(s, a2, 1);
        expect(lines(s)).toBe('LEVEL,regen | time,COUNT');
        stepLine(s, a2, 1); // over the edge: the first of the next group
        expect(lines(s)).toBe('LEVEL | regen,time,COUNT');
        stepLine(s, a2, -1); // and back: the last of the group before
        expect(lines(s)).toBe('LEVEL,regen | time,COUNT');
        stepLine(s, b2, -1); // no group before the first
        stepLine(s, 999999, 1);
        expect(lines(s)).toBe('LEVEL,regen | time,COUNT');
        stepLine(s, c2, -1);
        stepLine(s, c2, -1);
        expect(lines(s)).toBe('LEVEL,time,regen | COUNT');
        removeLine(s, c2);
        removeLine(s, 999999);
        expect(lines(s)).toBe('LEVEL,regen | COUNT');
        removeLine(s, s.rules[0]!.groups[1]!.lines[0]!.id);
        expect(lines(s)).toBe('LEVEL,regen');
        addLine(s.rules[0]!, s.rules[0]!.groups[0]);
        addLine(s.rules[0]!);
        expect(s.rules[0]!.groups.map((g) => g.lines.length)).toEqual([3, 1]);
        expect(s.rules[0]!.groups[1]!.op).toBe('all');
    });
    it('activities', () => {
        const d = draftOf(program);
        d.rules[0]!.then.splice(3);
        const [a, b, c] = d.rules[0]!.then.map((x) => x.id) as [number, number, number];
        const order = () => d.rules.map((r) => r.then.map((x) => [a, b, c].indexOf(x.id)).join('')).join('|');
        moveAction(d, a, c);
        expect(order()).toBe('120|');
        stepAction(d, a, -1);
        expect(order()).toBe('102|');
        stepAction(d, c, 1);
        stepAction(d, b, -1);
        stepAction(d, 999999, 1);
        expect(order()).toBe('102|');
        d.rules[1]!.then.push(newAction());
        moveAction(d, a, d.rules[1]!.then[0]!.id); // into another rule
        expect(order()).toBe('12|0-1');
        moveAction(d, a, a);
        moveAction(d, a, 999999);
        removeAction(d, a);
        removeAction(d, 999999);
        expect(order()).toBe('12|-1');
    });
    it('rules', () => {
        const d = draftOf(program);
        const first = d.rules[0]!.id;
        copyRule(d, first);
        expect(d.rules.map((r) => r.else)).toEqual([false, false, true]);
        expect(lines(d, 1)).toBe(lines(d, 0));
        expect(d.rules[1]!.groups[0]!.lines[0]!.id).not.toBe(d.rules[0]!.groups[0]!.lines[0]!.id);
        d.rules[1]!.groups[1]!.lines[0]!.time!.at = '08:00';
        expect(d.rules[0]!.groups[1]!.lines[0]!.time!.at).toBe('07:00');
        copyRule(d, d.rules[2]!.id); // the "Else…" is there once
        copyRule(d, 999999);
        expect(d.rules).toHaveLength(3);
        addRule(d, true);
        expect(d.rules).toHaveLength(3);
        addRule(d, false); // before the "Else…"
        expect(d.rules.map((r) => r.else)).toEqual([false, false, false, true]);
        const added = d.rules[2]!.id;
        stepRule(d, added, 1); // not past the "Else…"
        stepRule(d, d.rules[3]!.id, -1);
        stepRule(d, 999999, 1);
        stepRule(d, first, -1);
        expect(d.rules[2]!.id).toBe(added);
        stepRule(d, added, -1);
        expect(d.rules[1]!.id).toBe(added);
        removeRule(d, added);
        removeRule(d, d.rules[2]!.id);
        expect(d.rules.map((r) => r.else)).toEqual([false, false]);
        addRule(d, false);
        addRule(d, true);
        expect(d.rules.map((r) => r.else)).toEqual([false, false, false, true]);
    });
});

describe('the device picker', () => {
    const devices: Device[] = [
        {interface: 'HmIP-RF', address: 'A', name: 'Rollladen', type: 'HmIP-BROLL', channels: [{address: 'A:0', type: 'MAINTENANCE'}, {address: 'A:1', name: 'Rollladen Wohnen', type: 'SHUTTER', rooms: ['Wohnzimmer']}]},
        {interface: 'BidCos-RF', address: 'B', type: 'HM-LC-Sw1', channels: [{address: 'B:1', type: 'SWITCH', rooms: ['Bad', 'Wohnzimmer']}]},
    ];
    it('lists every channel, with what the filters offer', () => {
        const rows = pickerRows(devices);
        expect(rows.map((r) => `${r.channel} ${r.name}/${r.device}`)).toEqual(['HmIP-RF/A:0 Rollladen/Rollladen', 'HmIP-RF/A:1 Rollladen Wohnen/Rollladen', 'BidCos-RF/B:1 /B']);
        expect(pickerOptions(rows)).toEqual({interfaces: ['BidCos-RF', 'HmIP-RF'], rooms: ['Bad', 'Wohnzimmer'], types: ['HM-LC-Sw1', 'HmIP-BROLL']});
        const f = {query: '', interface: '', room: '', type: ''};
        expect(filterRows(rows, f)).toHaveLength(3);
        expect(filterRows(rows, {...f, query: 'wohn shutter'}).map((r) => r.address)).toEqual(['A:1']);
        expect(filterRows(rows, {...f, room: 'Wohnzimmer'}).map((r) => r.address)).toEqual(['A:1', 'B:1']);
        expect(filterRows(rows, {...f, type: 'HmIP-BROLL'}).map((r) => r.address)).toEqual(['A:0', 'A:1']);
        expect(filterRows(rows, {...f, interface: 'BidCos-RF', query: 'b:1'}).map((r) => r.address)).toEqual(['B:1']);
        expect(channelNames(devices)).toEqual({'HmIP-RF/A:0': 'Rollladen', 'HmIP-RF/A:1': 'Rollladen Wohnen'});
    });
});

describe('a time module\'s form', () => {
    const round = (m: TimeModule) => timeOf(timeFormOf(m));
    it('gives the module back', () => {
        for (const m of [
            {kind: 'daily', at: '07:00'},
            {kind: 'daily', at: '22:00', duration: '8h', weekdays: ['mon', 'tue', 'wed', 'thu', 'fri']},
            {kind: 'daily', all_day: true, weekdays: ['sat', 'sun'], every: 3, from: '2026-01-01'},
            {kind: 'weekly', astro: 'sunset', weekdays: ['mon', 'sun'], every: 2, from: '2026-01-01 12:00', until: '2026-12-31', count: 5},
            {kind: 'weekly', astro: 'before_sunset', offset: '30min', duration: '45s', weekdays: ['wed']},
            {kind: 'monthly', at: '06:30:15', day: 31},
            {kind: 'monthly', at: '06:30', nth: 2, weekdays: ['fri']},
            {kind: 'yearly', astro: 'night', offset: '2h', day: 24, month: 12},
            {kind: 'yearly', at: '12:00', nth: 5, weekdays: ['sun'], month: 3},
            {kind: 'once', at: '12:00', date: '2026-12-24'},
            {kind: 'interval', all_day: true, interval: '15min'},
            {kind: 'interval', at: '08:00', interval: '90s'},
        ] as TimeModule[]) {
            expect(round(m), JSON.stringify(m)).toEqual(m.offset === '2h' ? {...m, offset: '120min'} : m);
            expect(timeProblem(m), JSON.stringify(m)).toBe('');
        }
        expect(timeFormOf(null)).toMatchObject({kind: 'daily', when: 'at', at: '07:00', days: 'all', by: 'day'});
        expect(timeOf(timeFormOf(null))).toEqual({kind: 'daily', at: '07:00'});
        expect(astroWithOffset('day')).toBe(true);
        expect(astroWithOffset('sunrise')).toBe(false);
    });
    it('says what is missing', () => {
        const form = (change: object) => timeProblem(timeOf({...timeFormOf(null), ...change}));
        expect(timeProblem({kind: 'sometimes'})).toBe('Choose how the time repeats.');
        expect(form({at: '7'})).toBe('Enter the time of day as HH:MM.');
        expect(form({kind: 'interval', intervalN: ''})).toBe('Enter the interval as a whole number.');
        expect(form({kind: 'interval', intervalN: '5', ranged: true})).toBe('');
        expect(form({ranged: true, durationN: ''})).toBe('Enter how long the range lasts: a whole number, seven days at most.');
        expect(form({ranged: true, durationN: '200', durationUnit: 'h'})).toBe('Enter how long the range lasts: a whole number, seven days at most.');
        expect(form({when: 'day', ranged: true, durationN: 'x'})).toBe('Enter how long the range lasts: a whole number, seven days at most.');
        expect(form({when: 'day', offsetMin: 'x'})).toBe('Enter the offset in minutes: a whole number, less than a day.');
        expect(form({when: 'day', offsetMin: '1440'})).toBe('Enter the offset in minutes: a whole number, less than a day.');
        expect(form({when: 'day', offsetMin: '0'})).toBe('');
        expect(form({when: 'sunrise', offsetMin: '30'})).toBe('');
        expect(form({every: 'x'})).toBe('Enter "every" as a whole number.');
        expect(form({every: '1'})).toBe('');
        expect(form({every: '2'})).toBe('"Every" and the number of times are counted from "valid from": enter it.');
        expect(form({kind: 'once', date: '24.12.'})).toBe('Enter the date as YYYY-MM-DD.');
        expect(form({kind: 'once', date: '2026-12-24 10:00'})).toBe('Enter the date as YYYY-MM-DD.');
        expect(form({kind: 'weekly'})).toBe('Choose at least one weekday.');
        expect(form({kind: 'monthly', day: '32'})).toBe('The day of the month is 1 to 31.');
        expect(form({kind: 'monthly', day: 'x'})).toBe('The day of the month is 1 to 31.');
        expect(form({kind: 'monthly', by: 'weekday', nth: '6'})).toBe('The weekday\'s number in the month is 1 to 5.');
        expect(form({kind: 'monthly', by: 'weekday', nth: 'x'})).toBe('The weekday\'s number in the month is 1 to 5.');
        expect(form({kind: 'yearly', month: '13'})).toBe('The month is 1 to 12.');
        expect(form({kind: 'yearly', month: 'x'})).toBe('The month is 1 to 12.');
        expect(timeProblem({kind: 'yearly', at: '12:00', day: 1})).toBe('The month is 1 to 12.');
        expect(form({from: 'soon'})).toBe('Enter "valid from" as YYYY-MM-DD or YYYY-MM-DD HH:MM.');
        expect(form({until: 'later'})).toBe('Enter "valid until" as YYYY-MM-DD or YYYY-MM-DD HH:MM.');
        expect(form({count: 'x'})).toBe('Enter the number of times as a whole number.');
        expect(form({count: '3'})).toBe('"Every" and the number of times are counted from "valid from": enter it.');
        expect(form({from: '2026-12-02', until: '2026-12-01'})).toBe('"Valid until" must be after "valid from".');
        expect(form({from: '2026-12-01 12:00', until: '2026-12-01 12:00'})).toBe('"Valid until" must be after "valid from".');
        // a date without a time includes its day
        expect(form({from: '2026-12-01', until: '2026-12-01'})).toBe('');
        expect(form({from: '2026-12-01', until: '2026-12-01 00:00'})).toBe('"Valid until" must be after "valid from".');
    });
});

describe('webhooks and Wake-on-LAN', () => {
    const stored: Program = {
        key: 'melden', name: 'Melden', active: true, visible: true, operable: true,
        rules: [{when: {op: 'all', groups: []}, then: [
            {kind: 'webhook', webhook: {method: 'POST', url: 'https://h/x?v={value}', headers: [{name: 'Authorization', value: 'Bearer {secret:token}'}], body_type: 'form', body: 't={var:temp}', insecure: true}, after: '5s'},
            {kind: 'webhook', webhook: {method: 'GET', url: 'http://h/'}},
            {kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22', broadcast: '192.168.1.255'}},
            {kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22'}},
        ]}],
    };
    it('go into the draft and come back the same', () => {
        const d = draftOf(stored);
        const [hook, get, wake] = d.rules[0]!.then;
        expect(hook!.kind).toBe('webhook');
        expect(hook!.hook).toEqual({method: 'POST', url: 'https://h/x?v={value}', headers: [{name: 'Authorization', value: 'Bearer {secret:token}'}], bodyType: 'form', body: 't={var:temp}', insecure: true});
        expect(get!.hook).toEqual({...newHook(), method: 'GET', url: 'http://h/'});
        expect([wake!.kind, wake!.mac, wake!.broadcast]).toEqual(['wake', 'AA:BB:CC:00:11:22', '192.168.1.255']);
        expect(programOf(d).rules).toEqual(stored.rules);
        expect(draftProblems(d, {...env, secrets: ['token']})).toEqual({});
        // the draft's copy of the headers is its own
        hook!.hook.headers[0]!.value = 'x';
        expect(stored.rules[0]!.then[0]!.webhook!.headers![0]!.value).toBe('Bearer {secret:token}');
    });
    it('a form becomes a request: no header without a name, no body with GET, the address trimmed', () => {
        expect(hookOf({method: 'GET', url: ' http://h/ ', headers: [{name: ' ', value: 'x'}, {name: ' A ', value: ' b '}], bodyType: 'json', body: '{}', insecure: false})).toEqual({method: 'GET', url: 'http://h/', headers: [{name: 'A', value: ' b '}]});
        expect(hookOf({...newHook(), url: 'http://h/', body: '{}'})).toEqual({method: 'POST', url: 'http://h/', body_type: 'json', body: '{}'});
        expect(hookOf({...newHook(), url: 'http://h/'})).toEqual({method: 'POST', url: 'http://h/'});
    });
    it('names its placeholders, each once, and none of a GET\'s body', () => {
        expect(hookPlaceholders({method: 'POST', url: 'http://h/{var:temp}?{time}', headers: [{name: 'A', value: '{secret:k}{time}'}], bodyType: 'json', body: '{"a": "{program}{trigger}{value}{nope}"}', insecure: false}))
            .toEqual(['var:temp', 'time', 'secret:k', 'program', 'trigger', 'value']);
        expect(hookPlaceholders({...newHook(), method: 'GET', url: 'http://h/', body: '{value}'})).toEqual([]);
    });
    it('says what is wrong with a webhook', () => {
        const h = (over: Partial<ReturnType<typeof newHook>>) => hookProblem({...newHook(), url: 'http://h/', ...over}, env.ctx, ['token']);
        expect(h({})).toBe('');
        expect(h({url: ' '})).toBe('Enter the address.');
        for (const url of ['h/x', 'ftp://h/', 'http://h/a b', 'http://u@h/', 'http://u:p@h/', 'http://h/#x', 'http://h/#', 'http:///x', 'http://h/' + 'x'.repeat(2000)]) {
            expect(h({url}), url).toContain('The address must be');
        }
        expect(h({url: 'http://{var:temp}/x'})).toBe("A placeholder cannot stand in the address' host.");
        expect(h({url: 'https://h:8443/{var:temp}/{program}?a={value}'})).toBe('');
        const nine = Array.from({length: 9}, (_, i) => ({name: 'A' + i, value: ''}));
        expect(h({headers: nine})).toBe('A webhook has at most 8 headers.');
        expect(h({headers: [...nine.slice(0, 8), {name: '', value: ''}]})).toBe('');
        expect(h({headers: [{name: 'A B', value: ''}]})).toBe('"A B" is not a header\'s name.');
        expect(h({headers: [{name: '', value: 'x'}]})).toBe('A header needs a name.');
        expect(h({headers: [{name: 'Host', value: 'x'}]})).toBe('The header Host is set by the request itself.');
        expect(h({headers: [{name: 'A', value: 'x'}, {name: 'a', value: 'y'}]})).toBe('The header A is there twice.');
        expect(h({headers: [{name: 'A', value: 'x\ny'}]})).toBe('The value of the header A is too long or has a line break.');
        expect(h({headers: [{name: 'A', value: 'x'.repeat(1001)}]})).toContain('too long');
        expect(h({body: 'ä'.repeat(8193)})).toBe('The body is longer than 16384 bytes.');
        expect(h({method: 'GET', body: 'x'.repeat(20000)})).toBe('');
        expect(h({body: '{var:nope}'})).toBe('The variable nope does not exist.');
        expect(h({body: '{var:temp}{secret:token}'})).toBe('');
        expect(h({body: '{secret:other}'})).toBe('There is no secret other: enter it in the settings first.');
        // the secrets are not known yet: nothing is said about them
        expect(hookProblem({...newHook(), url: 'http://h/{secret:other}'}, env.ctx)).toBe('');
    });
    it('reads a hardware address however it is written', () => {
        expect(canonicalMAC(' aa-bb-cc-00-11-22 ')).toBe('AA:BB:CC:00:11:22');
        expect(canonicalMAC('aabbcc001122')).toBe('AA:BB:CC:00:11:22');
        for (const bad of ['', 'aa:bb:cc:00:11', 'aa:bbcc:00:11:22', 'gg:bb:cc:00:11:22']) expect(canonicalMAC(bad), bad).toBe('');
    });
    it('says what is wrong with a Wake-on-LAN, and writes the address the one way', () => {
        const a = newAction('wake');
        expect(wakeProblem(a)).toBe('Enter the hardware address: AA:BB:CC:DD:EE:FF.');
        a.mac = 'aa-bb-cc-00-11-22';
        expect(wakeProblem(a)).toBe('');
        a.broadcast = '192.168.1.256';
        expect(wakeProblem(a)).toContain('The broadcast address is');
        a.broadcast = ' 192.168.1.255 ';
        expect(wakeProblem(a)).toBe('');
        const d = newDraft();
        d.name = 'P';
        d.rules[0]!.then.push(a, {...newAction('wake'), mac: 'nope'}, {...newAction('webhook')});
        expect(programOf(d).rules[0]!.then).toEqual([{kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22', broadcast: '192.168.1.255'}}, {kind: 'wake', wake: {mac: 'nope'}}, {kind: 'webhook', webhook: {method: 'POST', url: ''}}]);
        const problems = Object.values(draftProblems(d, env));
        expect(problems).toContain('Enter the hardware address: AA:BB:CC:DD:EE:FF.');
        expect(problems).toContain('Enter the address.');
    });
});

describe('several values of a channel', () => {
    const light: Env = {
        ...env,
        params: {
            ...env.params,
            'HmIP-RF/L:8': [
                {id: 'COLOR', type: 'ENUM', read: true, write: true, values: ['BLACK', 'BLUE', '', 'RED']},
                {id: 'LEVEL', type: 'FLOAT', read: true, write: true, min: 0, max: 1.01},
                {id: 'DURATION_VALUE', type: 'INTEGER', read: false, write: true},
                {id: 'ON', type: 'BOOL', read: true, write: true},
                {id: 'ACTIVITY', type: 'ENUM', read: true, write: false, values: ['A', 'B']},
            ],
        },
    };
    const stored: Program = {
        key: 'licht', name: 'Licht', active: true, visible: true, operable: true,
        rules: [{when: {op: 'all', groups: []}, then: [{kind: 'channel', channel: 'HmIP-RF/L:8', values: [{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 0.5}], after: '5s'}]}],
    };
    it('go into the draft and come back the same; their channel is one whose datapoints are asked for', () => {
        const d = draftOf(stored);
        const a = d.rules[0]!.then[0]!;
        expect([a.kind, a.channel, a.param]).toEqual(['channel', 'HmIP-RF/L:8', '']);
        expect(a.values).toEqual([{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 0.5}]);
        expect(programOf(d).rules).toEqual(stored.rules);
        expect(draftChannels(d)).toEqual(['HmIP-RF/L:8']);
        expect(draftProblems(d, light)).toEqual({});
        a.values[0]!.value = 'BLUE';
        expect(stored.rules[0]!.then[0]!.values![0]!.value).toBe('RED');
        expect(valuesText(a.values)).toBe('COLOR = BLUE, LEVEL = 0.5');
        expect(valuesText([{param: 'ON', value: true}, {param: 'X', value: undefined}])).toBe('ON = true, X = ');
    });
    it('an entry of a list is its name; one without a name its number', () => {
        const color = light.params['HmIP-RF/L:8']![0]!;
        expect(valuesKind(color)).toEqual({kind: 'choice', options: [{value: 'BLACK', label: 'BLACK'}, {value: 'BLUE', label: 'BLUE'}, {value: 2, label: '2'}, {value: 'RED', label: 'RED'}]});
        expect(valuesKind(light.params['HmIP-RF/L:8']![1]!).kind).toBe('number');
        expect(valuesKind(undefined).kind).toBe('free');
        expect(namedValue(color, 3)).toBe('RED');
        expect(namedValue(color, 2)).toBe(2);
        expect(namedValue(color, 'BLUE')).toBe('BLUE');
        expect(namedValue(light.params['HmIP-RF/L:8']![1]!, 1)).toBe(1);
        expect(namedValue(undefined, 1)).toBe(1);
    });
    it('says what is wrong', () => {
        const v = (...values: [string, unknown][]) => valuesProblem('HmIP-RF/L:8', values.map(([param, value]) => ({param, value: value as never})), light);
        expect(valuesProblem('', [], light)).toBe('Choose a device.');
        expect(v()).toBe('Choose the datapoints to write.');
        expect(v(['COLOR', 'RED'], ['LEVEL', 0.5], ['DURATION_VALUE', 10], ['ON', false])).toBe('');
        expect(v(['COLOR', 3])).toBe('');
        expect(v(['COLOR', 'GREEN'])).toBe('COLOR: Choose one of the entries.');
        expect(v(['LEVEL', 'x'])).toBe('LEVEL: Enter a number.');
        expect(v(['LEVEL', 2])).toBe('LEVEL: outside 0 to 1.01');
        expect(v(['LEVEL', -1])).toBe('LEVEL: outside 0 to 1.01');
        expect(v(['DURATION_VALUE', 100000])).toBe('');
        expect(v(['ON', 1])).toBe('ON: Choose a value.');
        expect(v(['ACTIVITY', 'A'])).toBe('This channel has no datapoint ACTIVITY to write.');
        expect(v(['NOPE', 1])).toBe('This channel has no datapoint NOPE to write.');
        expect(v(['A B', 1])).toBe('"A B" is not a datapoint\'s name.');
        expect(v(['LEVEL', 0.5], ['LEVEL', 0.6])).toBe('The datapoint LEVEL is there twice.');
        expect(valuesProblem('HmIP-RF/L:8', Array.from({length: 33}, (_, i) => ({param: 'P' + i, value: 1})), light)).toBe('One activity writes at most 32 datapoints.');
        // a channel the system does not know: anything with a value passes, and is checked when it is sent
        expect(valuesProblem('HmIP-RF/NEW:1', [{param: 'ANYTHING', value: 5}], light)).toBe('');
        expect(valuesProblem('HmIP-RF/NEW:1', [{param: 'ANYTHING', value: ''}], light)).toBe('ANYTHING: Enter a value.');
        // a number with one limit only
        const half: Env = {...env, params: {'X/Y:1': [{id: 'A', type: 'FLOAT', read: true, write: true, min: 0}, {id: 'B', type: 'FLOAT', read: true, write: true, max: 1}]}};
        expect(valuesProblem('X/Y:1', [{param: 'A', value: -1}], half)).toBe('A: outside 0 to …');
        expect(valuesProblem('X/Y:1', [{param: 'B', value: 2}], half)).toBe('B: outside … to 1');
        const d = newDraft();
        d.name = 'P';
        d.rules[0]!.then.push(newAction('channel'));
        expect(Object.values(draftProblems(d, light))).toContain('Choose a device.');
    });
});
