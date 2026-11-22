// What the page knows about programs, variables and the import's report, and how it says it:
// the types the API answers with, and the words for a condition line, an activity, a value, a
// time module. No component logic here - the components only lay out what these functions say.
import {t} from 'occulite-ui/i18n';

import {commandText, commandWhat} from './command.js';
import {displayText} from './display.js';
import {helperTitle, type Helper} from './helpers.js';

export interface Ref {
    interface: string;
    address: string;
    param: string;
}

export interface TimeModule {
    kind: string;
    at?: string;
    all_day?: boolean;
    astro?: string;
    offset?: string;
    /** An astro module as a point instead of a range; the twilight its "sunrise" and "sunset" mean. */
    point?: boolean;
    twilight?: string;
    duration?: string;
    interval?: string;
    every?: number;
    weekdays?: string[];
    day?: number;
    nth?: number;
    month?: number;
    date?: string;
    from?: string;
    until?: string;
    count?: number;
}

export type Value = boolean | number | string | null | undefined;

export interface Line {
    device?: Ref;
    variable?: string;
    time?: TimeModule;
    op?: string;
    value?: Value;
    from?: Value;
    below?: Value;
    to?: Value;
    not?: boolean;
    /** "Hält seit": true only once the comparison has held this long. */
    for?: string;
    trigger: string;
    note?: string;
}

/** A webhook's request. */
export interface Webhook {
    method: string;
    url: string;
    headers?: {name: string; value: string}[];
    body_type?: string;
    body?: string;
    /** The server's certificate is not verified. */
    insecure?: boolean;
}

/** A Wake-on-LAN's target. */
export interface Wake {
    mac: string;
    broadcast?: string;
}

/** One argument of a command: a text, or the value of a variable or of a device's datapoint. */
export interface CommandArg {
    text?: string;
    variable?: string;
    device?: Ref;
    /** How the value is written: '' as it is, .0 to .6 a number's decimals, 01, index. */
    format?: string;
}

/** A "command" activity: a program of the system, run with the addon's rights. */
export interface Command {
    /** The line is run by /bin/sh; the arguments are its "$1", "$2", …. */
    shell?: boolean;
    line?: string;
    args?: CommandArg[];
    timeout?: string;
    /** The block's following activities wait until it has ended. */
    wait?: boolean;
    /** The variables its output and its exit code go into. */
    output?: string;
    exit_code?: string;
}

/** One line of a display: what is left out is the default (centred, black on white). */
export interface DisplayLine {
    text?: string;
    icon?: string;
    align?: string;
    color?: string;
    background?: string;
}

/** What a "display" activity shows on its channel: up to five lines and an optional signal tone. */
export interface Display {
    lines?: DisplayLine[];
    sound?: string;
    repetitions?: number;
    interval?: number;
}

export interface Action {
    kind: string;
    /** Several datapoints of one channel ("Interface/ADDRESS:CH"), written in one command. */
    channel?: string;
    values?: {param: string; value: Value}[];
    webhook?: Webhook;
    wake?: Wake;
    command?: Command;
    /** A "display" activity's lines and signal tone, on the channel. */
    display?: Display;
    device?: Ref;
    variable?: string;
    program?: string;
    to?: Value;
    from_device?: Ref;
    from_variable?: string;
    special?: string;
    mode?: string;
    script?: string;
    /** "Ein für": the on-time sent with the value. */
    on_for?: string;
    after?: string;
    /** With it the delay is random, between after and this. */
    after_up_to?: string;
    note?: string;
}

export interface Group {
    op: string;
    lines: Line[];
}

export interface Rule {
    else?: boolean;
    when?: {op: string; groups: Group[]};
    note?: string;
    cancel_pending?: boolean;
    then: Action[];
}

export interface Program {
    key: string;
    name: string;
    note?: string;
    active: boolean;
    visible: boolean;
    operable: boolean;
    internal?: boolean;
    at_start?: boolean;
    rules: Rule[];
}

export interface Run {
    time: string;
    trigger: string;
    block: number;
}

export interface ProgramView {
    program: Program;
    problems: string[];
    triggers: {kind: string; ref: string; mode: string}[];
    script: boolean;
    time: boolean;
    state: {
        key: string;
        last_run?: Run;
        paused_until?: string;
        failures?: number;
        last_failure?: string;
        /** When a time module next triggers the program. */
        next_time?: string;
        /** An astro time module and no location: it never fires. */
        no_position?: boolean;
        /** The system's clock is not set yet: no time module fires. */
        no_clock?: boolean;
    };
}

/** One instant a time module fires at: a point, or a range's begin, repeat or end. */
export interface TimeEdge {
    at: string;
    kind: string;
}

/** A time module line of a program, as the engine sees it now. */
export interface TimeLine {
    /** Counted over the program's condition lines, from 1. */
    line: number;
    triggers: boolean;
    active: boolean;
    next: TimeEdge[];
    last?: TimeEdge;
    no_position?: boolean;
}

