import {readdirSync, readFileSync} from 'node:fs';
import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, beforeEach, describe, expect, it} from 'vitest';

import {DISPLAY_ICONS} from './display.js';
import {ENTRIES} from './forms.js';
import {HELPER_KINDS} from './helpers.js';
import {MESSAGES} from './messages.js';
import {
    COMPUTED_FUNCTIONS,
    computedText,
    emptyInput,
    fitInputs,
    inputCount,
    inputLabel,
    actionText,
    blockText,
    channelName,
    channelOf,
    context,
    dateTime,
    defaultValue,
    defOf,
    durationText,
    edgeText,
    emptyForm,
    formOf,
    formProblems,
    importStatus,
    lineNumbers,
    lineText,
    marksTitle,
    matches,
    nameTitle,
    noteText,
    parseValue,
    programMarks,
    programState,
    runtimeFormOf,
    runtimeOf,
    refText,
    reportHasScript,
    reportTotals,
    ruleHeading,
    runTrigger,
    timeText,
    triggerLabel,
    triggerSummary,
    valueText,
    variableMarks,
    variableName,
    wallTime,
    type Context,
    type Overview,
    type ProgramView,
    type Report,
    type VariableDef,
} from './model.js';

beforeAll(() => addMessages(MESSAGES));
beforeEach(() => (i18n.language = 'de'));

const regen: VariableDef = {key: 'regen', name: 'Regen', type: 'bool', labels: {false: 'trocken', true: 'es regnet'}, visible: true};
const temp: VariableDef = {key: 'temp', name: 'Temperatur', type: 'number', unit: '°C', min: -40, max: 60, visible: true};
const modus: VariableDef = {key: 'modus', name: 'Modus', type: 'list', entries: ['Auto', 'Manuell'], visible: true};
const text: VariableDef = {key: 'text', name: 'Hinweis', type: 'string', visible: true};
const level = {interface: 'HmIP-RF', address: '0001:4', param: 'LEVEL'};

const markise: ProgramView = {
    program: {key: 'markise', name: 'Markise', active: true, visible: true, operable: true, rules: []},
    problems: [],
    triggers: [
        {kind: 'variable', ref: 'regen', mode: 'change'},
        {kind: 'variable', ref: 'unbekannt', mode: 'update'},
        {kind: 'device', ref: 'HmIP-RF/0001:4/LEVEL', mode: 'update'},
        {kind: 'device', ref: 'x', mode: 'update'},
        {kind: 'time', ref: 'daily', mode: 'update'},
        {kind: 'time', ref: 'weekly', mode: 'update'},
    ],
    script: false,
    time: true,
    state: {key: 'markise'},
};
const overview: Overview = {
    programs: [markise],
    variables: [regen, temp, modus, text].map((def) => ({def, value: null, users: []})),
    alarms: [],
    names: {'HmIP-RF/0001:4': 'Markise Terrasse'},
    notices: {},
    settings: {},
    now: '2026-11-21T06:00:00+01:00',
};
const ctx = context(overview);

describe('the context', () => {
    it('knows the variables, the programs and the channels\' names', () => {
        expect(ctx.variables.regen).toBe(regen);
        expect(ctx.programs.markise).toBe('Markise');
        expect(ctx.names['HmIP-RF/0001:4']).toBe('Markise Terrasse');
        expect(context(null)).toEqual({variables: {}, programs: {}, names: {}});
    });
    it('names a channel by the system\'s name, or by its address', () => {
        expect(channelOf(level)).toBe('HmIP-RF/0001:4');
        expect(refText(level)).toBe('HmIP-RF/0001:4/LEVEL');
        expect(channelName(level, ctx)).toBe('Markise Terrasse');
        expect(channelName({interface: 'BidCos-RF', address: 'LEQ1:1', param: 'STATE'}, ctx)).toBe('LEQ1:1 (BidCos-RF)');
    });
    it('names a system value, which has no file to carry a name', () => {
        expect(variableName(regen)).toBe('Regen');
        expect(variableName({key: 'alarm-messages', name: '', type: 'number', visible: true})).toBe('Alarmmeldungen');
        i18n.language = 'en';
        expect(variableName({key: 'duty-cycle', name: '', type: 'number', visible: true})).toBe('Duty cycle');
    });
});

