// The run trace in words: what caused a run, what every condition line was, which block ran and
// what each activity did. The engine sends data; here it becomes the reader's language.
import {t} from 'occulite-ui/i18n';

import {combinedDisplay} from './display.js';
import {firstValue, variableKind, type ValueKind} from './editor.js';
import {actionText, blockText, edgeText, lineText, matches, runTrigger, valueText, wallTime, type Action, type Context, type Line, type Program, type Value} from './model.js';

export interface TraceLine {
    line: number;
    rule: number;
    def: Line;
    value?: Value;
    unknown?: boolean;
    truth: boolean;
    skipped?: boolean;
    since?: string;
}

export interface TraceStep {
    action: number;
    def?: Action;
    what: string;
    value?: Value;
    due?: string;
    count?: number;
    reason?: string;
    detail?: string;
    confirmed?: string;
    failed?: string;
    /** A webhook's answer: the status code, and how long the call took (ms). */
    status?: number;
    took?: number;
    /** The beginning of what a command wrote. */
    output?: string;
    ran?: string;
    ended?: string;
    ended_by?: string;
}

export interface TraceRun {
    seq: number;
    program: string;
    time: string;
    trigger: string;
    event?: {value: Value; prev: Value; first?: boolean};
    mode?: string;
    block: number;
    none?: boolean;
    delayed?: boolean;
    untriggered?: boolean;
    info?: string;
    count?: number;
    since?: string;
    due?: string;
    /** A simulation: nothing was done, the steps say what would have been. */
    dry?: boolean;
    lines?: TraceLine[];
    steps?: TraceStep[];
}

export interface TracePage {
    runs: TraceRun[];
    more: boolean;
}

/** A condition line of an entry, said. */
export interface LineView {
    n: number;
    /** The line triggers. */
    mark: boolean;
    text: string;
    /** What the line saw: the value, or that there was none. */
    value: string;
    /** wahr, falsch, nicht geprüft. */
    verdict: string;
    /** ok (true), '' (false), skip (not looked at). */
    kind: string;
    /** What follows from it, where that surprises. */
    hint: string;
}

/** A step of an entry, said. */
export interface StepView {
    /** The activity's number in its block ('' for the block's own step). */
    n: string;
    text: string;
    status: string;
    /** ok, bad, wait, '' */
    kind: string;
}

/** An entry of the trace, said. */
export interface RunView {
    seq: number;
    time: string;
    programKey: string;
    program: string;
    trigger: string;
    /** What became of it, short: Dann, Sonst, nichts lief, nicht ausgelöst. */
    outcome: string;
    /** ok (a block ran), warn (paused, missed), bad (a step failed), '' */
    kind: string;
    /** Why, or how often. */
    detail: string;
    lines: LineView[];
    steps: StepView[];
    /** A block ran, or a delayed activity came due. */
    acted: boolean;
    /** Something went wrong or was left out. */
    problem: boolean;
}

/** The time of day of an instant on the system's clock, with seconds. */
export function clockTime(iso: string | undefined): string {
    const m = /T(\d\d:\d\d:\d\d)/.exec(iso ?? '');
    return m ? m[1]! : '–';
}

/** What caused an entry, with the value it brought. */
export function triggerWords(run: TraceRun, ctx: Context): string {
    const [kind, ...rest] = run.trigger.split(':');
    const what = rest.join(':');
    const base = runTrigger(run.trigger, ctx);
    switch (kind) {
        case 'time':
            if (!what) return base;
            return base + ' (' + what.split(',').map((f) => {
                const [line, edge] = f.split(':');
                return t('line {n}', {n: line ?? ''}) + (edge && edge !== 'point' ? ', ' + edgeText(edge) : '');
            }).join('; ') + ')';
        case 'held':
            return t('line {n} held long enough', {n: what});
        case 'variable':
        case 'device': {
            const ev = run.event;
            if (!ev) return base;
            const def = kind === 'variable' ? ctx.variables[what] : undefined;
            if (ev.first) return base + ': ' + valueText(def, ev.value);
            return base + ': ' + valueText(def, ev.prev) + ' → ' + valueText(def, ev.value);
        }
    }
    return base;
}

function lineView(l: TraceLine, ctx: Context): LineView {
    const lt = lineText(l.def, ctx);
    const out: LineView = {n: l.line, mark: lt.triggers, text: [lt.subject, lt.param, lt.not ? t('not') : '', lt.comparison].filter(Boolean).join(' '), value: '', verdict: '', kind: '', hint: ''};
    if (l.skipped) {
        out.verdict = t('not looked at');
        out.kind = 'skip';
        out.hint = l.def.trigger === 'change' ? t('A block before it ran. The line has no remembered state now: its next report triggers the program.') : t('A block before it ran.');
        return out;
    }
    out.verdict = l.truth ? t('true') : t('false');
    out.kind = l.truth ? 'ok' : '';
    if (l.def.time) return out;
    if (l.unknown) {
        out.value = t('value unknown');
        out.hint = t('A line without a value is false.');
        return out;
    }
    out.value = valueText(l.def.variable ? ctx.variables[l.def.variable] : undefined, l.value);
    if (l.def.for && l.since) out.hint = l.truth ? t('holds since {time}', {time: clockTime(l.since)}) : t('holds since {time}: not long enough yet', {time: clockTime(l.since)});
    return out;
}

