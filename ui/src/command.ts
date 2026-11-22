// The "command" activity: a program of the system, run with the addon's rights. Without a shell
// it is a list - the program, then its arguments, each handed over as it is; with one it is a
// line for /bin/sh, and the arguments are its "$1", "$2", …. An argument is a text with the
// placeholders a webhook has, or the value of a variable or of a device's datapoint.
import {t} from 'occulite-ui/i18n';

import type {Command, CommandArg, Context} from './model.js';

/** One argument as the dialog has it. */
export interface ArgForm {
    id: number;
    kind: 'text' | 'variable' | 'device';
    text: string;
    variable: string;
    /** "Interface/ADDRESS:CH" and the datapoint. */
    channel: string;
    param: string;
    format: string;
}

/** A command as the dialog has it. */
export interface CommandForm {
    shell: boolean;
    line: string;
    /** Without a shell the first is the program. */
    args: ArgForm[];
    /** The timeout in seconds, as typed. */
    timeout: string;
    wait: boolean;
    output: string;
    exitCode: string;
}

export const MAX_ARGS = 64;
export const DEFAULT_TIMEOUT = 10;
export const MAX_TIMEOUT = 600;
/** The formats of a value: as it is, a number's decimals, a logic value as 1/0, a list's entry as its number. */
export const NUMBER_FORMATS = ['', '.0', '.1', '.2', '.3', '.4', '.5', '.6'];

let ids = 0;

export function newArg(kind: ArgForm['kind'] = 'text', text = ''): ArgForm {
    return {id: ++ids, kind, text, variable: '', channel: '', param: '', format: ''};
}

/** A new command: no shell, the program still to be named, in the background. */
export const newCommand = (): CommandForm => ({shell: false, line: '', args: [newArg()], timeout: '', wait: false, output: '', exitCode: ''});

const seconds = (d?: string) => {
    const m = /^(?:(\d+)h)?(?:(\d+)min)?(?:(\d+)s)?$/.exec(d ?? '');
    return m && d ? Number(m[1] ?? 0) * 3600 + Number(m[2] ?? 0) * 60 + Number(m[3] ?? 0) : 0;
};

/** A stored command as the dialog has it. */
export function commandForm(c: Command | undefined): CommandForm {
    if (!c) return newCommand();
    const args = (c.args ?? []).map((a) => {
        const out = newArg(a.device ? 'device' : a.variable ? 'variable' : 'text', a.text ?? '');
        if (a.device) Object.assign(out, {channel: `${a.device.interface}/${a.device.address}`, param: a.device.param});
        out.variable = a.variable ?? '';
        out.format = a.format ?? '';
        return out;
    });
    if (!c.shell && args.length === 0) args.push(newArg());
    return {shell: !!c.shell, line: c.line ?? '', args, timeout: c.timeout ? String(seconds(c.timeout)) : '', wait: !!c.wait, output: c.output ?? '', exitCode: c.exit_code ?? ''};
}

function argOf(a: ArgForm): CommandArg {
    if (a.kind === 'device') {
        const [iface = '', address = ''] = a.channel.split('/');
        return {device: {interface: iface, address, param: a.param}, ...(a.format ? {format: a.format} : {})};
    }
    if (a.kind === 'variable') return {variable: a.variable, ...(a.format ? {format: a.format} : {})};
    return {text: a.text};
}

/** The command a form says, as the API takes it. */
export function commandOf(f: CommandForm): Command {
    const out: Command = {};
    if (f.shell) Object.assign(out, {shell: true, line: f.line});
    const args = f.args.map(argOf);
    if (args.length > 0) out.args = args;
    const timeout = Number(f.timeout.trim());
    if (f.timeout.trim() && Number.isInteger(timeout) && timeout > 0) out.timeout = timeout % 60 === 0 ? `${timeout / 60}min` : `${timeout}s`;
    if (f.wait) out.wait = true;
    if (f.output) out.output = f.output;
    if (f.exitCode) out.exit_code = f.exitCode;
    return out;
}

/** Whether a command names nothing to run yet. */
export const commandEmpty = (f: CommandForm) => (f.shell ? !f.line.trim() : !f.args[0] || f.args[0].kind !== 'text' || !f.args[0].text.trim());

/** The formats a value of a kind can have. */
export function formatsFor(kind: string): string[] {
    if (kind === 'number') return NUMBER_FORMATS;
    if (kind === 'bool' || kind === 'alarm') return ['', '01'];
    if (kind === 'list') return ['', 'index'];
    return [''];
}

/** A format in words. */
export function formatLabel(format: string): string {
    if (format === '') return t('as it is');
    if (format === '01') return t('as 1 or 0');
    if (format === 'index') return t('the entry\'s number');
    return t('{n} decimals', {n: Number(format.slice(1))}, Number(format.slice(1)));
}

const PLACEHOLDER = /\{(program|trigger|value|time|var:[a-z0-9-]+|secret:[a-z0-9-]+)\}/g;