describe('values', () => {
    it('says a value in its variable\'s words', () => {
        expect(valueText(regen, true)).toBe('es regnet');
        expect(valueText(regen, false)).toBe('trocken');
        expect(valueText({...regen, labels: undefined}, true)).toBe('wahr');
        expect(valueText(undefined, false)).toBe('falsch');
        expect(valueText(temp, 21.5)).toBe('21,5 °C');
        expect(valueText(undefined, 0.1 + 0.2)).toBe('0,3');
        expect(valueText(modus, 'Auto')).toBe('Auto');
        expect(valueText(text, '')).toBe('(leer)');
        expect(valueText(text, null)).toBe('–');
        expect(valueText(text, undefined)).toBe('–');
        i18n.language = 'en';
        expect(valueText(temp, 21.5)).toBe('21.5 °C');
        expect(valueText(undefined, true)).toBe('true');
    });
    it('reads what was typed as the variable\'s type', () => {
        expect(parseValue(regen, 'true')).toEqual({value: true});
        expect(parseValue(regen, true)).toEqual({value: true});
        expect(parseValue({...regen, type: 'alarm'}, 'false')).toEqual({value: false});
        expect(parseValue(temp, ' 21,5 ')).toEqual({value: 21.5});
        expect(parseValue(temp, '')).toEqual({error: 'Bitte eine Zahl eingeben.'});
        expect(parseValue(temp, 'warm')).toEqual({error: 'Bitte eine Zahl eingeben.'});
        expect(parseValue(temp, '-41')).toEqual({error: 'Der kleinste Wert ist -40.'});
        expect(parseValue(temp, '60.5')).toEqual({error: 'Der größte Wert ist 60.'});
        expect(parseValue({...temp, min: undefined, max: undefined}, '1e6')).toEqual({value: 1000000});
        expect(parseValue(modus, 'Manuell')).toEqual({value: 'Manuell'});
        expect(parseValue(modus, 'Winter')).toEqual({error: 'Bitte einen der Einträge wählen.'});
        expect(parseValue({...modus, entries: undefined}, 'x')).toEqual({error: 'Bitte einen der Einträge wählen.'});
        expect(parseValue(text, 'hallo')).toEqual({value: 'hallo'});
    });
    it('writes a duration with its units apart', () => {
        expect(durationText('1h30min')).toBe('1 h 30 min');
        expect(durationText('30s')).toBe('30 s');
        expect(durationText(undefined)).toBe('');
    });
    it('writes a time for the page\'s language, and a dash for none', () => {
        expect(dateTime(undefined, 'de')).toBe('–');
        expect(dateTime('nonsense', 'de')).toBe('–');
        expect(dateTime('0001-01-01T00:00:00Z', 'de')).toBe('–');
        expect(dateTime('2026-11-22T10:15:30Z', 'de')).toMatch(/22\.11\.(20)?26/);
        expect(dateTime('2026-11-22T10:15:30Z', 'en')).toMatch(/22\/11\/2026/);
    });
});