/** What a step of a simulation would have done. */
const WOULD: Record<string, string> = {sent: 'would be sent', written: 'would be written', scheduled: '', run: 'would be started', switched: 'would be switched'};

function endedBy(by: string | undefined, ctx: Context): string {
    if (by === 'user') return t('by hand');
    if (by === 'removed') return t('its program was removed');
    const key = (by ?? '').replace(/^program:/, '');
    return t('by program {name}', {name: ctx.programs[key] ?? key});
}

function stepView(s: TraceStep, ctx: Context, dry = false): StepView {
    const out: StepView = {n: s.action ? String(s.action) : '', text: '', status: '', kind: ''};
    if (s.what === 'cancelled') {
        out.text = dry ? t('{n} running delays for the targets of this block would be ended', {n: s.count ?? 0}, s.count ?? 0) : t('{n} running delays for the targets of this block ended', {n: s.count ?? 0}, s.count ?? 0);
        return out;
    }
    const def: Action = s.def ?? {kind: 'script'};
    // a display: the lines as they went out, the placeholders filled
    const shown = def.kind === 'display' && typeof s.value === 'string' ? combinedDisplay(s.value) : undefined;
    const at = actionText(shown ? {...def, display: shown} : def, ctx);
    const target = def.kind === 'variable' ? ctx.variables[def.variable ?? ''] : undefined;
    // what was really written: the value a "from" brought, in the target's words
    const what = s.value !== undefined && s.value !== null && (def.from_variable || def.from_device) ? t('to {v}', {v: valueText(target, s.value)}) + ' (' + at.what + ')' : at.what;
    out.text = [at.kind === t('Script') ? at.kind : '', at.subject, at.param, what].filter(Boolean).join(' ');
    // a command: the line as it was run, the values put in
    if (def.kind === 'command') out.text = t('Command') + ' ' + (typeof s.value === 'string' && s.value ? s.value : at.subject);
    if (dry && s.what === 'called') {
        out.status = def.kind === 'wake' ? t('would be sent') : def.kind === 'command' ? t('would be run') : t('would be called');
        out.kind = 'wait';
        return out;
    }
    if (dry && s.what in WOULD) {
        out.status = s.what === 'scheduled' ? t('would be scheduled for {time}', {time: clockTime(s.due)}) : t(WOULD[s.what]!);
        out.kind = 'wait';
        return out;
    }
    switch (s.what) {
        case 'sent':
            if (s.failed) {
                out.status = t('sent, refused: {error}', {error: s.failed});
                out.kind = 'bad';
            } else if (s.confirmed) {
                out.status = t('sent, confirmed {time}', {time: clockTime(s.confirmed)});
                out.kind = 'ok';
            } else {
                out.status = t('sent');
                out.kind = 'wait';
            }
            break;
        case 'called':
            if (s.failed) {
                out.status = t('failed: {error}', {error: s.failed}) + (s.took ? ' · ' + t('{n} ms', {n: s.took}) : '');
                out.kind = 'bad';
            } else if (s.confirmed) {
                out.status = def.kind === 'wake' ? t('sent {time}', {time: clockTime(s.confirmed)}) : def.kind === 'command' ? t('ended after {n} ms', {n: s.took ?? 0}) : t('answered {status} after {n} ms', {status: s.status ?? 0, n: s.took ?? 0});
                out.kind = 'ok';
            } else {
                out.status = def.kind === 'command' ? (def.command?.wait ? t('running, the block waits') : t('running')) : t('under way');
                out.kind = 'wait';
            }
            if (def.kind === 'command') {
                if (s.output) out.status += ' · ' + t('output: {text}', {text: s.output});
                // a variable that did not take the output or the exit code
                if (s.detail) {
                    out.status += ' · ' + s.detail;
                    out.kind = 'bad';
                }
            }
            break;
        case 'written':
            out.status = t('written');
            out.kind = 'ok';
            break;
        case 'scheduled':
            out.status = t('scheduled for {time}', {time: clockTime(s.due)});
            out.kind = 'wait';
            if (s.ran) {
                out.status += ' · ' + t('carried out {time}', {time: clockTime(s.ran)});
                out.kind = 'ok';
            } else if (s.ended) {
                out.status += ' · ' + t('ended {time}', {time: clockTime(s.ended)}) + ' (' + endedBy(s.ended_by, ctx) + ')';
                out.kind = '';
            }
            break;
        case 'refused':
            out.status = t('refused: {error}', {error: s.detail ?? ''});
            out.kind = 'bad';
            break;
        case 'run':
            out.status = t('started');
            out.kind = 'ok';
            break;
        case 'switched':
            out.status = t('switched');
            out.kind = 'ok';
            break;
        case 'dropped':
            out.status = t('dropped: it was due at {time}, while the addon was stopped', {time: clockTime(s.due)});
            out.kind = 'bad';
            break;
        default:
            out.status = t('skipped') + ': ' + t('skip.' + (s.reason ?? ''));
            out.kind = 'bad';
    }
    return out;
}

