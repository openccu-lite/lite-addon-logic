import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {commandEmpty, commandForm, commandOf, commandPlaceholders, commandProblem, commandText, commandWhat, formatLabel, formatsFor, newArg, newCommand, paramKind, type CommandForm} from './command.js';
import {draftChannels, draftOf, draftProblems, newAction, newDraft, programOf, type Env} from './editor.js';
import {MESSAGES} from './messages.js';
import {actionText, type Command, type Context, type Program} from './model.js';
import {runView} from './trace.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const ctx = {
    variables: {
        temp: {key: 'temp', name: 'Temperatur', type: 'number'},
        text: {key: 'text', name: 'Text', type: 'string'},
        an: {key: 'an', name: 'An', type: 'bool'},
        modus: {key: 'modus', name: 'Modus', type: 'list', entries: ['a', 'b']},
        alarm: {key: 'alarm', name: 'Alarm', type: 'alarm'},
        taupunkt: {key: 'taupunkt', name: 'Taupunkt', type: 'number', computed: {function: 'dew-point', inputs: []}},
    },
    programs: {},
    names: {'HmIP-RF/A:1': 'Thermostat'},
} as unknown as Context;
const env: Env = {ctx, params: {}, secrets: ['token']};

const stored: Command = {
    args: [{text: '/usr/bin/curl'}, {text: '-s'}, {text: 'http://h/?v={var:temp}&k={secret:token}'}, {variable: 'temp', format: '.1'}, {device: {interface: 'HmIP-RF', address: 'A:1', param: 'LEVEL'}, format: '01'}, {variable: 'text'}],
    timeout: '2min', wait: true, output: 'text', exit_code: 'temp',
};
const shell: Command = {shell: true, line: 'echo {var:text} "$1" | tr a b', args: [{variable: 'modus', format: 'index'}], timeout: '45s'};

describe('the command as the form and as the file', () => {
    it('reads a stored command into the form and writes the same back', () => {
        const form = commandForm(stored);
        expect(form).toMatchObject({shell: false, line: '', timeout: '120', wait: true, output: 'text', exitCode: 'temp'});
        expect(form.args.map((a) => a.kind)).toEqual(['text', 'text', 'text', 'variable', 'device', 'variable']);
        expect(form.args[4]).toMatchObject({channel: 'HmIP-RF/A:1', param: 'LEVEL', format: '01'});
        expect(commandOf(form)).toEqual(stored);
        expect(commandOf(commandForm(shell))).toEqual(shell);
        expect(commandForm(shell).timeout).toBe('45');
    });
    it('starts a new command with the program\'s row, in the background', () => {
        expect(commandForm(undefined)).toMatchObject({shell: false, wait: false, timeout: '', output: '', exitCode: ''});
        expect(commandForm(undefined).args).toHaveLength(1);
        expect(commandForm({}).args).toHaveLength(1);
        expect(commandForm({shell: true, line: 'ls'}).args).toHaveLength(0);
        expect(commandOf(newCommand())).toEqual({args: [{text: ''}]});
        expect([newCommand(), {...newCommand(), args: []}, {...newCommand(), args: [newArg('variable')]}, {...newCommand(), shell: true, line: ' '}, commandForm(stored), commandForm(shell)].map(commandEmpty)).toEqual([true, true, true, true, false, false]);
        expect(commandOf({...newCommand(), shell: true, line: 'ls', args: [], timeout: 'x'})).toEqual({shell: true, line: 'ls'});
        expect(commandOf({...newCommand(), timeout: ' 90 '}).timeout).toBe('90s');
        expect(commandOf({...newCommand(), timeout: '0'}).timeout).toBeUndefined();
        expect(commandOf({...newCommand(), args: [newArg('text', 'x'), {...newArg('variable'), variable: 'an'}, {...newArg('device'), channel: 'I/A:1', param: 'P'}]}).args)
            .toEqual([{text: 'x'}, {variable: 'an'}, {device: {interface: 'I', address: 'A:1', param: 'P'}}]);
        expect(commandOf({...newCommand(), args: [newArg('text', 'x'), newArg('device')]}).args![1]).toEqual({device: {interface: '', address: '', param: ''}});
    });
    it('offers the formats a value can have, in words', () => {
        expect(formatsFor('number')).toHaveLength(8);
        expect(formatsFor('bool')).toEqual(['', '01']);
        expect(formatsFor('alarm')).toEqual(['', '01']);
        expect(formatsFor('list')).toEqual(['', 'index']);
        expect(formatsFor('string')).toEqual(['']);
        expect(['', '01', 'index', '.1', '.3'].map(formatLabel)).toEqual(['as it is', 'as 1 or 0', 'the entry\'s number', '1 decimal', '3 decimals']);
        expect(['FLOAT', 'INTEGER', 'BOOL', 'ACTION', 'ENUM', undefined].map(paramKind)).toEqual(['number', 'number', 'bool', 'bool', '', '']);
    });
    it('names the placeholders of the line and the texts', () => {
        expect(commandPlaceholders(commandForm(stored))).toEqual(['var:temp', 'secret:token']);
        expect(commandPlaceholders(commandForm(shell))).toEqual(['var:text']);
        // a line that is not used names nothing
        expect(commandPlaceholders({...commandForm(stored), line: '{value}'})).toEqual(['var:temp', 'secret:token']);
    });
});