describe('a condition line', () => {
    it('on a variable, in the CCU\'s words', () => {
        expect(lineText({variable: 'regen', op: 'eq', value: true, trigger: 'change'}, ctx)).toEqual({
            kind: 'Systemvariable', subject: 'Regen', param: '', comparison: 'bei es regnet', mode: 'bei Änderung auslösen', triggers: true, not: false, unknown: false,
        });
        const gt = lineText({variable: 'temp', op: 'gt', value: 5, not: true, trigger: 'check'}, ctx);
        expect([gt.comparison, gt.mode, gt.triggers, gt.not]).toEqual(['größer als 5 °C', 'nur prüfen', false, true]);
        expect(lineText({variable: 'temp', op: 'range', from: 5, below: 15, trigger: 'update'}, ctx).comparison).toBe('im Wertebereich von 5 °C bis kleiner 15 °C');
        expect(lineText({variable: 'temp', op: 'gt', value: 5, for: '10min', trigger: 'change'}, ctx).comparison).toBe('größer als 5 °C, seit mindestens 10 min');
        expect(lineText({variable: 'temp', op: 'outside', from: 30, to: 35, trigger: 'value'}, ctx).comparison).toBe('außerhalb des Wertebereichs von 30 °C bis 35 °C');
        expect(lineText({variable: 'temp', op: 'outside', from: 30, to: 35, trigger: 'value'}, ctx).mode).toBe('bei jeder Wertänderung auslösen');
        // a line without an operator is "is"; a variable that is gone is marked
        const gone = lineText({variable: 'weg', value: 'x', trigger: 'check'}, ctx);
        expect([gone.subject, gone.comparison, gone.unknown]).toEqual(['weg', 'bei x', true]);
        expect(lineText({op: 'eq', value: 1, trigger: 'check'}, ctx).subject).toBe('');
        for (const op of ['eq', 'ne', 'gt', 'ge', 'lt', 'le']) {
            expect(lineText({variable: 'temp', op, value: 1, trigger: 'check'}, ctx).comparison).not.toContain('op.');
        }
    });
    it('on a device: the channel by its name, the datapoint beside it', () => {
        const l = lineText({device: level, op: 'gt', value: 0, trigger: 'update'}, ctx);
        expect([l.kind, l.subject, l.param, l.comparison, l.mode, l.unknown]).toEqual(['Geräteauswahl', 'Markise Terrasse', 'LEVEL', 'größer als 0', 'bei Aktualisierung auslösen', false]);
        expect(lineText({device: {interface: 'BidCos-RF', address: 'X:1', param: 'STATE'}, op: 'eq', value: true, trigger: 'check'}, ctx).unknown).toBe(true);
    });
    it('on a time module', () => {
        const l = lineText({time: {kind: 'daily', at: '06:30'}, trigger: 'update'}, ctx);
        expect([l.kind, l.subject, l.mode, l.comparison]).toEqual(['Zeitsteuerung', 'täglich, um 06:30 Uhr', 'zu Zeitpunkten auslösen', '']);
        expect(triggerLabel('check', true)).toBe('nur prüfen');
        // a time module triggers whatever its mode says, as on the CCU: the line is marked, and says why
        expect([l.triggers, l.timeCheck]).toEqual([true, false]);
        const checked = lineText({time: {kind: 'daily', at: '06:30'}, trigger: 'check'}, ctx);
        expect([checked.mode, checked.triggers, checked.timeCheck]).toEqual(['nur prüfen', true, true]);
        expect(lineText({variable: 'regen', op: 'eq', value: true, trigger: 'check'}, ctx).triggers).toBe(false);
        expect(triggerLabel('update')).toBe('bei Aktualisierung auslösen');
    });
    it('says every time module in a sentence', () => {
        expect(timeText({kind: 'interval', at: '00:00', interval: '15min'})).toBe('alle 15 min, ab 00:00 Uhr');
        expect(timeText({kind: 'interval', at: '08:00', interval: '30min', duration: '10h'})).toBe('alle 30 min, ab 08:00 Uhr, für 10 h');
        expect(timeText({kind: 'daily', at: '07:00', every: 2, from: '2026-12-01', until: '2027-03-31'})).toBe('alle 2 Tage, um 07:00 Uhr, gültig ab 2026-12-01, bis 2027-03-31');
        expect(timeText({kind: 'daily', at: '07:15', count: 10})).toBe('täglich, um 07:15 Uhr, 10-mal');
        expect(timeText({kind: 'weekly', at: '22:00', duration: '8h', weekdays: ['mon', 'wed']})).toBe('wöchentlich Mo, Mi, ab 22:00 Uhr, für 8 h');
        expect(timeText({kind: 'weekly', every: 2})).toBe('alle 2 Wochen ');
        expect(timeText({kind: 'monthly', at: '03:00', day: 1})).toBe('monatlich am 1., um 03:00 Uhr');
        expect(timeText({kind: 'monthly', every: 3})).toBe('alle 3 Monate');
        expect(timeText({kind: 'yearly', at: '00:00', day: 24, month: 12})).toBe('jährlich am 24.12., um 00:00 Uhr');
        expect(timeText({kind: 'yearly'})).toBe('jährlich');
        expect(timeText({kind: 'once', at: '12:00', date: '2027-01-01'})).toBe('einmalig am 2027-01-01, um 12:00 Uhr');
        expect(timeText({kind: 'once'})).toBe('einmalig');
        expect(timeText({kind: 'daily', astro: 'before_sunset', offset: '30min', duration: '1h'})).toBe('täglich, vor Sonnenuntergang (30 min), für 1 h');
        expect(timeText({kind: 'daily', astro: 'night'})).toBe('täglich, nachts');
        // what the CCU does not have: an astro point, a twilight
        expect(timeText({kind: 'daily', astro: 'before_sunset', offset: '20min', point: true, twilight: 'civil'})).toBe('täglich, vor Sonnenuntergang (20 min), als Zeitpunkt, bürgerliche Dämmerung');
        for (const tw of ['civil', 'nautical', 'astronomical']) expect(timeText({kind: 'daily', astro: 'night', twilight: tw})).not.toContain('twilight.');
        expect(timeText({kind: 'daily', all_day: true})).toBe('täglich, ganztägig');
        expect(timeText({kind: 'interval', all_day: true, interval: '15min'})).toBe('alle 15 min, ganztägig');
        expect(timeText({kind: 'daily', at: '07:00', weekdays: ['mon', 'tue', 'wed', 'thu', 'fri']})).toBe('werktags, um 07:00 Uhr');
        expect(timeText({kind: 'daily', at: '09:00', weekdays: ['sat', 'sun']})).toBe('am Wochenende, um 09:00 Uhr');
        expect(timeText({kind: 'daily', at: '09:00', weekdays: ['mon', 'sun']})).toBe('Mo, So, um 09:00 Uhr');
        expect(timeText({kind: 'daily', at: '09:00', every: 2, weekdays: ['sat', 'sun'], from: '2026-11-21'})).toBe('alle 2 Tage am Wochenende, um 09:00 Uhr, gültig ab 2026-11-21');
        expect(timeText({kind: 'monthly', at: '08:00', nth: 1, weekdays: ['wed'], every: 6, from: '2020-07-01'})).toBe('alle 6 Monate am 1. Mi, um 08:00 Uhr, gültig ab 2020-07-01');
        expect(timeText({kind: 'yearly', at: '08:00', nth: 4, weekdays: ['thu'], month: 11})).toBe('jährlich am 4. Do im 11. Monat, um 08:00 Uhr');
        i18n.language = 'en';
        expect(timeText({kind: 'weekly', at: '22:00', weekdays: ['sat', 'sun']})).toBe('weekly Sat, Sun, at 22:00');
    });
    it('writes a time as the system\'s clock shows it, whatever zone the browser is in', () => {
        expect(wallTime('2026-11-21T06:30:00+01:00', 'de')).toBe('21.11.26, 06:30');
        expect(wallTime('2026-11-21T06:30:15+01:00', 'de')).toBe('21.11.26, 06:30:15');
        expect(wallTime('2020-03-29T03:00:00+02:00', 'en')).toBe('29/03/2020, 03:00');
        expect(wallTime('2026-06-15T21:31:11-04:00', 'en')).toBe('15/06/2026, 21:31:11');
        expect(wallTime(undefined, 'de')).toBe('–');
        expect(wallTime('gestern', 'de')).toBe('–');
    });
    it('names what a time module does, what caused a run, and counts the lines as the engine does', () => {
        expect(['point', 'begin', 'repeat', 'end'].map(edgeText)).toEqual(['Zeitpunkt', 'Beginn', 'Intervall', 'Ende']);
        expect(runTrigger('time:1:point,2:point', ctx)).toBe('Zeitmodul');
        expect(runTrigger('variable:regen', ctx)).toBe('Regen');
        expect(runTrigger('held:1', ctx)).toBe('eine Bedingung hielt lange genug');
        expect(runTrigger('variable:weg', ctx)).toBe('weg');
        expect(runTrigger('device:HmIP-RF/0001:4/LEVEL', ctx)).toBe('Markise Terrasse LEVEL');
        expect(runTrigger('device:BidCos-RF/X:1/STATE', ctx)).toBe('X:1 STATE');
        expect(runTrigger('program:markise', ctx)).toBe('Programm Markise');
        expect(runTrigger('program:weg', ctx)).toBe('Programm weg');
        expect(runTrigger('user', ctx)).toBe('von Hand');
        expect(runTrigger('outside', ctx)).toBe('von außen');
        const line = {variable: 'regen', trigger: 'check'};
        expect(
            lineNumbers({
                key: 'p',
                name: 'P',
                active: true,
                visible: true,
                operable: true,
                rules: [
                    {when: {op: 'any', groups: [{op: 'all', lines: [line, line]}, {op: 'all', lines: [line]}]}, then: []},
                    {when: {op: 'all', groups: [{op: 'all', lines: [line]}]}, then: []},
                    {else: true, then: []},
                ],
            }),
        ).toEqual([[[1, 2], [3]], [[4]], []]);
    });
});