export interface Settings {
    location?: {latitude: number; longitude: number};
    /** The pause between two write commands on a radio interface, in milliseconds (unset: 300). */
    pacing_ms?: number;
    /** The loop guard (unset fields: 20 runs in 10 s, a pause of 60 s). */
    loop_guard?: {runs?: number; window_seconds?: number; pause_seconds?: number};
}

/** The defaults and limits of the pause between write commands and of the loop guard. */
export const RUNTIME = {
    pacing: {standard: 300, min: 100, max: 10000},
    runs: {standard: 20, min: 2, max: 1000},
    window: {standard: 10, min: 1, max: 600},
    pause: {standard: 60, min: 1, max: 86400},
} as const;

/** The fields of the pause between write commands and of the loop guard, as typed. */
export interface RuntimeForm {
    pacing: string;
    runs: string;
    window: string;
    pause: string;
}

/** The form of the settings as they are: what is not set shows its default. */
export function runtimeFormOf(s: Settings): RuntimeForm {
    return {
        pacing: String(s.pacing_ms ?? RUNTIME.pacing.standard),
        runs: String(s.loop_guard?.runs ?? RUNTIME.runs.standard),
        window: String(s.loop_guard?.window_seconds ?? RUNTIME.window.standard),
        pause: String(s.loop_guard?.pause_seconds ?? RUNTIME.pause.standard),
    };
}

/** What the form says, as settings (a default is not stored) - or what is wrong with it. */
export function runtimeOf(f: RuntimeForm): {pacing_ms?: number; loop_guard?: {runs?: number; window_seconds?: number; pause_seconds?: number}; error?: string} {
    const read = (text: string, limits: {standard: number; min: number; max: number}, name: string): number | string => {
        const n = Number(text.trim());
        if (text.trim() === '' || !Number.isInteger(n) || n < limits.min || n > limits.max) return t('{name}: a whole number from {min} to {max}.', {name, min: limits.min, max: limits.max});
        return n;
    };
    const pacing = read(f.pacing, RUNTIME.pacing, t('Pause between write commands'));
    const runs = read(f.runs, RUNTIME.runs, t('Runs'));
    const window = read(f.window, RUNTIME.window, t('Within'));
    const pause = read(f.pause, RUNTIME.pause, t('Pause'));
    for (const v of [pacing, runs, window, pause]) if (typeof v === 'string') return {error: v};
    const guard: {runs?: number; window_seconds?: number; pause_seconds?: number} = {};
    if (runs !== RUNTIME.runs.standard) guard.runs = runs as number;
    if (window !== RUNTIME.window.standard) guard.window_seconds = window as number;
    if (pause !== RUNTIME.pause.standard) guard.pause_seconds = pause as number;
    const out: {pacing_ms?: number; loop_guard?: typeof guard} = {};
    if (pacing !== RUNTIME.pacing.standard) out.pacing_ms = pacing as number;
    if (Object.keys(guard).length > 0) out.loop_guard = guard;
    return out;
}

export interface VariableDef {
    key: string;
    name: string;
    note?: string;
    type: string;
    labels?: {false?: string; true?: string};
    entries?: string[];
    unit?: string;
    min?: number;
    max?: number;
    visible: boolean;
    internal?: boolean;
    logged?: boolean;
    channel?: string;
    start_value?: Value;
    /** A computed value: a number calculated from its inputs, which nobody writes. */
    computed?: Computed;
    /** A device helper keeps the number: a counter's total, today's or yesterday's amount. */
    helper?: Helper;
}

export interface ComputedInput {
    device?: Ref;
    variable?: string;
}

export interface Computed {
    function: string;
    inputs: ComputedInput[];
}

/** The functions of a computed value, in the order the form offers them. */
export const COMPUTED_FUNCTIONS = ['dew-point', 'absolute-humidity', 'difference', 'sum', 'average', 'min', 'max'];

/** How many inputs a function takes, at least and at most. */
export function inputCount(fn: string): [number, number] {
    return ['dew-point', 'absolute-humidity', 'difference'].includes(fn) ? [2, 2] : [2, 8];
}

/** What an input of a function is: the temperature and the humidity of a dew point, the two sides of a difference, a number otherwise. */
export function inputLabel(fn: string, index: number): string {
    if (fn === 'dew-point' || fn === 'absolute-humidity') return index === 0 ? t('Temperature (°C)') : t('Relative humidity (%)');
    if (fn === 'difference') return index === 0 ? t('Value') : t('minus');
    return t('Input {n}', {n: index + 1});
}