/** An entry of the trace in words. */
export function runView(run: TraceRun, ctx: Context, language: string): RunView {
    const out: RunView = {seq: run.seq, time: wallTime(run.time, language), programKey: run.program, program: ctx.programs[run.program] ?? run.program, trigger: triggerWords(run, ctx),
        outcome: '', kind: '', detail: '', lines: (run.lines ?? []).map((l) => lineView(l, ctx)), steps: (run.steps ?? []).map((s) => stepView(s, ctx, run.dry)), acted: false, problem: false};
    if (run.untriggered) {
        out.outcome = run.dry ? t('would not trigger') : t('not triggered');
        out.detail = t('why.' + (run.info ?? ''));
        if ((run.count ?? 1) > 1) out.detail += ' · ' + t('{n} times since {time}', {n: run.count ?? 0, time: clockTime(run.since)});
    } else if (run.info === 'paused') {
        out.outcome = t('paused');
        out.detail = t('The program ran too often and pauses (the loop guard).');
        out.kind = 'warn';
        out.problem = true;
    } else if (run.info === 'missed') {
        out.outcome = t('missed');
        out.detail = t('Its time module was due at {time} and is not made up: the clock was set, or the system stood still.', {time: wallTime(run.due, language)});
        out.kind = 'warn';
        out.problem = true;
    } else if (run.delayed) {
        out.outcome = t('delayed activity');
        out.detail = t('A delayed activity of "{block}" came due. Nothing is checked again then.', {block: blockText(run.block)});
        out.kind = 'ok';
        out.acted = true;
    } else if (run.none) {
        out.outcome = t('nothing ran');
        out.detail = t('The program was evaluated; no condition was true and there is no "Else".');
    } else {
        out.outcome = blockText(run.block);
        out.kind = 'ok';
        out.acted = true;
        if (run.mode === 'then') out.detail = t('Run without a look at the conditions.');
    }
    if (out.steps.some((s) => s.kind === 'bad')) {
        out.kind = 'bad';
        out.problem = true;
    }
    return out;
}

/** The entries a search and a filter leave: show is '' (all), 'acted' or 'problems'. */
export function filterRuns(views: RunView[], f: {query?: string; show?: string; program?: string}): RunView[] {
    return views.filter((v) => {
        if (f.program && v.programKey !== f.program) return false;
        if (f.show === 'acted' && !v.acted) return false;
        if (f.show === 'problems' && !v.problem) return false;
        return !f.query || matches(f.query, v.program, v.trigger, v.outcome, v.detail, ...v.steps.map((s) => s.text + ' ' + s.status), ...v.lines.map((l) => l.text));
    });
}

/** Older entries joined to the ones the page has, newest first, each once. */
export function mergeRuns(have: TraceRun[], got: TraceRun[]): TraceRun[] {
    const by = new Map<number, TraceRun>();
    for (const r of have) by.set(r.seq, r);
    for (const r of got) by.set(r.seq, r);
    return [...by.values()].sort((a, b) => b.seq - a.seq);
}


/** A condition line to simulate: its number as the engine counts, what it says, and how its value is entered (null: a time module, tried as firing now). */
export interface SimLine {
    n: number;
    label: string;
    kind: ValueKind | null;
    /** The value to start from: the one the line compares with. */
    value: Value;
}

/** The lines of a program a trigger can be simulated on. */
export function simLines(p: Program, ctx: Context): SimLine[] {
    const out: SimLine[] = [];
    let n = 0;
    for (const rule of p.rules) {
        for (const group of rule.when?.groups ?? []) {
            for (const l of group.lines) {
                n++;
                const lt = lineText(l, ctx);
                const label = `${n}: ` + [lt.subject, lt.param, lt.not ? t('not') : '', lt.comparison].filter(Boolean).join(' ');
                const kind: ValueKind | null = l.time ? null : l.variable ? variableKind(ctx.variables[l.variable]) : {kind: 'free'};
                out.push({n, label, kind, value: l.time ? undefined : (l.value ?? l.from ?? (kind ? firstValue(kind) : undefined))});
            }
        }
    }
    return out;
}

/** What the dry run is asked: the program as it stands (line 0), or a line with the value to try. */
export function simRequest(line: SimLine | undefined, value: Value): {line?: number; value?: Value} {
    if (!line) return {};
    return line.kind ? {line: line.n, value} : {line: line.n};
}