describe('an activity', () => {
    it('on a variable and on a device', () => {
        expect(actionText({kind: 'variable', variable: 'modus', to: 'Auto'}, ctx)).toEqual({
            kind: 'Systemvariable', subject: 'Modus', param: '', what: 'auf Auto', timing: 'sofort', script: '', skipped: false, unknown: false,
        });
        const delayed = actionText({kind: 'set', device: level, to: 0, after: '1min30s'}, ctx);
        expect([delayed.kind, delayed.subject, delayed.param, delayed.what, delayed.timing]).toEqual(['Geräteauswahl', 'Markise Terrasse', 'LEVEL', 'auf 0', 'verzögert um 1 min 30 s']);
        const random = actionText({kind: 'set', device: level, to: 1, on_for: '5min', after_up_to: '30min'}, ctx);
        expect([random.what, random.timing]).toEqual(['auf 1 für 5 min', 'verzögert um zufällig 0 s bis 30 min']);
        expect(actionText({kind: 'variable', variable: 'modus', to: 'Auto', after: '5min', after_up_to: '1h'}, ctx).timing).toBe('verzögert um zufällig 5 min bis 1 h');
        expect(actionText({kind: 'variable', variable: 'weg', to: 1}, ctx).unknown).toBe(true);
        expect(actionText({kind: 'variable', to: 1}, ctx).subject).toBe('');
        expect(actionText({kind: 'set', to: 1}, ctx).subject).toBe('');
        expect(actionText({kind: 'set', device: {interface: 'BidCos-RF', address: 'X:1', param: 'STATE'}, to: true}, ctx).unknown).toBe(true);
    });
    it('with a value from elsewhere, or a special value', () => {
        expect(actionText({kind: 'variable', variable: 'temp', from_variable: 'temp'}, ctx).what).toBe('auf den Wert von Temperatur');
        expect(actionText({kind: 'variable', variable: 'temp', from_variable: 'weg'}, ctx).what).toBe('auf den Wert von weg');
        expect(actionText({kind: 'variable', variable: 'temp', from_device: level}, ctx).what).toBe('auf den Wert von Markise Terrasse LEVEL');
        const sv = actionText({kind: 'set', device: level, special: '[SV]lastValue'}, ctx);
        expect([sv.what, sv.skipped]).toEqual(['Sonderwert [SV]lastValue: wird übersprungen, bis er bearbeitet ist', true]);
    });
    it('on a program, and a kept script', () => {
        expect(actionText({kind: 'run', program: 'markise', mode: 'then'}, ctx).what).toBe('erstes „Dann“ ausführen');
        expect(actionText({kind: 'run', program: 'markise', mode: 'check'}, ctx).what).toBe('prüfen und ausführen');
        expect(actionText({kind: 'activate', program: 'markise'}, ctx).what).toBe('aktivieren');
        const off = actionText({kind: 'deactivate', program: 'weg'}, ctx);
        expect([off.kind, off.subject, off.what, off.unknown]).toEqual(['Programm', 'weg', 'deaktivieren', true]);
        expect(actionText({kind: 'deactivate'}, ctx).subject).toBe('');
        const script = actionText({kind: 'script', script: 'WriteLine(1);'}, ctx);
        expect([script.kind, script.script, script.skipped]).toEqual(['Script', 'WriteLine(1);', true]);
        expect(actionText({kind: 'script'}, ctx).script).toBe('');
    });
});