/** A computed value in words: "Dew point of Bad ACTUAL_TEMPERATURE, Bad HUMIDITY". */
export function computedText(c: Computed, ctx: Context): string {
    const inputs = c.inputs.map((in_) => {
        if (in_.device) return channelName(in_.device, ctx) + ' ' + in_.device.param;
        const def = ctx.variables[in_.variable ?? ''];
        return def ? variableName(def) : (in_.variable ?? '');
    });
    return t('{function} of {inputs}', {function: t('function.' + c.function), inputs: inputs.join(', ')});
}

export interface AlarmState {
    state: number;
    count: number;
    first?: string;
    trigger?: string;
}

export interface VariableView {
    def: VariableDef;
    value: Value;
    time?: string;
    changed?: string;
    alarm?: AlarmState;
    users: string[];
    system?: boolean;
}

export interface Overview {
    programs: ProgramView[];
    variables: VariableView[];
    alarms: VariableView[];
    names: Record<string, string>;
    notices: {state?: string; history?: string; remote?: string; bad_files?: {path: string; error: string}[]};
    settings: Settings;
    /** The engine's time, with the system's own offset. */
    now: string;
}

export interface Note {
    code: string;
    args?: string[];
}

export interface ProgramReport {
    id: number;
    name: string;
    key?: string;
    status: string;
    was_active: boolean;
    active: boolean;
    updated?: boolean;
    reasons?: Note[];
    notes?: Note[];
}

export interface VariableReport {
    id: number;
    name: string;
    key?: string;
    type?: string;
    value?: string;
    entries?: string[];
    status: string;
    updated?: boolean;
    notes?: Note[];
}

export interface DeviceReport {
    channel: string;
    name?: string;
    device?: string;
    type?: string;
    rooms?: string[];
    known: boolean;
    programs: string[];
}

export interface Report {
    source: string;
    kind: string;
    firmware?: string;
    all_inactive?: boolean;
    location: {latitude?: string; longitude?: string; timezone?: string};
    programs: ProgramReport[];
    variables: VariableReport[];
    devices: DeviceReport[];
    repairs?: {ID: number; Before: string; After: string}[];
    services?: number;
    events?: Note[];
    histogram: {what: string; n: number}[];
}

export interface Preview {
    id: string;
    created: string;
    report: Report;
}

export type Codes = Record<string, {de: string; en: string}>;

/** What the words need to know: the variables and programs by key, the channels' names. */
export interface Context {
    variables: Record<string, VariableDef>;
    programs: Record<string, string>;
    names: Record<string, string>;
}

export function context(o: Overview | null): Context {
    const ctx: Context = {variables: {}, programs: {}, names: o?.names ?? {}};
    for (const v of o?.variables ?? []) ctx.variables[v.def.key] = v.def;
    for (const p of o?.programs ?? []) ctx.programs[p.program.key] = p.program.name;
    return ctx;
}

/** The system values' names: they have no file to carry one. */
export function variableName(def: VariableDef): string {
    if (def.name) return def.name;
    return t('system.' + def.key);
}

export function refText(ref: Ref): string {
    return `${ref.interface}/${ref.address}/${ref.param}`;
}

export function channelOf(ref: Ref): string {
    return `${ref.interface}/${ref.address}`;
}

/** A channel by the system's name for it, or by its address while it has none here. */
export function channelName(ref: Ref, ctx: Context): string {
    return ctx.names[channelOf(ref)] ?? `${ref.address} (${ref.interface})`;
}

const numberFormat = (n: number) => String(Math.round(n * 1000) / 1000).replace('.', t('decimal.'));

/** A value in the words of its variable: a boolean's label, a number with its unit. */
export function valueText(def: VariableDef | undefined, value: Value): string {
    if (value === null || value === undefined) return '–';
    if (typeof value === 'boolean') {
        const label = value ? def?.labels?.true : def?.labels?.false;
        return label || (value ? t('true') : t('false'));
    }
    if (typeof value === 'number') return numberFormat(value) + (def?.unit ? ' ' + def.unit : '');
    return value === '' ? t('(empty)') : value;
}

/** "1h30min" as "1 h 30 min". */
export function durationText(d: string | undefined): string {
    if (!d) return '';
    return d.replace(/(\d+)(h|min|s)/g, '$1 $2 ').trim();
}

/** The trigger mode in the CCU's words; a time module says "zu Zeitpunkten auslösen". */
export function triggerLabel(trigger: string, time = false): string {
    if (time && trigger === 'update') return t('trigger at the times');
    return t('trigger.' + trigger);
}