describe('what is wrong with a command', () => {
    const form = (change: (f: CommandForm) => void, from = stored) => {
        const f = commandForm(from);
        change(f);
        return commandProblem(f, ctx, env.secrets);
    };
    it('is said, one thing at a time', () => {
        expect(form(() => undefined)).toBe('');
        expect(form(() => undefined, shell)).toBe('');
        expect(form((f) => (f.args[0]!.text = ' '))).toBe('Enter the program to run.');
        expect(form((f) => (f.args = []))).toBe('Enter the program to run.');
        expect(form((f) => (f.args[0]!.kind = 'variable'))).toBe('Enter the program to run.');
        expect(form((f) => (f.args[0]!.text = '/bin/{value}'))).toBe('The program is written out: a placeholder cannot stand in it.');
        // (asked twice: the answer is the same)
        expect(form((f) => (f.args[0]!.text = '/bin/{value}'))).toBe('The program is written out: a placeholder cannot stand in it.');
        expect(form((f) => (f.line = ' '), shell)).toBe('Enter the line the shell runs.');
        expect(form((f) => (f.line = 'x'.repeat(8001)), shell)).toBe('The line is longer than 8000 bytes.');
        expect(form((f) => (f.args = Array.from({length: 65}, () => newArg('text', 'x'))))).toBe('A command has at most 64 arguments.');
        expect(form((f) => (f.args[3]!.variable = ''))).toBe('Argument 3: choose a variable.');
        expect(form((f) => (f.args[3]!.variable = 'weg'))).toBe('The variable weg does not exist.');
        expect(form((f) => (f.args[3]!.format = '01'))).toBe('Argument 3: this format does not fit the variable.');
        expect(form((f) => (f.args[4]!.channel = ''))).toBe('Argument 4: choose a device.');
        expect(form((f) => (f.args[4]!.param = ''))).toBe('Argument 4: choose a datapoint.');
        expect(form((f) => (f.args[4]!.format = 'index'))).toBe('Argument 4: this format does not fit the variable.');
        expect(form((f) => (f.args[1]!.text = 'ä'.repeat(2001)))).toBe('Argument 1 is longer than 4000 bytes.');
        // with a shell the values count from 1
        expect(form((f) => (f.args[0]!.variable = ''), shell)).toBe('Argument 1: choose a variable.');
        for (const bad of ['0', '601', '1.5', 'x', '-1']) expect(form((f) => (f.timeout = bad))).toBe('The timeout is a whole number of seconds from 1 to 600.');
        expect(form((f) => (f.timeout = ' 600 '))).toBe('');
        expect(form((f) => (f.output = 'weg'))).toBe('The variable weg does not exist.');
        expect(form((f) => (f.output = 'alarm'))).toBe('The output goes into a text, a number or a logic value.');
        expect(form((f) => (f.output = 'taupunkt'))).toBe('The output goes into a text, a number or a logic value.');
        expect(form((f) => (f.output = 'an'))).toBe('');
        expect(form((f) => (f.exitCode = 'weg'))).toBe('The variable weg does not exist.');
        expect(form((f) => (f.exitCode = 'text'))).toBe('The exit code goes into a number.');
        expect(form((f) => (f.exitCode = 'taupunkt'))).toBe('The exit code goes into a number.');
        expect(form((f) => (f.output = 'temp'))).toBe('The output and the exit code need two variables.');
        expect(form((f) => (f.args[1]!.text = '{var:weg}'))).toBe('The variable weg does not exist.');
        expect(form((f) => (f.args[1]!.text = '{secret:nope}'))).toBe('There is no secret nope: enter it in the settings first.');
        // the secrets are not known yet: nothing is said about them
        const f = commandForm(stored);
        f.args[1]!.text = '{secret:nope}';
        expect(commandProblem(f, ctx)).toBe('');
    });
});