describe('a program in the list', () => {
    it('has the CCU\'s headings', () => {
        expect(ruleHeading({then: []}, 0)).toBe('Bedingung: Wenn…');
        expect(ruleHeading({then: []}, 1)).toBe('Sonst, wenn…');
        expect(ruleHeading({else: true, then: []}, 2)).toBe('Sonst…');
        expect([blockText(0), blockText(1), blockText(3)]).toEqual(['Sonst', 'Dann', 'Sonst, wenn 2']);
    });
    it('names what triggers it, each once', () => {
        expect(triggerSummary(markise, ctx)).toEqual(['Regen', 'unbekannt', 'Markise Terrasse LEVEL', ' (x) ', 'Zeitsteuerung']);
    });
    it('has one word for its state', () => {
        expect(programState(markise)).toEqual({label: 'aktiv', kind: 'ok'});
        expect(programState({...markise, program: {...markise.program, active: false}})).toEqual({label: 'inaktiv', kind: ''});
        expect(programState({...markise, problems: ['x']})).toEqual({label: 'fehlerhaft', kind: 'bad'});
        expect(programState({...markise, state: {key: 'markise', paused_until: '2026-01-01T00:01:00Z'}}, Date.parse('2026-01-01T00:00:00Z'))).toEqual({label: 'pausiert', kind: 'warn'});
        expect(programState({...markise, state: {key: 'markise', paused_until: '2026-01-01T00:01:00Z'}}, Date.parse('2026-01-01T00:02:00Z')).label).toBe('aktiv');
    });
    it('carries its marks, what needs attention first', () => {
        expect(programMarks(markise)).toEqual([]);
        const marked = {...markise, script: true, program: {...markise.program, internal: true}, state: {key: 'markise', failures: 2, last_failure: 'refused', no_position: true, no_clock: true}};
        expect(programMarks(marked).map((m) => [m.id, m.label, m.kind])).toEqual([
            ['failures', '2 Befehle abgelehnt', 'bad'],
            ['script', 'Script', 'warn'],
            ['no-location', 'kein Standort', 'warn'],
            ['no-clock', 'Uhr nicht gestellt', 'warn'],
            ['internal', 'systemintern', ''],
        ]);
        expect(programMarks({...markise, state: {key: 'markise', failures: 1}})[0]).toMatchObject({label: '1 Befehl abgelehnt', title: ''});
        expect(marksTitle(programMarks(marked)).split('\n')).toEqual([
            '2 Befehle abgelehnt: refused',
            'Script: Enthält ein Script: Scripts laufen hier nicht.',
            expect.stringMatching(/^kein Standort: Sonnenaufgang/),
            expect.stringMatching(/^Uhr nicht gestellt: Die Uhr/),
            'systemintern',
        ]);
        expect(marksTitle([])).toBe('');
        expect(nameTitle('Markise')).toBe('Markise');
        expect(nameTitle('Markise', 'Terrasse')).toBe('Markise\nTerrasse');
    });
    it('a variable carries its marks', () => {
        const view = {def: regen, value: false, users: []};
        expect(variableMarks(view)).toEqual([]);
        expect(variableMarks({...view, system: true, def: {...regen, internal: true, logged: true}}).map((m) => m.id)).toEqual(['system', 'internal', 'logged']);
        // a device helper's variable: the mark says what it is and on which channel; off comes first
        const kept = {...view, def: {...temp, helper: {kind: 'energy', channel: 'HmIP-RF/000A:6', role: 'total'}}};
        expect(variableMarks(kept).map((m) => m.id)).toEqual(['helper']);
        expect(variableMarks(kept)[0]!.title).toContain('Energie, Gesamtstand, an HmIP-RF/000A:6');
        expect(variableMarks(kept, {variables: {}, programs: {}, names: {'HmIP-RF/000A:6': 'Waschmaschine'}})[0]!.title).toContain('an Waschmaschine.');
        expect(variableMarks({...view, def: {...temp, helper: {kind: 'energy', channel: 'HmIP-RF/000A:6', role: 'total', off: true}}}).map((m) => m.id)).toEqual(['helper-off', 'helper']);
    });
    it('names the system start as what ran it', () => {
        expect(runTrigger('start', ctx)).toBe('Systemstart');
    });
    it('is found by every word of the search', () => {
        expect(matches('', 'Markise')).toBe(true);
        expect(matches('  mark  REGEN ', 'Markise bei', undefined, 'Regen')).toBe(true);
        expect(matches('mark wind', 'Markise bei Regen')).toBe(false);
    });
});

describe('the pause between write commands and the loop guard', () => {
    it('show their defaults when nothing is set, and what is set', () => {
        expect(runtimeFormOf({})).toEqual({pacing: '300', runs: '20', window: '10', pause: '60'});
        expect(runtimeFormOf({pacing_ms: 500, loop_guard: {runs: 5}})).toEqual({pacing: '500', runs: '5', window: '10', pause: '60'});
        expect(runtimeFormOf({loop_guard: {window_seconds: 3, pause_seconds: 120}})).toMatchObject({window: '3', pause: '120'});
    });
    it('store only what differs from the default', () => {
        expect(runtimeOf({pacing: '300', runs: '20', window: '10', pause: '60'})).toEqual({});
        expect(runtimeOf({pacing: ' 500 ', runs: '20', window: '10', pause: '60'})).toEqual({pacing_ms: 500});
        expect(runtimeOf({pacing: '300', runs: '5', window: '3', pause: '120'})).toEqual({loop_guard: {runs: 5, window_seconds: 3, pause_seconds: 120}});
    });
    it('say what is wrong', () => {
        const ok = {pacing: '300', runs: '20', window: '10', pause: '60'};
        expect(runtimeOf({...ok, pacing: '50'}).error).toBe('Pause zwischen Schreibbefehlen: eine ganze Zahl von 100 bis 10000.');
        expect(runtimeOf({...ok, pacing: ''}).error).toContain('Pause zwischen Schreibbefehlen');
        expect(runtimeOf({...ok, runs: '1.5'}).error).toBe('Ausführungen: eine ganze Zahl von 2 bis 1000.');
        expect(runtimeOf({...ok, window: 'x'}).error).toBe('Innerhalb von: eine ganze Zahl von 1 bis 600.');
        expect(runtimeOf({...ok, pause: '100000'}).error).toBe('Pause: eine ganze Zahl von 1 bis 86400.');
    });
});