/** A time module in a sentence. */
export function timeText(m: TimeModule): string {
    const parts: string[] = [];
    const days = (m.weekdays ?? []).map((d) => t('day.' + d)).join(', ');
    // the day of a monthly or yearly module: "am 31." or "am 1. Mo"
    const dayOf = m.nth ? t('on the {n}. {weekday}', {n: m.nth, weekday: days}) : m.day ? t('on day {n}', {n: m.day}) : '';
    switch (m.kind) {
        case 'interval':
            parts.push(t('every {d}', {d: durationText(m.interval)}));
            break;
        case 'daily': {
            const set = (m.weekdays ?? []).join(',');
            const which = set === 'mon,tue,wed,thu,fri' ? t('on workdays') : set === 'sat,sun' ? t('at the weekend') : days;
            parts.push((m.every ? t('every {n} days', {n: m.every}) : which ? '' : t('daily')) + (m.every && which ? ' ' : '') + which);
            break;
        }
        case 'weekly':
            parts.push((m.every ? t('every {n} weeks', {n: m.every}) : t('weekly')) + ' ' + days);
            break;
        case 'monthly':
            parts.push((m.every ? t('every {n} months', {n: m.every}) : t('monthly')) + (dayOf ? ' ' + dayOf : ''));
            break;
        case 'yearly':
            if (m.nth && m.month) parts.push(t('yearly') + ' ' + dayOf + ' ' + t('in month {n}', {n: m.month}));
            else parts.push(t('yearly') + (m.day && m.month ? ' ' + t('on {day}.{month}.', {day: m.day, month: m.month}) : ''));
            break;
        default:
            parts.push(t('once') + (m.date ? ' ' + t('on {date}', {date: m.date}) : ''));
    }
    if (m.astro) {
        parts.push(t('astro.' + m.astro) + (m.offset ? ' (' + durationText(m.offset) + ')' : '') + (m.point ? ', ' + t('as a point in time') : '') +
            (m.twilight ? ', ' + t('twilight.' + m.twilight) : ''));
    } else if (m.all_day) {
        parts.push(t('all day'));
    } else if (m.at) {
        parts.push(m.duration || m.kind === 'interval' ? t('from {time}', {time: m.at}) : t('at {time}', {time: m.at}));
    }
    if (m.duration) parts.push(t('for {d}', {d: durationText(m.duration)}));
    if (m.from) parts.push(t('valid from {date}', {date: m.from}));
    if (m.until) parts.push(t('until {date}', {date: m.until}));
    if (m.count) parts.push(t('{n} times', {n: m.count}));
    return parts.join(', ');
}

/**
 * A time as the system's clock shows it: the engine sends its times with the system's own
 * offset ("2026-11-21T06:30:00+01:00"), and that wall clock is what is written - not the
 * browser's, which may stand in another time zone. Seconds only when there are any.
 */
export function wallTime(iso: string | undefined, language: string): string {
    const m = /^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)/.exec(iso ?? '');
    if (!m) return '–';
    const [y = 0, mo = 1, d = 1, h = 0, mi = 0, sec = 0] = m.slice(1).map(Number);
    const date = new Date(Date.UTC(y, mo - 1, d, h, mi, sec));
    return date.toLocaleString(language === 'de' ? 'de-DE' : 'en-GB', {dateStyle: 'short', timeStyle: sec ? 'medium' : 'short', timeZone: 'UTC'});
}

/** What a time module does at an instant, in a word. */
export function edgeText(kind: string): string {
    return t('edge.' + kind);
}

/** What caused a run, in words: the run's trigger as the engine names it. */
export function runTrigger(trigger: string, ctx: Context): string {
    const [kind, ...rest] = trigger.split(':');
    const what = rest.join(':');
    switch (kind) {
        case 'time':
            return t('time module');
        case 'variable':
            return ctx.variables[what]?.name ?? what;
        case 'device': {
            const [iface, address, param] = what.split('/');
            return (ctx.names[iface + '/' + address] ?? address) + ' ' + param;
        }
        case 'program':
            return t('program {name}', {name: ctx.programs[what] ?? what});
        case 'user':
            return t('by hand');
        case 'start':
            return t('system start');
        case 'held':
            return t('a condition held long enough');
        default:
            return t('from outside');
    }
}

/** The number of every condition line of a program, counted from 1 as the engine counts them: [rule][group][line]. */
export function lineNumbers(p: Program): number[][][] {
    let n = 0;
    return p.rules.map((r) => (r.when?.groups ?? []).map((g) => g.lines.map(() => ++n)));
}

/** What a condition line says, in the CCU's columns. */
export interface LineText {
    /** The kind of the line: Systemvariable, Geräteauswahl, Zeitsteuerung. */
    kind: string;
    subject: string;
    /** The datapoint of a device line. */
    param: string;
    comparison: string;
    mode: string;
    /** The line triggers: a time module always does, whatever its mode (as on the CCU). */
    triggers: boolean;
    /** A time module on "check only": it triggers all the same. */
    timeCheck?: boolean;
    not: boolean;
    /** The device or variable is not known here. */
    unknown: boolean;
}