describe('a command in words', () => {
    it('says the line with the values by their names, and what becomes of it', () => {
        expect(commandText(stored, ctx)).toBe('/usr/bin/curl -s \'http://h/?v={var:temp}&k={secret:token}\' <Temperatur> <Thermostat LEVEL> <Text>');
        expect(commandWhat(stored, ctx)).toBe('the block waits for it, at most 2 min, output into Text, exit code into Temperatur');
        expect(commandText(shell, ctx)).toBe('echo {var:text} "$1" | tr a b <Modus>');
        expect(commandWhat(shell, ctx)).toBe('through a shell, in the background, at most 45 s');
        expect(commandText({args: [{text: 'x'}, {text: ''}, {text: 'it\'s'}, {variable: 'weg'}, {device: {interface: 'I', address: 'B:2', param: 'P'}}]}, ctx)).toBe('x \'\' \'it\'\\\'\'s\' <weg> <B:2 P>');
        expect(commandWhat({args: [{text: 'x'}], output: 'weg', exit_code: 'fort'}, ctx)).toBe('in the background, output into weg, exit code into fort');
        expect(commandText(undefined, ctx)).toBe('');
        expect(commandWhat(undefined, ctx)).toBe('');
        expect(commandText({shell: true}, ctx)).toBe('');
        expect(actionText({kind: 'command', command: shell, after: '5s'}, ctx)).toMatchObject({kind: 'Command', subject: 'echo {var:text} "$1" | tr a b <Modus>', what: 'through a shell, in the background, at most 45 s', timing: 'delayed by 5 s'});
    });
});

describe('a command in the editor', () => {
    const program = {key: 'p', name: 'P', active: true, visible: true, operable: true, rules: [{when: {op: 'any', groups: []}, then: [{kind: 'command', command: stored}, {kind: 'command', command: shell, after: '5s'}]}]} as unknown as Program;
    it('is read into the draft and written back', () => {
        const d = draftOf(program);
        expect(d.rules[0]!.then.map((a) => a.kind)).toEqual(['command', 'command']);
        expect(programOf(d).rules[0]!.then).toEqual(program.rules[0]!.then);
        expect(draftChannels(d)).toEqual(['HmIP-RF/A:1']);
        expect(draftProblems(d, env)).toEqual({});
    });
    it('says what is wrong with it on its row', () => {
        const d = newDraft();
        d.name = 'P';
        const a = newAction('command');
        d.rules[0]!.then.push(a);
        expect(Object.values(draftProblems(d, env))).toEqual(['Enter the program to run.']);
        a.command.args[0]!.text = 'true';
        expect(draftProblems(d, env)).toEqual({});
        expect(programOf(d).rules[0]!.then).toEqual([{kind: 'command', command: {args: [{text: 'true'}]}}]);
    });
});

describe('a command in the trace', () => {
    const base = {seq: 1, program: 'p', time: '2026-11-21T14:02:10+01:00', trigger: 'user', block: 1};
    const cmd = {kind: 'command', command: {args: [{text: 'curl'}, {variable: 'temp'}], wait: true}};
    const bg = {kind: 'command', command: {args: [{text: 'curl'}]}};
    it('says the line as it was run and what became of it', () => {
        const v = runView({
            ...base,
            steps: [
                {action: 1, def: cmd, what: 'called', value: 'curl 21.5'},
                {action: 2, def: bg, what: 'called', value: 'curl'},
                {action: 3, def: cmd, what: 'called', value: 'curl 21.5', confirmed: '2026-11-21T14:02:12+01:00', took: 87, output: 'ok'},
                {action: 4, def: cmd, what: 'called', value: 'curl 21.5', failed: 'exit code 3: kaputt', status: 3, took: 1200, output: 'halb'},
                {action: 5, def: cmd, what: 'called', value: 'curl 21.5', confirmed: '2026-11-21T14:02:12+01:00', detail: 'temp does not take the output "x"'},
                {action: 6, def: cmd, what: 'skipped', reason: 'queue-full'},
                {action: 7, def: cmd, what: 'skipped', reason: 'unknown-value', detail: 'HmIP-RF/A:1/LEVEL'},
            ],
        } as never, ctx, 'en');
        expect(v.steps.map((s) => [s.text, s.status, s.kind])).toEqual([
            ['Command curl 21.5', 'running, the block waits', 'wait'],
            ['Command curl', 'running', 'wait'],
            ['Command curl 21.5', 'ended after 87 ms · output: ok', 'ok'],
            ['Command curl 21.5', 'failed: exit code 3: kaputt · 1200 ms · output: halb', 'bad'],
            ['Command curl 21.5', 'ended after 0 ms · temp does not take the output "x"', 'bad'],
            ['Command curl <Temperatur>', 'skipped: too many calls were waiting, this one was dropped', 'bad'],
            ['Command curl <Temperatur>', expect.stringContaining('skipped'), expect.any(String)],
        ]);
    });
    it('and what a simulation would do', () => {
        const v = runView({...base, dry: true, steps: [{action: 1, def: cmd, what: 'called', value: 'curl 21.5'}]} as never, ctx, 'en');
        expect(v.steps.map((s) => [s.text, s.status, s.kind])).toEqual([['Command curl 21.5', 'would be run', 'wait']]);
    });
});