describe('the variable form', () => {
    it('starts empty, or from a variable', () => {
        expect(emptyForm().type).toBe('bool');
        expect(formOf(temp)).toMatchObject({key: 'temp', name: 'Temperatur', unit: '°C', min: '-40', max: '60', visible: true, internal: false});
        expect(formOf(modus).entries).toEqual([{label: 'Auto', original: 'Auto'}, {label: 'Manuell', original: 'Manuell'}]);
        expect(formOf(regen)).toMatchObject({labelFalse: 'trocken', labelTrue: 'es regnet', min: '', entries: [{label: '', original: ''}]});
        expect(formOf({...text, note: 'n', internal: true, logged: true, channel: 'HmIP-RF/1:1', start_value: 'x'})).toMatchObject({note: 'n', internal: true, logged: true, channel: 'HmIP-RF/1:1', startValue: 'x'});
    });
    it('says what is wrong, field by field', () => {
        expect(formProblems({...emptyForm(), name: ' '})).toEqual({name: 'Eine Variable braucht einen Namen.'});
        expect(formProblems({...emptyForm(), name: 'x'})).toEqual({});
        const list = {...emptyForm(), name: 'x', type: 'list'};
        expect(formProblems(list).entries).toBe('Jeder Eintrag braucht einen Text.');
        expect(formProblems({...list, entries: []}).entries).toBe('Jeder Eintrag braucht einen Text.');
        expect(formProblems({...list, entries: [{label: 'a', original: ''}, {label: ' a ', original: ''}]}).entries).toBe('Jeder Eintrag darf nur einmal vorkommen.');
        expect(formProblems({...list, entries: [{label: 'a;b', original: ''}]}).entries).toBe('Ein Eintrag darf kein Semikolon enthalten.');
        expect(formProblems({...list, entries: [{label: 'a', original: ''}]})).toEqual({});
        const num = {...emptyForm(), name: 'x', type: 'number'};
        expect(formProblems({...num, min: 'a', max: 'b'})).toEqual({min: 'Bitte eine Zahl eingeben.', max: 'Bitte eine Zahl eingeben.'});
        expect(formProblems({...num, min: '10', max: '1'})).toEqual({max: 'Das Maximum liegt unter dem Minimum.'});
        expect(formProblems({...num, min: '1,5', max: ''})).toEqual({});
    });
    it('becomes the definition the API takes, with the renamed entries', () => {
        expect(defOf({...emptyForm(), name: ' Regen ', labelTrue: ' es regnet '})).toEqual({variable: {key: '', name: 'Regen', type: 'bool', visible: true, labels: {false: '', true: 'es regnet'}}, renames: {}});
        expect(defOf({...emptyForm(), name: 'x'}).variable.labels).toBeUndefined();
        expect(defOf({...emptyForm(), name: 'x', type: 'alarm', labelFalse: 'ruhig'}).variable.labels).toEqual({false: 'ruhig', true: ''});
        const list = defOf({...formOf(modus), entries: [{label: 'Auto', original: 'Auto'}, {label: ' Hand ', original: 'Manuell'}, {label: 'Neu', original: ''}]});
        expect(list.variable.entries).toEqual(['Auto', 'Hand', 'Neu']);
        expect(list.renames).toEqual({Manuell: 'Hand'});
        expect(defOf({...formOf(temp), min: '', max: '1,5', unit: ' '}).variable).toEqual({key: 'temp', name: 'Temperatur', type: 'number', visible: true, max: 1.5});
        // a device helper's variable keeps its place in the helper; "off" is the one thing the form changes; it has no start value
        const helper = {kind: 'rain', channel: 'HmIP-RF/000B:1', role: 'total'};
        const form = formOf({...temp, helper, start_value: 3});
        expect(form.helper).toEqual(helper);
        expect(form.helper).not.toBe(helper);
        expect(defOf(form).variable).toEqual({key: 'temp', name: 'Temperatur', type: 'number', visible: true, unit: '°C', min: -40, max: 60, helper});
        expect(defOf({...form, helper: {...helper, off: true}}).variable.helper).toEqual({...helper, off: true});
        expect(defOf({...form, type: 'string'}).variable.helper).toBeUndefined();
        expect(defOf({...formOf(temp), min: 'x'}).variable.min).toBeUndefined();
        expect(defOf({...formOf(text), note: ' n ', internal: true, logged: true, channel: 'HmIP-RF/1:1', startValue: 'x'}).variable).toEqual({
            key: 'text', name: 'Hinweis', type: 'string', visible: true, note: 'n', internal: true, logged: true, channel: 'HmIP-RF/1:1', start_value: 'x',
        });
    });
    it('a computed value: a number with a function and its inputs', () => {
        const tau = {key: 'tau', name: 'Taupunkt', type: 'number', visible: true, unit: '°C',
            computed: {function: 'dew-point', inputs: [{device: {interface: 'HmIP-RF', address: 'A:1', param: 'ACTUAL_TEMPERATURE'}}, {variable: 'temp'}]}};
        const form = formOf(tau);
        expect(form).toMatchObject({type: 'computed', fn: 'dew-point', unit: '°C', inputs: [{kind: 'device', channel: 'HmIP-RF/A:1', param: 'ACTUAL_TEMPERATURE', variable: ''}, {kind: 'variable', channel: '', param: '', variable: 'temp'}]});
        expect(defOf(form).variable).toEqual(tau);
        expect(defOf({...form, unit: ' ', startValue: 3}).variable).toEqual({...tau, unit: undefined});
        expect(formProblems(form)).toEqual({});
        // a plain variable's form has two empty inputs ready
        expect(formOf(temp)).toMatchObject({type: 'number', fn: 'dew-point', inputs: [emptyInput(), emptyInput()]});
        // what is wrong with the inputs
        expect(formProblems({...form, inputs: [form.inputs[0]!]}).inputs).toBe('Diese Funktion nimmt genau 2 Eingänge.');
        expect(formProblems({...form, fn: 'sum', inputs: [form.inputs[0]!]}).inputs).toBe('Diese Funktion nimmt 2 bis 8 Eingänge.');
        expect(formProblems({...form, fn: 'sum', inputs: Array.from({length: 9}, () => form.inputs[1]!)}).inputs).toBe('Diese Funktion nimmt 2 bis 8 Eingänge.');
        expect(formProblems({...form, inputs: [form.inputs[0]!, emptyInput()]}).inputs).toBe('Jeder Eingang braucht einen Datenpunkt oder eine Variable.');
        expect(formProblems({...form, inputs: [{...form.inputs[0]!, param: ' '}, form.inputs[1]!]}).inputs).toBe('Jeder Eingang braucht einen Datenpunkt oder eine Variable.');
        expect(formProblems({...form, inputs: [form.inputs[0]!, {...emptyInput(), kind: 'variable'}]}).inputs).toBe('Jeder Eingang braucht einen Datenpunkt oder eine Variable.');
        expect(formProblems({...form, inputs: [form.inputs[0]!, {...emptyInput(), kind: 'variable', variable: 'tau'}]}).inputs).toBe('Ein berechneter Wert kann nicht sein eigener Eingang sein.');
        // the inputs fit the function
        expect(inputCount('dew-point')).toEqual([2, 2]);
        expect(inputCount('max')).toEqual([2, 8]);
        expect(fitInputs('difference', [])).toEqual([emptyInput(), emptyInput()]);
        expect(fitInputs('difference', [form.inputs[1]!, form.inputs[0]!, form.inputs[1]!])).toEqual([form.inputs[1], form.inputs[0]]);
        expect(fitInputs('sum', Array.from({length: 9}, emptyInput))).toHaveLength(8);
        expect([inputLabel('dew-point', 0), inputLabel('absolute-humidity', 1), inputLabel('difference', 0), inputLabel('difference', 1), inputLabel('sum', 2)]).toEqual(['Temperatur (°C)', 'Relative Luftfeuchte (%)', 'Wert', 'minus', 'Eingang 3']);
        expect(COMPUTED_FUNCTIONS).toHaveLength(7);
        // in words, with the names the page knows
        expect(computedText(tau.computed, ctx)).toBe('Taupunkt aus A:1 (HmIP-RF) ACTUAL_TEMPERATURE, Temperatur');
        expect(computedText({function: 'max', inputs: [{variable: 'weg'}, {}]}, ctx)).toBe('Maximum aus weg, ');
        expect(variableMarks({def: tau, value: 9.3, users: []}).map((m) => m.id)).toEqual(['computed']);
    });
    it('gives a new variable its first value', () => {
        expect([defaultValue('bool', []), defaultValue('number', []), defaultValue('list', ['a']), defaultValue('list', []), defaultValue('string', []), defaultValue('alarm', [])]).toEqual([false, 0, 'a', '', '', undefined]);
    });
});