export function lineText(l: Line, ctx: Context): LineText {
    const out: LineText = {kind: '', subject: '', param: '', comparison: '', mode: triggerLabel(l.trigger, !!l.time), triggers: l.trigger !== 'check' || !!l.time, not: !!l.not, unknown: false};
    if (l.time) {
        out.kind = t('Time control');
        out.subject = timeText(l.time);
        out.timeCheck = l.trigger === 'check';
        return out;
    }
    let def: VariableDef | undefined;
    if (l.device) {
        out.kind = t('Device selection');
        out.subject = channelName(l.device, ctx);
        out.param = l.device.param;
        out.unknown = !(channelOf(l.device) in ctx.names);
    } else {
        out.kind = t('System variable');
        def = ctx.variables[l.variable ?? ''];
        out.subject = def ? variableName(def) : (l.variable ?? '');
        out.unknown = !def;
    }
    switch (l.op) {
        case 'range':
            out.comparison = t('in the range from {from} to less than {below}', {from: valueText(def, l.from), below: valueText(def, l.below)});
            break;
        case 'outside':
            out.comparison = t('outside the range from {from} to {to}', {from: valueText(def, l.from), to: valueText(def, l.to)});
            break;
        default:
            out.comparison = t('op.' + (l.op ?? 'eq')) + ' ' + valueText(def, l.value);
    }
    if (l.for) out.comparison += ', ' + t('for at least {d}', {d: durationText(l.for)});
    return out;
}

/** What an activity says. */
export interface ActionText {
    kind: string;
    subject: string;
    param: string;
    /** What is done: "auf 21 °C", "ausführen". */
    what: string;
    /** "sofort" or "verzögert um 30 s". */
    timing: string;
    script: string;
    /** The activity is skipped: a special value, or a kept script. */
    skipped: boolean;
    unknown: boolean;
}

export function actionText(a: Action, ctx: Context): ActionText {
    const out: ActionText = {kind: '', subject: '', param: '', what: '', timing: a.after_up_to ? t('delayed at random by {from} to {to}', {from: durationText(a.after ?? '0s'), to: durationText(a.after_up_to)}) : a.after ? t('delayed by {d}', {d: durationText(a.after)}) : t('immediately'),
        script: '', skipped: false, unknown: false};
    let def: VariableDef | undefined;
    switch (a.kind) {
        case 'set':
            out.kind = t('Device selection');
            if (a.device) {
                out.subject = channelName(a.device, ctx);
                out.param = a.device.param;
                out.unknown = !(channelOf(a.device) in ctx.names);
            }
            break;
        case 'variable':
            out.kind = t('System variable');
            def = ctx.variables[a.variable ?? ''];
            out.subject = def ? variableName(def) : (a.variable ?? '');
            out.unknown = !def;
            break;
        case 'script':
            out.kind = t('Script');
            out.script = a.script ?? '';
            out.what = t('is kept as text and never run');
            out.skipped = true;
            return out;
        case 'channel':
            out.kind = t('Several values');
            out.subject = ctx.names[a.channel ?? ''] ?? a.channel ?? '';
            out.unknown = !((a.channel ?? '') in ctx.names);
            out.what = (a.values ?? []).map((v) => v.param + ' = ' + (typeof v.value === 'string' && v.value !== '' ? v.value : valueText(undefined, v.value))).join(', ');
            return out;
        case 'display':
            out.kind = t('Display');
            out.subject = ctx.names[a.channel ?? ''] ?? a.channel ?? '';
            out.unknown = !((a.channel ?? '') in ctx.names);
            out.what = displayText(a.display);
            return out;
        case 'webhook':
            out.kind = t('Webhook');
            out.subject = ((a.webhook?.method ?? '') + ' ' + (a.webhook?.url ?? '')).trim();
            out.what = [a.webhook?.body ? t('call with a body ({type})', {type: t('body.' + (a.webhook.body_type ?? ''))}) : t('call'), a.webhook?.insecure ? t('certificate not verified') : ''].filter(Boolean).join(', ');
            return out;
        case 'command':
            out.kind = t('Command');
            out.subject = commandText(a.command, ctx);
            out.what = commandWhat(a.command, ctx);
            return out;
        case 'wake':
            out.kind = t('Wake-on-LAN');
            out.subject = a.wake?.mac ?? '';
            out.what = a.wake?.broadcast ? t('wake up, through {address}', {address: a.wake.broadcast}) : t('wake up');
            return out;
        default:
            out.kind = t('Program');
            out.subject = ctx.programs[a.program ?? ''] ?? a.program ?? '';
            out.unknown = !(a.program && a.program in ctx.programs);
            out.what = a.kind === 'run' ? (a.mode === 'check' ? t('evaluate') : t('run the first "Then"')) : a.kind === 'activate' ? t('switch on') : t('switch off');
            return out;
    }
    if (a.special) {
        out.what = t('special value {v}: skipped until edited', {v: a.special});
        out.skipped = true;
    } else if (a.from_variable) {
        out.what = t('to the value of {name}', {name: ctx.variables[a.from_variable] ? variableName(ctx.variables[a.from_variable]!) : a.from_variable});
    } else if (a.from_device) {
        out.what = t('to the value of {name}', {name: channelName(a.from_device, ctx) + ' ' + a.from_device.param});
    } else {
        out.what = t('to {v}', {v: valueText(def, a.to)});
        if (a.on_for) out.what += ' ' + t('for {d}', {d: durationText(a.on_for)});
    }
    return out;
}

