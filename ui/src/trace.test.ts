import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import type {Context} from './model.js';
import {clockTime, filterRuns, mergeRuns, runView, simLines, simRequest, triggerWords, type TraceRun} from './trace.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const ctx: Context = {
    variables: {
        regen: {key: 'regen', name: 'Regen', type: 'bool', visible: true, labels: {true: 'es regnet', false: 'trocken'}},
        hinweis: {key: 'hinweis', name: 'Hinweis', type: 'string', visible: true},
        temp: {key: 'temp', name: 'Temperatur', type: 'number', visible: true, unit: '°C'},
    },
    programs: {markise: 'Markise', nacht: 'Nachtabsenkung'},
    names: {'HmIP-RF/A:1': 'Markise Terrasse'},
};
const T = '2026-11-21T14:02:11+01:00';
const base: TraceRun = {seq: 7, program: 'markise', time: T, trigger: 'variable:regen', block: 1};
const dev = {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'};

describe('what caused an entry', () => {
    it('names the source and the value it brought', () => {
        expect(triggerWords({...base, event: {prev: false, value: true}}, ctx)).toBe('Regen: trocken → es regnet');
        expect(triggerWords({...base, event: {prev: null, value: true, first: true}}, ctx)).toBe('Regen: es regnet');
        expect(triggerWords(base, ctx)).toBe('Regen');
        expect(triggerWords({...base, trigger: 'device:HmIP-RF/A:1/LEVEL', event: {prev: 0, value: 0.5}}, ctx)).toBe('Markise Terrasse LEVEL: 0 → 0.5');
        expect(triggerWords({...base, trigger: 'time:1:point,3:begin'}, ctx)).toBe('time module (line 1; line 3, begin)');
        expect(triggerWords({...base, trigger: 'time'}, ctx)).toBe('time module');
        expect(triggerWords({...base, trigger: 'held:2'}, ctx)).toBe('line 2 held long enough');
        expect(triggerWords({...base, trigger: 'user'}, ctx)).toBe('by hand');
        expect(triggerWords({...base, trigger: 'program:nacht'}, ctx)).toBe('program Nachtabsenkung');
    });
    it('says a time of day', () => {
        expect(clockTime(T)).toBe('14:02:11');
        expect(clockTime(undefined)).toBe('–');
    });
});

describe('an entry in words', () => {
    it('a run with its lines and steps', () => {
        const v = runView({
            ...base,
            event: {prev: false, value: true},
            lines: [
                {line: 1, rule: 1, def: {variable: 'regen', op: 'eq', value: true, trigger: 'change'}, value: true, truth: true},
                {line: 2, rule: 1, def: {device: dev, op: 'gt', value: 0, trigger: 'check', not: true}, unknown: true, truth: false},
                {line: 3, rule: 1, def: {time: {kind: 'daily', at: '06:00'}, trigger: 'update'}, truth: false},
                {line: 4, rule: 1, def: {variable: 'temp', op: 'gt', value: 20, for: '10min', trigger: 'change'}, value: 21, truth: false, since: '2026-11-21T14:00:00+01:00'},
                {line: 5, rule: 1, def: {variable: 'temp', op: 'gt', value: 20, for: '10min', trigger: 'check'}, value: 21, truth: true, since: '2026-11-21T13:00:00+01:00'},
                {line: 6, rule: 2, def: {variable: 'temp', op: 'gt', value: 30, trigger: 'change'}, truth: false, skipped: true},
                {line: 7, rule: 2, def: {variable: 'temp', op: 'gt', value: 30, trigger: 'check'}, truth: false, skipped: true},
            ],
            steps: [
                {action: 0, what: 'cancelled', count: 2},
                {action: 0, what: 'cancelled'},
                {action: 1, def: {kind: 'set', device: dev, to: 0}, what: 'sent', value: 0, confirmed: '2026-11-21T14:02:13+01:00'},
                {action: 2, def: {kind: 'set', device: dev, to: 0}, what: 'sent', value: 0},
                {action: 3, def: {kind: 'variable', variable: 'hinweis', to: 'wegen Regen'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00'},
                {action: 4, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00', ran: '2026-11-21T14:02:41+01:00'},
                {action: 5, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00', ended: '2026-11-21T14:02:20+01:00', ended_by: 'user'},
                {action: 6, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00', ended: '2026-11-21T14:02:20+01:00', ended_by: 'program:nacht'},
                {action: 7, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00', ended: '2026-11-21T14:02:20+01:00', ended_by: 'removed'},
                {action: 8, def: {kind: 'variable', variable: 'temp', from_variable: 'hinweis'}, what: 'written', value: 21},
                {action: 9, def: {kind: 'run', program: 'nacht', mode: 'check'}, what: 'run'},
                {action: 10, def: {kind: 'deactivate', program: 'nacht'}, what: 'switched'},
            ],
        }, ctx, 'en');
        expect(v).toMatchObject({seq: 7, programKey: 'markise', program: 'Markise', trigger: 'Regen: trocken → es regnet', outcome: 'Then', kind: 'ok', detail: '', acted: true, problem: false});
        expect(v.time).toContain('14:02:11');
        expect(v.lines.map((l) => [l.n, l.mark, l.text, l.value, l.verdict, l.kind, l.hint])).toEqual([
            [1, true, 'Regen is es regnet', 'es regnet', 'true', 'ok', ''],
            [2, false, 'Markise Terrasse LEVEL not greater than 0', 'value unknown', 'false', '', 'A line without a value is false.'],
            [3, true, 'daily, at 06:00', '', 'false', '', ''],
            [4, true, 'Temperatur greater than 20 °C, for at least 10 min', '21 °C', 'false', '', 'holds since 14:00:00: not long enough yet'],
            [5, false, 'Temperatur greater than 20 °C, for at least 10 min', '21 °C', 'true', 'ok', 'holds since 13:00:00'],
            [6, true, 'Temperatur greater than 30 °C', '', 'not looked at', 'skip', 'A block before it ran. The line has no remembered state now: its next report triggers the program.'],
            [7, false, 'Temperatur greater than 30 °C', '', 'not looked at', 'skip', 'A block before it ran.'],
        ]);
        expect(v.steps.map((s) => [s.n, s.text, s.status, s.kind])).toEqual([
            ['', '2 running delays for the targets of this block ended', '', ''],
            ['', '0 running delays for the targets of this block ended', '', ''],
            ['1', 'Markise Terrasse LEVEL to 0', 'sent, confirmed 14:02:13', 'ok'],
            ['2', 'Markise Terrasse LEVEL to 0', 'sent', 'wait'],
            ['3', 'Hinweis to wegen Regen', 'scheduled for 14:02:41', 'wait'],
            ['4', 'Hinweis to x', 'scheduled for 14:02:41 · carried out 14:02:41', 'ok'],
            ['5', 'Hinweis to x', 'scheduled for 14:02:41 · ended 14:02:20 (by hand)', ''],
            ['6', 'Hinweis to x', 'scheduled for 14:02:41 · ended 14:02:20 (by program Nachtabsenkung)', ''],
            ['7', 'Hinweis to x', 'scheduled for 14:02:41 · ended 14:02:20 (its program was removed)', ''],
            ['8', 'Temperatur to 21 °C (to the value of Hinweis)', 'written', 'ok'],
            ['9', 'Nachtabsenkung evaluate', 'started', 'ok'],
            ['10', 'Nachtabsenkung switch off', 'switched', 'ok'],
        ]);
    });
    it('what went wrong marks the entry', () => {
        const v = runView({
            ...base,
            program: 'weg',
            block: 0,
            mode: 'then',
            steps: [
                {action: 1, def: {kind: 'set', device: dev, to: 1}, what: 'sent', value: 1, failed: '-1 Failure'},
                {action: 2, def: {kind: 'variable', variable: 'gone', to: 1}, what: 'refused', reason: 'unknown-variable', detail: 'there is no variable gone'},
                {action: 3, def: {kind: 'variable', variable: 'gone', to: 1}, what: 'refused'},
                {action: 4, def: {kind: 'script'}, what: 'skipped', reason: 'script'},
                {action: 5, def: {kind: 'variable', variable: 'hinweis', special: 'OLD'}, what: 'skipped', reason: 'special', detail: 'OLD'},
                {action: 6, def: {kind: 'variable', variable: 'hinweis', from_device: dev}, what: 'skipped', reason: 'unknown-value'},
                {action: 7, def: {kind: 'variable', variable: 'hinweis', from_variable: 'gone'}, what: 'skipped', reason: 'no-variable'},
                {action: 8, def: {kind: 'run', program: 'gone', mode: 'then'}, what: 'skipped', reason: 'no-program'},
                {action: 9, def: {kind: 'set', device: dev, to: 1}, what: 'skipped', reason: 'no-writer'},
                {action: 10, what: 'skipped'},
                {action: 11, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'dropped', due: '2026-11-21T13:00:00+01:00'},
            ],
        }, ctx, 'en');
        expect(v).toMatchObject({program: 'weg', outcome: 'Else', kind: 'bad', detail: 'Run without a look at the conditions.', acted: true, problem: true});
        expect(v.steps.map((s) => s.status)).toEqual([
            'sent, refused: -1 Failure',
            'refused: there is no variable gone',
            'refused: ',
            'skipped: scripts never run here',
            'skipped: this special value is not supported',
            'skipped: the value to take over is not known',
            'skipped: the variable whose value to take over does not exist',
            'skipped: there is no such program',
            'skipped: nothing sends writes to devices',
            'skipped: no reason given',
            'dropped: it was due at 13:00:00, while the addon was stopped',
        ]);
        expect(v.steps.every((s) => s.kind === 'bad')).toBe(true);
        expect(v.steps[3]!.text).toBe('Script is kept as text and never run');
    });
    it('the entries that are no run', () => {
        const kinds = (r: Partial<TraceRun>) => {
            const v = runView({...base, ...r}, ctx, 'en');
            return [v.outcome, v.kind, v.detail, v.acted, v.problem];
        };
        expect(kinds({none: true, untriggered: true, info: 'no-change', count: 1})).toEqual(['not triggered', '', 'The line triggers when changed, and its result (true or false) is the same as before.', false, false]);
        expect(kinds({none: true, untriggered: true, info: 'check-only', count: 3, since: '2026-11-21T14:00:00+01:00'})).toEqual(['not triggered', '', 'The line is "check only": it never triggers. · 3 times since 14:00:00', false, false]);
        expect(kinds({none: true, untriggered: true, info: 'same-value'})[2]).toBe('The line triggers at every change of the value, and the value is the same as before.');
        expect(kinds({none: true, untriggered: true, info: 'first-value'})[2]).toBe('The first value the line sees: it only notes where it stands.');
        expect(kinds({none: true, untriggered: true})[2]).toBe('no reason given');
        expect(kinds({none: true, info: 'paused'})).toEqual(['paused', 'warn', 'The program ran too often and pauses (the loop guard).', false, true]);
        const missed = kinds({none: true, info: 'missed', due: '2026-11-21T06:00:00+01:00'});
        expect(missed.slice(0, 2)).toEqual(['missed', 'warn']);
        expect(missed[2]).toContain('06:00');
        expect(kinds({delayed: true, block: 2})).toEqual(['delayed activity', 'ok', 'A delayed activity of "Else if 1" came due. Nothing is checked again then.', true, false]);
        expect(kinds({none: true})).toEqual(['nothing ran', '', 'The program was evaluated; no condition was true and there is no "Else".', false, false]);
    });
    it('in German', () => {
        i18n.language = 'de';
        const v = runView({...base, none: true, untriggered: true, info: 'no-change', event: {prev: true, value: true}}, ctx, 'de');
        expect([v.trigger, v.outcome]).toEqual(['Regen: es regnet → es regnet', 'nicht ausgelöst']);
        i18n.language = 'en';
    });
});

describe('the list', () => {
    const runs: TraceRun[] = [
        {...base, seq: 3, steps: [{action: 1, def: {kind: 'variable', variable: 'hinweis', to: 'Regen kommt'}, what: 'written'}]},
        {...base, seq: 2, program: 'nacht', none: true, lines: [{line: 1, rule: 1, def: {variable: 'temp', op: 'gt', value: 20, trigger: 'change'}, value: 3, truth: false}]},
        {...base, seq: 1, none: true, info: 'paused'},
    ];
    const views = runs.map((r) => runView(r, ctx, 'en'));
    it('is searched and filtered', () => {
        const seqs = (f: {query?: string; show?: string; program?: string}) => filterRuns(views, f).map((v) => v.seq);
        expect(seqs({})).toEqual([3, 2, 1]);
        expect(seqs({show: 'acted'})).toEqual([3]);
        expect(seqs({show: 'problems'})).toEqual([1]);
        expect(seqs({program: 'nacht'})).toEqual([2]);
        expect(seqs({query: 'regen kommt'})).toEqual([3]);
        expect(seqs({query: 'temperatur'})).toEqual([2]);
        expect(seqs({query: 'nachtabsenkung'})).toEqual([2]);
        expect(seqs({query: 'loop guard'})).toEqual([1]);
        expect(seqs({query: 'xyzzy'})).toEqual([]);
    });
    it('takes older pages and newer answers, each entry once', () => {
        const merged = mergeRuns(runs, [{...base, seq: 3, block: 0}, {...base, seq: 5}, {...base, seq: 0}]);
        expect(merged.map((r) => r.seq)).toEqual([5, 3, 2, 1, 0]);
        expect(merged[1]!.block).toBe(0);
    });
});

describe('a simulation', () => {
    it('says what would be done', () => {
        const v = runView({
            ...base,
            dry: true,
            steps: [
                {action: 0, what: 'cancelled', count: 1},
                {action: 1, def: {kind: 'set', device: dev, to: 0}, what: 'sent', value: 0},
                {action: 2, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'written', value: 'x'},
                {action: 3, def: {kind: 'variable', variable: 'hinweis', to: 'x'}, what: 'scheduled', due: '2026-11-21T14:02:41+01:00'},
                {action: 4, def: {kind: 'run', program: 'nacht', mode: 'then'}, what: 'run'},
                {action: 5, def: {kind: 'activate', program: 'nacht'}, what: 'switched'},
                {action: 6, def: {kind: 'script'}, what: 'skipped', reason: 'script'},
            ],
        }, ctx, 'en');
        expect(v.steps.map((s) => [s.text, s.status, s.kind])).toEqual([
            ['1 running delay for the targets of this block would be ended', '', ''],
            ['Markise Terrasse LEVEL to 0', 'would be sent', 'wait'],
            ['Hinweis to x', 'would be written', 'wait'],
            ['Hinweis to x', 'would be scheduled for 14:02:41', 'wait'],
            ['Nachtabsenkung run the first "Then"', 'would be started', 'wait'],
            ['Nachtabsenkung switch on', 'would be switched', 'wait'],
            ['Script is kept as text and never run', 'skipped: scripts never run here', 'bad'],
        ]);
        expect(runView({...base, dry: true, none: true, untriggered: true, info: 'check-only'}, ctx, 'en').outcome).toBe('would not trigger');
    });
    it('offers the lines of the program, each with the way its value is entered', () => {
        const lines = simLines({key: 'p', name: 'P', active: true, visible: true, operable: true, rules: [
            {when: {op: 'any', groups: [
                {op: 'all', lines: [{variable: 'regen', op: 'eq', value: true, trigger: 'change'}, {device: dev, op: 'range', from: 0.2, below: 0.8, trigger: 'check'}]},
                {op: 'all', lines: [{time: {kind: 'daily', at: '06:00'}, trigger: 'update'}]},
            ]}, then: []},
            {when: {op: 'all', groups: [{op: 'all', lines: [{variable: 'hinweis', op: 'ne', trigger: 'check'}]}]}, then: []},
            {else: true, then: []},
        ]}, ctx);
        expect(lines.map((l) => [l.n, l.label, l.kind?.kind ?? null, l.value])).toEqual([
            [1, '1: Regen is es regnet', 'bool', true],
            [2, '2: Markise Terrasse LEVEL in the range from 0.2 to less than 0.8', 'free', 0.2],
            [3, '3: daily, at 06:00', null, undefined],
            [4, '4: Hinweis is not –', 'text', ''],
        ]);
        expect(simRequest(undefined, 5)).toEqual({});
        expect(simRequest(lines[0], false)).toEqual({line: 1, value: false});
        expect(simRequest(lines[2], 'x')).toEqual({line: 3});
    });
});

describe('webhooks and Wake-on-LAN in the trace', () => {
    const hook = {kind: 'webhook', webhook: {method: 'GET', url: 'http://h/x'}};
    const wake = {kind: 'wake', wake: {mac: 'AA:BB:CC:00:11:22'}};
    it('say what became of a call', () => {
        const v = runView({
            ...base,
            steps: [
                {action: 1, def: hook, what: 'called'},
                {action: 2, def: hook, what: 'called', confirmed: '2026-11-21T14:02:12+01:00', status: 204, took: 87},
                {action: 3, def: hook, what: 'called', failed: 'HTTP 500', status: 500, took: 1200},
                {action: 4, def: hook, what: 'called', failed: 'the secret token is not set'},
                {action: 5, def: wake, what: 'called', confirmed: '2026-11-21T14:02:12+01:00', took: 200},
                {action: 6, def: hook, what: 'called', confirmed: '2026-11-21T14:02:12+01:00'},
                {action: 7, def: hook, what: 'skipped', reason: 'queue-full'},
            ],
        }, ctx, 'en');
        expect(v.steps.map((s) => [s.text, s.status, s.kind])).toEqual([
            ['GET http://h/x call', 'under way', 'wait'],
            ['GET http://h/x call', 'answered 204 after 87 ms', 'ok'],
            ['GET http://h/x call', 'failed: HTTP 500 · 1200 ms', 'bad'],
            ['GET http://h/x call', 'failed: the secret token is not set', 'bad'],
            ['AA:BB:CC:00:11:22 wake up', 'sent ' + clockTime('2026-11-21T14:02:12+01:00'), 'ok'],
            ['GET http://h/x call', 'answered 0 after 0 ms', 'ok'],
            ['GET http://h/x call', 'skipped: too many calls were waiting, this one was dropped', 'bad'],
        ]);
    });
    it('and what a simulation would do', () => {
        const v = runView({...base, dry: true, steps: [{action: 1, def: hook, what: 'called'}, {action: 2, def: wake, what: 'called'}]}, ctx, 'en');
        expect(v.steps.map((s) => [s.status, s.kind])).toEqual([['would be called', 'wait'], ['would be sent', 'wait']]);
    });
});