describe('the import\'s report', () => {
    const codes = {script: {de: 'enthält ein Script', en: 'holds a script'}, 'missing-device': {de: 'Geräte fehlen: %s', en: 'devices missing: %s'}};
    const report: Report = {
        source: 'ccu', kind: 'regadom', location: {},
        programs: [
            {id: 1, name: 'A', status: 'imported', was_active: true, active: true},
            {id: 2, name: 'B', status: 'imported', was_active: false, active: false},
            {id: 3, name: 'C', status: 'inactive', was_active: true, active: false, reasons: [{code: 'script'}]},
            {id: 4, name: 'D', status: 'skipped', was_active: true, active: false},
        ],
        variables: [{id: 5, name: 'V', status: 'imported'}, {id: 6, name: 'W', status: 'skipped'}],
        devices: [{channel: 'a', known: true, programs: []}, {channel: 'b', known: false, programs: []}],
        histogram: [],
    };
    it('fills the importer\'s words with a note\'s arguments', () => {
        expect(noteText({code: 'script'}, codes, 'de')).toBe('enthält ein Script');
        expect(noteText({code: 'missing-device', args: ['a', '', 'b']}, codes, 'de')).toBe('Geräte fehlen: a, b');
        expect(noteText({code: 'missing-device', args: ['a']}, codes, 'en')).toBe('devices missing: a');
        expect(noteText({code: 'unknown', args: ['x']}, codes, 'en')).toBe('unknown x');
        expect(noteText({code: 'unknown'}, codes, 'en')).toBe('unknown');
    });
    it('counts, and says each program\'s result', () => {
        expect(reportTotals(report)).toEqual({imported: 2, inactive: 1, skipped: 1, variables: 1, missing: 1});
        expect(report.programs.map((p) => importStatus(p))).toEqual([
            {label: 'aktiv', kind: 'ok'}, {label: 'inaktiv', kind: ''}, {label: 'inaktiv', kind: 'warn'}, {label: 'nicht übernommen', kind: 'bad'},
        ]);
        expect(reportHasScript(report)).toBe(true);
        expect(reportHasScript({...report, programs: report.programs.slice(0, 2)})).toBe(false);
    });
});