/** The heading of a rule in the CCU's words. */
export function ruleHeading(rule: Rule, index: number): string {
    if (rule.else) return t('Else…');
    return index === 0 ? t('Condition: If…') : t('Else if…');
}

/** What triggers a program, for the list: the names, each once. */
export function triggerSummary(p: ProgramView, ctx: Context): string[] {
    const out: string[] = [];
    for (const tr of p.triggers) {
        let name = tr.ref;
        if (tr.kind === 'variable') {
            name = ctx.variables[tr.ref] ? variableName(ctx.variables[tr.ref]!) : tr.ref;
        } else if (tr.kind === 'device') {
            const [iface, address, param] = tr.ref.split('/');
            name = channelName({interface: iface ?? '', address: address ?? '', param: param ?? ''}, ctx) + ' ' + (param ?? '');
        } else {
            name = t('Time control');
        }
        if (!out.includes(name)) out.push(name);
    }
    return out;
}

/** The state of a program in one word, and the class of its pill. */
export function programState(p: ProgramView, now = Date.now()): {label: string; kind: string} {
    if (p.problems.length > 0) return {label: t('faulty'), kind: 'bad'};
    if (p.state.paused_until && Date.parse(p.state.paused_until) > now) return {label: t('paused'), kind: 'warn'};
    if (p.program.active) return {label: t('active'), kind: 'ok'};
    return {label: t('inactive'), kind: ''};
}

/** A mark on a row of a list: a short word in a pill, and what it means. */
export interface Mark {
    id: string;
    label: string;
    kind: string;
    title: string;
}

/** The marks of a program, what needs attention first. */
export function programMarks(p: ProgramView): Mark[] {
    const out: Mark[] = [];
    if (p.state.failures) out.push({id: 'failures', label: t('{n} writes refused', {n: p.state.failures}, p.state.failures), kind: 'bad', title: p.state.last_failure ?? ''});
    if (p.script) out.push({id: 'script', label: t('script'), kind: 'warn', title: t('Holds a script: scripts do not run here.')});
    if (p.state.no_position) out.push({id: 'no-location', label: t('no location'), kind: 'warn', title: t('Sunrise and sunset need the location of the system: without it this time module never triggers.')});
    if (p.state.no_clock) out.push({id: 'no-clock', label: t('clock not set'), kind: 'warn', title: t('The clock of the system is not set yet: time modules wait until it is.')});
    if (p.program.internal) out.push({id: 'internal', label: t('internal'), kind: '', title: ''});
    return out;
}

/** The marks of a variable. */
export function variableMarks(v: VariableView, ctx?: Context): Mark[] {
    const out: Mark[] = [];
    if (v.def.helper) {
        const h = v.def.helper;
        // (what needs attention first: a row shows its first mark)
        if (h.off) out.push({id: 'helper-off', label: t('off'), kind: 'warn', title: t('The device helper is switched off: it adds nothing, and counts on from the device\'s reading when it is switched on again.')});
        out.push({id: 'helper', label: t('helper'), kind: '', title: helperTitle(h, ctx?.names[h.channel] ?? h.channel)});
    }
    if (v.system) out.push({id: 'system', label: t('system value'), kind: '', title: t('A system value: the system maintains it, nobody else writes it.')});
    if (v.def.computed) out.push({id: 'computed', label: t('computed'), kind: '', title: t('A computed value: calculated from its inputs whenever one of them reports; nobody writes it.')});
    if (v.def.internal) out.push({id: 'internal', label: t('internal'), kind: '', title: ''});
    if (v.def.logged) out.push({id: 'logged', label: t('logged'), kind: '', title: ''});
    return out;
}

/** The tooltip of a name: the name whole, and the description below it where there is one. */
export function nameTitle(name: string, note?: string): string {
    return note ? name + '\n' + note : name;
}

/** All marks of a row in words, for the tooltip of their cell: one per line. */
export function marksTitle(marks: Mark[]): string {
    return marks.map((m) => (m.title ? m.label + ': ' + m.title : m.label)).join('\n');
}

/** Which block ran last. */
export function blockText(block: number): string {
    return block === 0 ? t('Else') : block === 1 ? t('Then') : t('Else if {n}', {n: block - 1});
}

export function dateTime(iso: string | undefined, language: string): string {
    if (!iso) return '–';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '–';
    return d.toLocaleString(language === 'de' ? 'de-DE' : 'en-GB', {dateStyle: 'short', timeStyle: 'medium'});
}