/** The placeholders a command names in its line and its texts, each once. */
export function commandPlaceholders(f: CommandForm): string[] {
    const texts = [f.shell ? f.line : '', ...f.args.filter((a) => a.kind === 'text').map((a) => a.text)];
    return [...new Set(texts.flatMap((text) => [...text.matchAll(PLACEHOLDER)].map((m) => m[1]!)))];
}

/** The kind of value a datapoint brings, for its formats: number, bool or '' (anything else). */
export function paramKind(type: string | undefined): string {
    if (type === 'FLOAT' || type === 'INTEGER') return 'number';
    return type === 'BOOL' || type === 'ACTION' ? 'bool' : '';
}

/**
 * What is wrong with a command; '' when it can be saved. secrets are the names entered in the
 * settings (undefined: not known yet).
 */
export function commandProblem(f: CommandForm, ctx: Context, secrets?: string[]): string {
    if (f.shell) {
        if (!f.line.trim()) return t('Enter the line the shell runs.');
        if (new TextEncoder().encode(f.line).length > 8000) return t('The line is longer than {n} bytes.', {n: 8000});
    } else {
        const program = f.args[0];
        if (!program || program.kind !== 'text' || !program.text.trim()) return t('Enter the program to run.');
        if (PLACEHOLDER.test(program.text)) {
            PLACEHOLDER.lastIndex = 0;
            return t('The program is written out: a placeholder cannot stand in it.');
        }
    }
    if (f.args.length > MAX_ARGS) return t('A command has at most {n} arguments.', {n: MAX_ARGS});
    for (const [i, a] of f.args.entries()) {
        const n = f.shell ? i + 1 : i;
        if (a.kind === 'variable') {
            if (!a.variable) return t('Argument {n}: choose a variable.', {n});
            const def = ctx.variables[a.variable];
            if (!def) return t('The variable {key} does not exist.', {key: a.variable});
            if (!formatsFor(def.type).includes(a.format)) return t('Argument {n}: this format does not fit the variable.', {n});
        } else if (a.kind === 'device') {
            if (!a.channel) return t('Argument {n}: choose a device.', {n});
            if (!a.param) return t('Argument {n}: choose a datapoint.', {n});
            if (a.format === 'index') return t('Argument {n}: this format does not fit the variable.', {n});
        } else if (new TextEncoder().encode(a.text).length > 4000) {
            return t('Argument {n} is longer than {max} bytes.', {n, max: 4000});
        }
    }
    const timeout = f.timeout.trim();
    if (timeout && !(/^\d+$/.test(timeout) && Number(timeout) >= 1 && Number(timeout) <= MAX_TIMEOUT)) return t('The timeout is a whole number of seconds from 1 to {max}.', {max: MAX_TIMEOUT});
    if (f.output) {
        const def = ctx.variables[f.output];
        if (!def) return t('The variable {key} does not exist.', {key: f.output});
        if (def.computed || !['string', 'number', 'bool'].includes(def.type)) return t('The output goes into a text, a number or a logic value.');
    }
    if (f.exitCode) {
        const def = ctx.variables[f.exitCode];
        if (!def) return t('The variable {key} does not exist.', {key: f.exitCode});
        if (def.computed || def.type !== 'number') return t('The exit code goes into a number.');
        if (f.exitCode === f.output) return t('The output and the exit code need two variables.');
    }
    for (const p of commandPlaceholders(f)) {
        const [kind, name] = p.split(':');
        if (kind === 'var' && !ctx.variables[name!]) return t('The variable {key} does not exist.', {key: name!});
        if (kind === 'secret' && secrets && !secrets.includes(name!)) return t('There is no secret {name}: enter it in the settings first.', {name: name!});
    }
    return '';
}

const word = (s: string) => (s !== '' && !/[\s"'\\$`|&;<>()*?#~]/.test(s) ? s : "'" + s.replace(/'/g, "'\\''") + "'");

/** A command in one line, as a list and a history say it: values by their names in angle brackets. */
export function commandText(c: Command | undefined, ctx: Context): string {
    if (!c) return '';
    const args = (c.args ?? []).map((a) => {
        if (a.device) return `<${ctx.names[`${a.device.interface}/${a.device.address}`] ?? a.device.address} ${a.device.param}>`;
        if (a.variable) return `<${ctx.variables[a.variable]?.name ?? a.variable}>`;
        return word(a.text ?? '');
    });
    return [c.shell ? (c.line ?? '') : '', ...args].filter((s) => s !== '').join(' ');
}

/** What a command does besides running, in words: shell, waits, where its output goes. */
export function commandWhat(c: Command | undefined, ctx: Context): string {
    if (!c) return '';
    const name = (key: string) => ctx.variables[key]?.name ?? key;
    return [
        c.shell ? t('through a shell') : '',
        c.wait ? t('the block waits for it') : t('in the background'),
        c.timeout ? t('at most {d}', {d: c.timeout.replace(/(\d+)(min|s|h)/g, '$1 $2 ').trim()}) : '',
        c.output ? t('output into {name}', {name: name(c.output)}) : '',
        c.exit_code ? t('exit code into {name}', {name: name(c.exit_code)}) : '',
    ].filter(Boolean).join(', ');
}