describe('a webhook and a Wake-on-LAN in words', () => {
    const ctx: Context = {variables: {}, programs: {}, names: {}};
    it('say what they call and wake', () => {
        i18n.language = 'en';
        expect(actionText({kind: 'webhook', webhook: {method: 'POST', url: 'https://h/x', body_type: 'json', body: '{}', insecure: true}, after: '5s'}, ctx))
            .toMatchObject({kind: 'Webhook', subject: 'POST https://h/x', what: 'call with a body (JSON), certificate not verified', timing: 'delayed by 5 s'});
        expect(actionText({kind: 'webhook', webhook: {method: 'GET', url: 'http://h/'}}, ctx)).toMatchObject({subject: 'GET http://h/', what: 'call'});
        expect(actionText({kind: 'webhook', webhook: {method: 'PUT', url: 'http://h/', body: 'x'}}, ctx).what).toBe('call with a body (not given)');
        expect(actionText({kind: 'webhook'}, ctx)).toMatchObject({subject: '', what: 'call'});
        expect(actionText({kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22'}}, ctx)).toMatchObject({kind: 'Wake-on-LAN', subject: 'AA:BB:CC:00:11:22', what: 'wake up'});
        expect(actionText({kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22', broadcast: '10.0.0.255'}}, ctx).what).toBe('wake up, through 10.0.0.255');
        expect(actionText({kind: 'wake'}, ctx)).toMatchObject({subject: '', what: 'wake up'});
        expect(actionText({kind: 'channel', channel: 'HmIP-RF/L:8', values: [{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 0.5}, {param: 'ON', value: true}, {param: 'TEXT', value: ''}]}, {...ctx, names: {'HmIP-RF/L:8': 'Taster Flur'}}))
            .toMatchObject({kind: 'Several values', subject: 'Taster Flur', unknown: false, what: 'COLOR = RED, LEVEL = 0.5, ON = true, TEXT = (empty)'});
        expect(actionText({kind: 'channel', channel: 'HmIP-RF/L:8'}, ctx)).toMatchObject({subject: 'HmIP-RF/L:8', unknown: true, what: ''});
        expect(actionText({kind: 'channel'}, ctx)).toMatchObject({subject: '', unknown: true});
    });
});

// The keys a value completes (t('op.' + op) and the like) are not found by the scan of i18n.test.ts:
// every one the code can make is named here.
describe('the keys a value completes', () => {
    const dynamic: Record<string, string[]> = {
        'trigger.': ['change', 'update', 'value', 'check'],
        'op.': ['eq', 'ne', 'gt', 'ge', 'lt', 'le', 'range', 'outside'],
        'day.': ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'],
        'astro.': ['sunrise', 'before_sunrise', 'day', 'sunset', 'before_sunset', 'night'],
        'edge.': ['point', 'begin', 'repeat', 'end'],
        'type.': ['bool', 'list', 'number', 'string', 'alarm', 'computed'],
        'function.': ['dew-point', 'absolute-humidity', 'difference', 'sum', 'average', 'min', 'max'],
        'timekind.': ['once', 'interval', 'daily', 'weekly', 'monthly', 'yearly'],
        'unit.': ['s', 'min', 'h'],
        'system.': ['alarm-messages', 'service-messages', 'duty-cycle', 'sun-azimuth', 'sun-elevation'],
        'twilight.': ['civil', 'nautical', 'astronomical'],
        'decimal.': [''],
        'skip.': ['', 'script', 'special', 'unknown-value', 'no-variable', 'no-program', 'no-writer', 'queue-full'],
        'body.': ['', 'json', 'form', 'text'],
        'why.': ['', 'no-change', 'same-value', 'check-only', 'first-value'],
        'role.': ['total', 'today', 'yesterday'],
        'helpernoun.': HELPER_KINDS,
        'helpertotal.': HELPER_KINDS.filter((k) => k !== 'rain-hm'),
        'entry.': ENTRIES,
        'icon.': DISPLAY_ICONS,
    };
    it('all have both languages', () => {
        for (const [prefix, values] of Object.entries(dynamic)) {
            for (const v of values) {
                const e = MESSAGES[prefix + v] as {de?: string; en?: string} | undefined;
                expect(e?.de, prefix + v).toBeTruthy();
                expect(e?.en, prefix + v).toBeTruthy();
            }
        }
    });
    it('are the only prefixes the page uses', () => {
        const dir = process.cwd() + '/src/';
        const used = new Set<string>();
        for (const f of readdirSync(dir).filter((f) => f.endsWith('.svelte') || (f.endsWith('.ts') && !f.endsWith('.test.ts')))) {
            for (const m of readFileSync(dir + f, 'utf8').matchAll(/\bt\('([a-z]+\.)'/g)) used.add(m[1]!);
        }
        expect([...used].sort()).toEqual(Object.keys(dynamic).sort());
    });
});