/** Search: every word of the query somewhere in the texts, case and accents as typed. */
export function matches(query: string, ...texts: (string | undefined)[]): boolean {
    const hay = texts.join(' ').toLowerCase();
    return query
        .toLowerCase()
        .split(/\s+/)
        .filter(Boolean)
        .every((w) => hay.includes(w));
}

/** What the user typed for a value, as the variable's own type; an error says what is wrong. */
export function parseValue(def: VariableDef, input: string | boolean): {value?: Value; error?: string} {
    switch (def.type) {
        case 'bool':
        case 'alarm':
            return {value: input === true || input === 'true'};
        case 'number': {
            const text = String(input).trim().replace(',', '.');
            const n = Number(text);
            if (text === '' || !Number.isFinite(n)) return {error: t('Enter a number.')};
            if (def.min !== undefined && n < def.min) return {error: t('The smallest value is {n}.', {n: numberFormat(def.min)})};
            if (def.max !== undefined && n > def.max) return {error: t('The largest value is {n}.', {n: numberFormat(def.max)})};
            return {value: n};
        }
        case 'list':
            if (!(def.entries ?? []).includes(String(input))) return {error: t('Choose one of the entries.')};
            return {value: String(input)};
        default:
            return {value: String(input)};
    }
}

/** The form of a variable: what the dialog edits. */
export interface VariableForm {
    key: string;
    name: string;
    note: string;
    type: string;
    labelFalse: string;
    labelTrue: string;
    /** One entry per line; original keeps the label an entry had when the dialog opened. */
    entries: {label: string; original: string}[];
    unit: string;
    min: string;
    max: string;
    visible: boolean;
    internal: boolean;
    logged: boolean;
    channel: string;
    startValue: Value;
    /** A computed value (type "computed"): its function and inputs. */
    fn: string;
    inputs: FormInput[];
    /** The variable's place in a device helper: kept as it is, but for "off". */
    helper?: Helper;
}

/** One input of a computed value in the form: a datapoint ("Interface/ADDRESS:CH" and its parameter) or a variable. */
export interface FormInput {
    kind: 'device' | 'variable';
    channel: string;
    param: string;
    variable: string;
}

export const emptyInput = (): FormInput => ({kind: 'device', channel: '', param: '', variable: ''});

/** The form's inputs made to fit a function: as many as it takes at least, no more than it takes at most. */
export function fitInputs(fn: string, inputs: FormInput[]): FormInput[] {
    const [min, max] = inputCount(fn);
    const out = inputs.slice(0, max);
    while (out.length < min) out.push(emptyInput());
    return out;
}

export function emptyForm(): VariableForm {
    return {key: '', name: '', note: '', type: 'bool', labelFalse: '', labelTrue: '', entries: [{label: '', original: ''}], unit: '', min: '', max: '',
        visible: true, internal: false, logged: false, channel: '', startValue: undefined, fn: 'dew-point', inputs: [emptyInput(), emptyInput()]};
}

export function formOf(def: VariableDef): VariableForm {
    return {
        key: def.key,
        name: def.name,
        note: def.note ?? '',
        type: def.computed ? 'computed' : def.type,
        labelFalse: def.labels?.false ?? '',
        labelTrue: def.labels?.true ?? '',
        entries: (def.entries ?? ['']).map((e) => ({label: e, original: e})),
        unit: def.unit ?? '',
        min: def.min === undefined ? '' : String(def.min),
        max: def.max === undefined ? '' : String(def.max),
        visible: def.visible,
        internal: !!def.internal,
        logged: !!def.logged,
        channel: def.channel ?? '',
        startValue: def.start_value,
        fn: def.computed?.function ?? 'dew-point',
        inputs: def.computed
            ? def.computed.inputs.map((in_) => (in_.device ? {kind: 'device', channel: `${in_.device.interface}/${in_.device.address}`, param: in_.device.param, variable: ''} : {kind: 'variable', channel: '', param: '', variable: in_.variable ?? ''}))
            : [emptyInput(), emptyInput()],
        ...(def.helper ? {helper: {...def.helper}} : {}),
    };
}

const optionalNumber = (s: string): number | undefined | null => {
    const text = s.trim().replace(',', '.');
    if (text === '') return undefined;
    const n = Number(text);
    return Number.isFinite(n) ? n : null;
};

/** What is wrong with the form, field by field; empty when it can be saved. */
export function formProblems(f: VariableForm): Record<string, string> {
    const out: Record<string, string> = {};
    if (f.name.trim() === '') out.name = t('A variable needs a name.');
    if (f.type === 'list') {
        const labels = f.entries.map((e) => e.label.trim());
        if (labels.length === 0 || labels.some((l) => l === '')) out.entries = t('Every entry needs a text.');
        else if (new Set(labels).size !== labels.length) out.entries = t('Every entry may be there once only.');
        else if (labels.some((l) => l.includes(';'))) out.entries = t('An entry cannot hold a semicolon.');
    }
    if (f.type === 'number') {
        const min = optionalNumber(f.min);
        const max = optionalNumber(f.max);
        if (min === null) out.min = t('Enter a number.');
        if (max === null) out.max = t('Enter a number.');
        if (typeof min === 'number' && typeof max === 'number' && min > max) out.max = t('The maximum is below the minimum.');
    }
    if (f.type === 'computed') {
        const [min, max] = inputCount(f.fn);
        if (f.inputs.length < min || f.inputs.length > max) out.inputs = min === max ? t('This function takes exactly {n} inputs.', {n: min}) : t('This function takes {min} to {max} inputs.', {min, max});
        else if (f.inputs.some((in_) => (in_.kind === 'device' ? !in_.channel || !in_.param.trim() : !in_.variable))) out.inputs = t('Every input needs a datapoint or a variable.');
        else if (f.inputs.some((in_) => in_.kind === 'variable' && in_.variable === f.key)) out.inputs = t('A computed value cannot be its own input.');
    }
    return out;
}

/** The definition the API takes, and the entries that were renamed (old label to new). */
export function defOf(f: VariableForm): {variable: VariableDef; renames: Record<string, string>} {
    const v: VariableDef = {key: f.key, name: f.name.trim(), type: f.type === 'computed' ? 'number' : f.type, visible: f.visible};
    if (f.note.trim()) v.note = f.note.trim();
    if (f.internal) v.internal = true;
    if (f.logged) v.logged = true;
    if (f.channel) v.channel = f.channel;
    if (f.startValue !== undefined && f.startValue !== null && f.type !== 'computed' && !f.helper) v.start_value = f.startValue;
    if (f.helper && f.type === 'number') {
        v.helper = {kind: f.helper.kind, channel: f.helper.channel, role: f.helper.role};
        if (f.helper.off) v.helper.off = true;
    }
    const renames: Record<string, string> = {};
    switch (f.type) {
        case 'bool':
        case 'alarm':
            if (f.labelFalse.trim() || f.labelTrue.trim()) v.labels = {false: f.labelFalse.trim(), true: f.labelTrue.trim()};
            break;
        case 'list':
            v.entries = f.entries.map((e) => e.label.trim());
            for (const e of f.entries) {
                if (e.original && e.original !== e.label.trim()) renames[e.original] = e.label.trim();
            }
            break;
        case 'number': {
            if (f.unit.trim()) v.unit = f.unit.trim();
            const min = optionalNumber(f.min);
            const max = optionalNumber(f.max);
            if (typeof min === 'number') v.min = min;
            if (typeof max === 'number') v.max = max;
            break;
        }
        case 'computed':
            if (f.unit.trim()) v.unit = f.unit.trim();
            v.computed = {
                function: f.fn,
                inputs: f.inputs.map((in_) => {
                    if (in_.kind === 'variable') return {variable: in_.variable};
                    const [iface = '', address = ''] = in_.channel.split('/');
                    return {device: {interface: iface, address, param: in_.param.trim()}};
                }),
            };
            break;
    }
    return {variable: v, renames};
}

/** The first value a new variable gets in the dialog. */
export function defaultValue(type: string, entries: string[]): Value {
    switch (type) {
        case 'bool':
            return false;
        case 'number':
            return 0;
        case 'list':
            return entries[0] ?? '';
        case 'string':
            return '';
    }
    return undefined;
}

// --- the import's report ---

/** A note in the page's language: the importer's words with the arguments filled in. */
export function noteText(note: Note, codes: Codes, language: string): string {
    const entry = codes[note.code];
    const args = (note.args ?? []).filter((a) => a !== '').join(', ');
    if (!entry) return (note.code + ' ' + args).trim();
    const template = language === 'de' ? entry.de : entry.en;
    return template.includes('%s') ? template.replace('%s', args) : template;
}

/** The result of a program's import in words, and the class of its pill. */
export function importStatus(p: ProgramReport): {label: string; kind: string} {
    if (p.status === 'skipped') return {label: t('not imported'), kind: 'bad'};
    if (p.status === 'inactive') return {label: t('inactive'), kind: 'warn'};
    return p.active ? {label: t('active'), kind: 'ok'} : {label: t('inactive'), kind: ''};
}

export function reportTotals(r: Report): {imported: number; inactive: number; skipped: number; variables: number; missing: number} {
    const out = {imported: 0, inactive: 0, skipped: 0, variables: 0, missing: 0};
    for (const p of r.programs) {
        if (p.status === 'skipped') out.skipped++;
        else if (p.status === 'inactive') out.inactive++;
        else out.imported++;
    }
    out.variables = r.variables.filter((v) => v.status === 'imported').length;
    out.missing = r.devices.filter((d) => !d.known).length;
    return out;
}

/** Whether a program of the report holds a script: the pointer to other tools is shown then. */
export function reportHasScript(r: Report): boolean {
    return r.programs.some((p) => (p.reasons ?? []).some((n) => n.code === 'script'));
}
