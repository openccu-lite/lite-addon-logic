// The program editor's own logic: the draft a program is edited as, what is wrong with it and
// why, moving and copying its parts, the device picker's list, a time module's form. The
// components only lay out what these functions say.
import {t} from 'occulite-ui/i18n';

import {commandForm, commandOf, commandProblem, newCommand, type CommandForm} from './command.js';
import {displayForm, displayOf, displayProblem, type DisplayForm} from './display.js';
import {durationText, matches, type Action, type Context, type Line, type Program, type Rule, type TimeModule, type Value, type VariableDef, type Webhook} from './model.js';

/** A datapoint of a channel, as the system describes it. */
export interface Param {
    id: string;
    type: string;
    read: boolean;
    write: boolean;
    min?: number;
    max?: number;
    unit?: string;
    values?: string[];
}

export interface DeviceChannel {
    address: string;
    name?: string;
    type: string;
    rooms?: string[];
    functions?: string[];
}

export interface Device {
    interface: string;
    address: string;
    name?: string;
    type: string;
    channels: DeviceChannel[];
}

/** What the editor knows besides the draft: the variables and programs, and the system's devices and datapoints. */
export interface Env {
    ctx: Context;
    /** The datapoints by channel ("Interface/ADDRESS:CH"); a channel that is not here is not known to the system. */
    params: Record<string, Param[]>;
    /** The names of the webhooks' secrets (not there: not known yet, nothing is said about them). */
    secrets?: string[];
}

export interface DraftLine {
    id: number;
    kind: 'device' | 'variable' | 'time';
    /** "Interface/ADDRESS:CH", '' while none is chosen. */
    channel: string;
    param: string;
    variable: string;
    time: TimeModule | null;
    op: string;
    value: Value;
    from: Value;
    below: Value;
    to: Value;
    not: boolean;
    /** "Hält seit": whether the comparison must have held for a while, and for how long. */
    held: boolean;
    holdN: string;
    holdUnit: string;
    trigger: string;
    note: string;
}

/** A webhook as the dialog has it. */
export interface HookForm {
    method: string;
    url: string;
    headers: {name: string; value: string}[];
    bodyType: string;
    body: string;
    insecure: boolean;
}

export const newHook = (): HookForm => ({method: 'POST', url: '', headers: [], bodyType: 'json', body: '', insecure: false});

export interface DraftAction {
    id: number;
    kind: 'set' | 'variable' | 'program' | 'script' | 'webhook' | 'wake' | 'channel' | 'display' | 'command';
    /** A "command" activity's command. */
    command: CommandForm;
    /** What a "display" activity shows on its channel. */
    display: DisplayForm;
    /** The datapoints a "channel" activity writes to its channel, with their values. */
    values: {param: string; value: Value}[];
    /** A webhook's request; a Wake-on-LAN's hardware address and where its packets go ('' the local network). */
    hook: HookForm;
    mac: string;
    broadcast: string;
    channel: string;
    param: string;
    variable: string;
    program: string;
    /** What a program activity does: run-then, run-check, activate, deactivate. */
    what: string;
    /** Where the written value comes from: value, variable, device, special. */
    source: 'value' | 'variable' | 'device' | 'special';
    to: Value;
    fromVariable: string;
    fromChannel: string;
    fromParam: string;
    special: string;
    script: string;
    delayed: boolean;
    delayN: string;
    delayUnit: string;
    /** "Ein für": the value is sent with the channel's on-time. */
    timed: boolean;
    onForN: string;
    onForUnit: string;
    /** A random delay: between the delay (none: at once) and this, drawn at every run. */
    random: boolean;
    upToN: string;
    upToUnit: string;
    note: string;
}

export interface DraftGroup {
    id: number;
    op: string;
    lines: DraftLine[];
}

export interface DraftRule {
    id: number;
    else: boolean;
    op: string;
    groups: DraftGroup[];
    cancelPending: boolean;
    then: DraftAction[];
    note: string;
}

export interface Draft {
    key: string;
    name: string;
    note: string;
    active: boolean;
    visible: boolean;
    operable: boolean;
    internal: boolean;
    atStart: boolean;
    rules: DraftRule[];
}

let lastId = 0;
const nextId = () => ++lastId;

/** "90min" as a number and a unit: the largest unit that says it in whole numbers. */
export function splitDuration(d: string | undefined): {n: string; unit: string} {
    const m = /^(?:(\d+)h)?(?:(\d+)min)?(?:(\d+)s)?$/.exec(d ?? '');
    if (!m || !d) return {n: '', unit: 'min'};
    const total = Number(m[1] ?? 0) * 3600 + Number(m[2] ?? 0) * 60 + Number(m[3] ?? 0);
    if (total % 3600 === 0 && total > 0) return {n: String(total / 3600), unit: 'h'};
    if (total % 60 === 0 && total > 0) return {n: String(total / 60), unit: 'min'};
    return {n: String(total), unit: 's'};
}

/** A whole number of at least min typed into a field, or null. */
export function wholeNumber(text: string, min = 1): number | null {
    const s = text.trim();
    if (!/^\d{1,9}$/.test(s)) return null;
    const n = Number(s);
    return n >= min ? n : null;
}

/** The duration a file has for a number and a unit; '' when the number is none. */
export function joinDuration(n: string, unit: string): string {
    const v = wholeNumber(n);
    return v === null ? '' : `${v}${unit}`;
}

const splitRef = (ref?: {interface: string; address: string; param: string}) => (ref ? {channel: `${ref.interface}/${ref.address}`, param: ref.param} : {channel: '', param: ''});

const joinRef = (channel: string, param: string) => {
    const [iface = '', address = ''] = channel.split('/');
    return {interface: iface, address, param};
};

export function newLine(kind: DraftLine['kind'] = 'device'): DraftLine {
    return {id: nextId(), kind, channel: '', param: '', variable: '', time: null, op: 'eq', value: undefined, from: undefined, below: undefined, to: undefined, not: false,
        held: false, holdN: '', holdUnit: 'min', trigger: kind === 'time' ? 'update' : 'change', note: ''};
}

export function newAction(kind: DraftAction['kind'] = 'set'): DraftAction {
    return {id: nextId(), kind, values: [], display: displayForm(undefined), command: newCommand(), hook: newHook(), mac: '', broadcast: '', channel: '', param: '', variable: '', program: '', what: 'run-check', source: 'value', to: undefined, fromVariable: '', fromChannel: '', fromParam: '',
        special: '', script: '', delayed: false, delayN: '', delayUnit: 's', timed: false, onForN: '', onForUnit: 'min', random: false, upToN: '', upToUnit: 'min', note: ''};
}

export function newRule(isElse = false): DraftRule {
    return {id: nextId(), else: isElse, op: 'any', groups: [], cancelPending: false, then: [], note: ''};
}

/** A program that does not exist yet: one "If…" without lines. New programs are not evaluated at a system start. */
export function newDraft(): Draft {
    return {key: '', name: '', note: '', active: true, visible: true, operable: true, internal: false, atStart: false, rules: [newRule()]};
}

function draftLine(l: Line): DraftLine {
    const out = newLine(l.time ? 'time' : l.device ? 'device' : 'variable');
    const hold = splitDuration(l.for);
    Object.assign(out, splitRef(l.device), {variable: l.variable ?? '', time: l.time ? {...l.time} : null, op: l.op ?? 'eq', value: l.value, from: l.from, below: l.below, to: l.to,
        not: !!l.not, held: !!l.for, holdN: hold.n, holdUnit: hold.unit, trigger: l.trigger, note: l.note ?? ''});
    return out;
}

function draftAction(a: Action): DraftAction {
    const out = newAction(a.kind === 'set' || a.kind === 'variable' || a.kind === 'script' || a.kind === 'webhook' || a.kind === 'wake' || a.kind === 'channel' || a.kind === 'display' || a.kind === 'command' ? a.kind : 'program');
    out.command = commandForm(a.command);
    out.values = (a.values ?? []).map((v) => ({...v}));
    out.display = displayForm(a.display);
    if (a.webhook) {
        out.hook = {method: a.webhook.method, url: a.webhook.url, headers: (a.webhook.headers ?? []).map((h) => ({...h})), bodyType: a.webhook.body_type || 'json', body: a.webhook.body ?? '', insecure: !!a.webhook.insecure};
    }
    const from = splitRef(a.from_device);
    const delay = splitDuration(a.after);
    const onFor = splitDuration(a.on_for);
    const upTo = splitDuration(a.after_up_to);
    Object.assign(out, a.kind === 'channel' || a.kind === 'display' ? {channel: a.channel ?? '', param: ''} : splitRef(a.device), {
        variable: a.variable ?? '',
        program: a.program ?? '',
        what: a.kind === 'run' ? (a.mode === 'then' ? 'run-then' : 'run-check') : a.kind === 'deactivate' ? 'deactivate' : a.kind === 'activate' ? 'activate' : 'run-check',
        source: a.special ? 'special' : a.from_variable ? 'variable' : a.from_device ? 'device' : 'value',
        to: a.to,
        fromVariable: a.from_variable ?? '',
        fromChannel: from.channel,
        fromParam: from.param,
        special: a.special ?? '',
        script: a.script ?? '',
        delayed: !!a.after || !!a.after_up_to,
        delayN: delay.n,
        delayUnit: a.after ? delay.unit : 's',
        mac: a.wake?.mac ?? '',
        broadcast: a.wake?.broadcast ?? '',
        timed: !!a.on_for,
        onForN: onFor.n,
        onForUnit: onFor.unit,
        random: !!a.after_up_to,
        upToN: upTo.n,
        upToUnit: upTo.unit,
        note: a.note ?? '',
    });
    return out;
}

/** The draft of a program as it is stored. */
export function draftOf(p: Program): Draft {
    return {
        key: p.key,
        name: p.name,
        note: p.note ?? '',
        active: p.active,
        visible: p.visible,
        operable: p.operable,
        internal: !!p.internal,
        atStart: !!p.at_start,
        rules: p.rules.map((r) => ({
            id: nextId(),
            else: !!r.else,
            op: r.when?.op ?? 'any',
            groups: (r.when?.groups ?? []).map((g) => ({id: nextId(), op: g.op, lines: g.lines.map(draftLine)})),
            cancelPending: !!r.cancel_pending,
            then: r.then.map(draftAction),
            note: r.note ?? '',
        })),
    };
}

/** A copy of a program as a new one: its own name, no key, and not active until it is looked at. */
export function copyOf(p: Program): Draft {
    return {...draftOf(p), key: '', name: t('{name} (copy)', {name: p.name}), active: false};
}

function lineOf(l: DraftLine): Line {
    const out: Line = {trigger: l.trigger};
    if (l.kind === 'time') {
        out.time = l.time ?? {kind: 'daily'};
        if (l.trigger !== 'check') out.trigger = 'update';
    } else {
        if (l.kind === 'device') out.device = joinRef(l.channel, l.param);
        else out.variable = l.variable;
        out.op = l.op;
        if (l.op === 'range') Object.assign(out, {from: l.from, below: l.below});
        else if (l.op === 'outside') Object.assign(out, {from: l.from, to: l.to});
        else out.value = l.value;
        const hold = l.held ? joinDuration(l.holdN, l.holdUnit) : '';
        if (hold) out.for = hold;
    }
    if (l.not) out.not = true;
    if (l.note.trim()) out.note = l.note.trim();
    return out;
}

/** "On for" belongs to a fixed value written to a device. */
export const canOnFor = (a: DraftAction) => a.kind === 'set' && a.source === 'value';

/** The webhook a form says, as the API takes it: headers without a name are left out, a GET has no body. */
export function hookOf(f: HookForm): Webhook {
    const out: Webhook = {method: f.method, url: f.url.trim()};
    const headers = f.headers.filter((h) => h.name.trim()).map((h) => ({name: h.name.trim(), value: h.value}));
    if (headers.length > 0) out.headers = headers;
    if (f.method !== 'GET' && f.body !== '') Object.assign(out, {body_type: f.bodyType, body: f.body});
    if (f.insecure) out.insecure = true;
    return out;
}

/** A hardware address written AA:BB:CC:DD:EE:FF; '' when the text is none. */
export function canonicalMAC(text: string): string {
    const s = text.trim();
    const hex = s.replace(/[:-]/g, '').toUpperCase();
    const seps = s.length - hex.length;
    if (!/^[0-9A-F]{12}$/.test(hex) || (seps !== 0 && seps !== 5)) return '';
    return hex.match(/../g)!.join(':');
}

const PLACEHOLDER = /\{(program|trigger|value|time|var:[a-z0-9-]+|secret:[a-z0-9-]+)\}/g;
const HEADER_NAME = /^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,64}$/;
const OWN_HEADERS = ['host', 'content-length', 'transfer-encoding', 'connection'];
export const MAX_HEADERS = 8;
const MAX_BODY = 16 << 10;

/** The placeholders a webhook names, each once. */
export function hookPlaceholders(f: HookForm): string[] {
    const texts = [f.url, ...f.headers.map((h) => h.value), f.method === 'GET' ? '' : f.body];
    return [...new Set(texts.flatMap((text) => [...text.matchAll(PLACEHOLDER)].map((m) => m[1]!)))];
}

/** What is wrong with a webhook; '' when it can be saved. secrets are the names entered in the settings (undefined: not known yet). */
export function hookProblem(f: HookForm, ctx: Context, secrets?: string[]): string {
    const address = f.url.trim();
    if (!address) return t('Enter the address.');
    let url: URL | null = null;
    try {
        url = new URL(address.replace(PLACEHOLDER, 'x'));
    } catch {
        url = null;
    }
    if (/\s/.test(address) || address.length > 2000 || !/^https?:\/\/[^/?#]/i.test(address) || !url || (url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname || url.username || url.password || url.hash || address.includes('#')) {
        return t('The address must be http://… or https://… with a host, without a blank, a user name or a fragment.');
    }
    if (/\{/.test(address.replace(/^[a-z]+:\/\//i, '').split(/[/?]/)[0]!)) return t('A placeholder cannot stand in the address\' host.');
    const names = f.headers.filter((h) => h.name.trim() || h.value).map((h) => h.name.trim().toLowerCase());
    if (names.length > MAX_HEADERS) return t('A webhook has at most {n} headers.', {n: MAX_HEADERS});
    for (const h of f.headers) {
        const name = h.name.trim();
        if (!name && !h.value) continue;
        if (!HEADER_NAME.test(name)) return name ? t('"{name}" is not a header\'s name.', {name}) : t('A header needs a name.');
        if (OWN_HEADERS.includes(name.toLowerCase())) return t('The header {name} is set by the request itself.', {name});
        if (names.filter((n) => n === name.toLowerCase()).length > 1) return t('The header {name} is there twice.', {name});
        if (h.value.length > 1000 || /[\r\n]/.test(h.value)) return t('The value of the header {name} is too long or has a line break.', {name});
    }
    if (f.method !== 'GET' && new TextEncoder().encode(f.body).length > MAX_BODY) return t('The body is longer than {n} bytes.', {n: MAX_BODY});
    for (const p of hookPlaceholders(f)) {
        const [kind, name] = p.split(':');
        if (kind === 'var' && !ctx.variables[name!]) return t('The variable {key} does not exist.', {key: name!});
        if (kind === 'secret' && secrets && !secrets.includes(name!)) return t('There is no secret {name}: enter it in the settings first.', {name: name!});
    }
    return '';
}

const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;

/** What is wrong with a Wake-on-LAN; '' when it can be saved. */
export function wakeProblem(a: DraftAction): string {
    if (!canonicalMAC(a.mac)) return t('Enter the hardware address: AA:BB:CC:DD:EE:FF.');
    return a.broadcast.trim() === '' || IPV4.test(a.broadcast.trim()) ? '' : t('The broadcast address is an IPv4 address like 192.168.1.255, or empty.');
}

function actionOf(a: DraftAction): Action {
    const out: Action = {kind: a.kind};
    switch (a.kind) {
        case 'channel':
            out.channel = a.channel;
            out.values = a.values.map((v) => ({...v}));
            break;
        case 'display':
            out.channel = a.channel;
            out.display = displayOf(a.display);
            break;
        case 'webhook':
            out.webhook = hookOf(a.hook);
            break;
        case 'command':
            out.command = commandOf(a.command);
            break;
        case 'wake':
            out.wake = {mac: canonicalMAC(a.mac) || a.mac.trim()};
            if (a.broadcast.trim()) out.wake.broadcast = a.broadcast.trim();
            break;
        case 'script':
            out.script = a.script;
            break;
        case 'program':
            out.program = a.program;
            if (a.what === 'activate' || a.what === 'deactivate') out.kind = a.what;
            else Object.assign(out, {kind: 'run', mode: a.what === 'run-then' ? 'then' : 'check'});
            break;
        default:
            if (a.kind === 'set') out.device = joinRef(a.channel, a.param);
            else out.variable = a.variable;
            if (a.source === 'special') out.special = a.special;
            else if (a.source === 'variable') out.from_variable = a.fromVariable;
            else if (a.source === 'device') out.from_device = joinRef(a.fromChannel, a.fromParam);
            else out.to = a.to;
            if (canOnFor(a) && a.timed && joinDuration(a.onForN, a.onForUnit)) out.on_for = joinDuration(a.onForN, a.onForUnit);
    }
    const after = a.delayed ? joinDuration(a.delayN, a.delayUnit) : '';
    if (after) out.after = after;
    const upTo = a.delayed && a.random && a.kind !== 'script' ? joinDuration(a.upToN, a.upToUnit) : '';
    if (upTo) out.after_up_to = upTo;
    if (a.note.trim()) out.note = a.note.trim();
    return out;
}

/** The program a draft says, as the API takes it. */
export function programOf(d: Draft): Program {
    const rules: Rule[] = d.rules.map((r) => {
        const out: Rule = r.else ? {else: true, then: r.then.map(actionOf)} : {when: {op: r.op, groups: r.groups.map((g) => ({op: g.op, lines: g.lines.map(lineOf)}))}, then: r.then.map(actionOf)};
        if (r.cancelPending) out.cancel_pending = true;
        if (r.note.trim()) out.note = r.note.trim();
        return out;
    });
    const out: Program = {key: d.key, name: d.name.trim(), active: d.active, visible: d.visible, operable: d.operable, rules};
    if (d.note.trim()) out.note = d.note.trim();
    if (d.internal) out.internal = true;
    if (d.atStart) out.at_start = true;
    return out;
}

/** Whether two drafts say the same program. */
export function sameProgram(a: Draft, b: Draft): boolean {
    return JSON.stringify(programOf(a)) === JSON.stringify(programOf(b));
}

// --- values ---

/** How a value is entered: what kind of field, and what it offers. */
export interface ValueKind {
    /** bool: a choice of two; choice: one of options; number, text; free: typed, and read as what it looks like. */
    kind: 'bool' | 'choice' | 'number' | 'text' | 'free';
    options?: {value: Value; label: string}[];
    unit?: string;
    min?: number;
    max?: number;
}

export function variableKind(def: VariableDef | undefined): ValueKind {
    switch (def?.type) {
        case 'bool':
        case 'alarm':
            return {kind: 'bool', options: [{value: true, label: def.labels?.true || t('true')}, {value: false, label: def.labels?.false || t('false')}]};
        case 'list':
            return {kind: 'choice', options: (def.entries ?? []).map((e) => ({value: e, label: e}))};
        case 'number':
            return {kind: 'number', unit: def.unit, min: def.min, max: def.max};
        case 'string':
            return {kind: 'text'};
    }
    return {kind: 'free'};
}

export function paramKind(p: Param | undefined): ValueKind {
    switch (p?.type) {
        case 'BOOL':
        case 'ACTION':
            return {kind: 'bool', options: [{value: true, label: t('true')}, {value: false, label: t('false')}]};
        case 'ENUM':
            return {kind: 'choice', options: (p.values ?? []).map((label, i) => ({value: i, label: label || String(i)}))};
        case 'FLOAT':
        case 'INTEGER':
            return {kind: 'number', unit: p.unit, min: p.min, max: p.max};
        case 'STRING':
            return {kind: 'text'};
    }
    return {kind: 'free'};
}

/** How a value of a "channel" activity is entered: as paramKind, but an ENUM's entry is its name - the file says COLOR: RED. */
export function valuesKind(p: Param | undefined): ValueKind {
    if (p?.type !== 'ENUM') return paramKind(p);
    return {kind: 'choice', options: (p.values ?? []).map((label, i) => ({value: label || i, label: label || String(i)}))};
}

/** A value as valuesKind has it: an ENUM's entry given by its number becomes its name. */
export function namedValue(p: Param | undefined, v: Value): Value {
    return p?.type === 'ENUM' && typeof v === 'number' && p.values?.[v] ? p.values[v] : v;
}

/** The most datapoints one "channel" activity writes. */
export const MAX_VALUES = 32;

function rangeProblem(kind: ValueKind, v: Value): string {
    if (kind.kind !== 'number' || typeof v !== 'number') return '';
    if ((kind.min !== undefined && v < kind.min) || (kind.max !== undefined && v > kind.max)) return t('outside {min} to {max}', {min: kind.min ?? '…', max: kind.max ?? '…'});
    return '';
}

/** What is wrong with the values a "channel" activity writes; '' when they can be saved. */
export function valuesProblem(channel: string, values: {param: string; value: Value}[], env: Env): string {
    if (!channel) return t('Choose a device.');
    if (values.length === 0) return t('Choose the datapoints to write.');
    if (values.length > MAX_VALUES) return t('One activity writes at most {n} datapoints.', {n: MAX_VALUES});
    const known = env.params[channel];
    const seen = new Set<string>();
    for (const v of values) {
        if (!/^[A-Za-z0-9_.]{1,64}$/.test(v.param)) return t('"{param}" is not a datapoint\'s name.', {param: v.param});
        if (seen.has(v.param)) return t('The datapoint {param} is there twice.', {param: v.param});
        seen.add(v.param);
        const p = known?.find((x) => x.id === v.param);
        if (known && !p?.write) return t('This channel has no datapoint {param} to write.', {param: v.param});
        const kind = valuesKind(p);
        const problem = valueProblem(kind, namedValue(p, v.value)) || rangeProblem(kind, v.value);
        if (problem) return v.param + ': ' + problem;
    }
    return '';
}

/** The values of a "channel" activity in one line: "COLOR = RED, LEVEL = 0.5". */
export function valuesText(values: {param: string; value: Value}[]): string {
    return values.map((v) => v.param + ' = ' + typedText(v.value)).join(', ');
}

export function paramOf(env: Env, channel: string, param: string): Param | undefined {
    return env.params[channel]?.find((p) => p.id === param);
}

/** How the value of a line or an activity is entered. */
export function lineKind(l: {kind: string; channel: string; param: string; variable: string}, env: Env): ValueKind {
    return l.kind === 'variable' ? variableKind(env.ctx.variables[l.variable]) : paramKind(paramOf(env, l.channel, l.param));
}

/** The comparisons a line offers: "equal" and "not equal" for what is not a number. */
export function opsFor(kind: ValueKind): string[] {
    return kind.kind === 'bool' || kind.kind === 'text' || (kind.kind === 'choice' && typeof kind.options?.[0]?.value !== 'number') ? ['eq', 'ne'] : ['eq', 'ne', 'gt', 'ge', 'lt', 'le', 'range', 'outside'];
}

/** What was typed, as a value of that kind; text that is no number stays text, and is said to be wrong where a number is needed. */
export function parseTyped(kind: ValueKind['kind'], text: string): Value {
    if (kind === 'text') return text;
    const s = text.trim().replace(',', '.');
    if (s !== '' && Number.isFinite(Number(s))) return Number(s);
    if (kind === 'free' && (s === 'true' || s === 'false')) return s === 'true';
    return text;
}

/** A value as it stands in its field. */
export function typedText(v: Value): string {
    return v === undefined || v === null ? '' : String(v);
}

/** The value a field of that kind starts with. */
export function firstValue(kind: ValueKind): Value {
    switch (kind.kind) {
        case 'bool':
            return true;
        case 'choice':
            return kind.options?.[0]?.value;
        case 'text':
            return '';
    }
    return undefined;
}

/** What is wrong with a value for its kind; '' when it fits. */
export function valueProblem(kind: ValueKind, v: Value, numeric = false): string {
    switch (kind.kind) {
        case 'bool':
            return typeof v === 'boolean' ? '' : t('Choose a value.');
        case 'choice':
            return (kind.options ?? []).some((o) => o.value === v) ? '' : t('Choose one of the entries.');
        case 'number':
            return typeof v === 'number' ? '' : t('Enter a number.');
        case 'text':
            return typeof v === 'string' ? '' : t('Enter a text.');
    }
    if (numeric) return typeof v === 'number' ? '' : t('Enter a number.');
    return v === undefined || v === null || v === '' ? t('Enter a value.') : '';
}

// --- what is wrong, and why ---

const SYSTEM_VALUES = ['alarm-messages', 'service-messages', 'duty-cycle', 'sun-azimuth', 'sun-elevation'];

function lineProblem(l: DraftLine, env: Env): string {
    if (l.kind === 'time') return l.time ? timeProblem(l.time) : t('Set the time.');
    if (l.kind === 'device') {
        if (!l.channel) return t('Choose a device.');
        if (!l.param) return t('Choose a datapoint.');
        const known = env.params[l.channel];
        if (known && !known.some((p) => p.id === l.param && p.read)) return t('This channel has no datapoint {param} to look at.', {param: l.param});
    } else {
        if (!l.variable) return t('Choose a variable.');
        if (!env.ctx.variables[l.variable]) return t('The variable {key} does not exist.', {key: l.variable});
    }
    const kind = lineKind(l, env);
    if (!opsFor(kind).includes(l.op)) return t('This can only be compared for "equal" and "not equal".');
    if (l.op === 'range') return valueProblem({kind: 'number'}, l.from) || valueProblem({kind: 'number'}, l.below);
    if (l.op === 'outside') return valueProblem({kind: 'number'}, l.from) || valueProblem({kind: 'number'}, l.to);
    return valueProblem(kind, l.value, l.op !== 'eq' && l.op !== 'ne') || holdProblem(l);
}

const MAX_HOLD = 7 * 86400;
const UNIT_SECONDS: Record<string, number> = {s: 1, min: 60, h: 3600};
/** What a number and a unit say in seconds; null when the number is none. */
const secondsOf = (n: string, unit: string, min = 1) => {
    const v = wholeNumber(n, min);
    return v === null ? null : v * (UNIT_SECONDS[unit] ?? 1);
};
const within = (n: string, unit: string) => {
    const s = secondsOf(n, unit);
    return s !== null && s <= MAX_HOLD;
};

/** What is wrong with a line's "holds for"; '' when it is fine or not asked for. */
export function holdProblem(l: {kind: string; held: boolean; holdN: string; holdUnit: string}): string {
    if (!l.held || l.kind === 'time') return '';
    return within(l.holdN, l.holdUnit) ? '' : t('Enter how long the comparison must have held: a whole number, seven days at most.');
}

/** What is wrong with an activity's "on for"; '' when it is fine or not asked for. */
export function onForProblem(a: DraftAction, env: Env): string {
    if (!a.timed || !canOnFor(a)) return '';
    const known = env.params[a.channel];
    if (known && !known.some((p) => p.id === 'ON_TIME' && p.write)) return t('This channel has no on-time (ON_TIME): "on for" is not possible here.');
    return within(a.onForN, a.onForUnit) ? '' : t('Enter how long it stays on: a whole number, seven days at most.');
}

/** What is wrong with an activity's delay, the random one included; '' when it is fine. */
export function delayProblem(a: DraftAction): string {
    if (!a.delayed) return '';
    if (!a.random || a.kind === 'script') return wholeNumber(a.delayN) === null ? t('Enter the delay as a whole number.') : '';
    const from = a.delayN.trim() === '' ? 0 : secondsOf(a.delayN, a.delayUnit, 0);
    if (from === null) return t('Enter the delay as a whole number.');
    const upTo = secondsOf(a.upToN, a.upToUnit);
    if (upTo === null) return t('Enter the longest delay as a whole number.');
    return upTo > from ? '' : t('The longest delay must be longer than the shortest.');
}

/** What a line has beyond its row, in words; '' for nothing. */
export function lineExtras(l: DraftLine): string {
    const hold = l.kind !== 'time' && l.held ? joinDuration(l.holdN, l.holdUnit) : '';
    return hold ? t('for at least {d}', {d: durationText(hold)}) : '';
}

/** What an activity has beyond its row, in words; '' for nothing. */
export function actionExtras(a: DraftAction): string {
    const out: string[] = [];
    const onFor = canOnFor(a) && a.timed ? joinDuration(a.onForN, a.onForUnit) : '';
    if (onFor) out.push(t('on for {d}', {d: durationText(onFor)}));
    const upTo = a.delayed && a.random && a.kind !== 'script' ? joinDuration(a.upToN, a.upToUnit) : '';
    if (upTo) out.push(t('delayed at random, by {d} at most', {d: durationText(upTo)}));
    return out.join(' · ');
}

const durationWords = (d: string) => d.replace(/(\d+)(h|min|s)/g, '$1 $2 ').trim();

function actionProblem(a: DraftAction, d: Draft, env: Env): string {
    const delay = delayProblem(a);
    if (delay) return delay;
    switch (a.kind) {
        case 'channel':
            return valuesProblem(a.channel, a.values, env);
        case 'display':
            return displayProblem(a.channel, a.display, env);
        case 'webhook':
            return hookProblem(a.hook, env.ctx, env.secrets);
        case 'command':
            return commandProblem(a.command, env.ctx, env.secrets);
        case 'wake':
            return wakeProblem(a);
        case 'script':
            return d.active ? t('A program with a script cannot be active.') : '';
        case 'program':
            if (!a.program) return t('Choose a program.');
            return a.program in env.ctx.programs ? '' : t('The program {key} does not exist.', {key: a.program});
        case 'set': {
            if (!a.channel) return t('Choose a device.');
            if (!a.param) return t('Choose a datapoint.');
            const known = env.params[a.channel];
            if (known && !known.some((p) => p.id === a.param && p.write)) return t('This channel has no datapoint {param} to write.', {param: a.param});
            break;
        }
        default:
            if (!a.variable) return t('Choose a variable.');
            if (SYSTEM_VALUES.includes(a.variable)) return t('A system value cannot be written.');
            if (!env.ctx.variables[a.variable]) return t('The variable {key} does not exist.', {key: a.variable});
    }
    switch (a.source) {
        case 'special':
            return '';
        case 'variable':
            if (!a.fromVariable) return t('Choose a variable.');
            return env.ctx.variables[a.fromVariable] ? '' : t('The variable {key} does not exist.', {key: a.fromVariable});
        case 'device':
            return a.fromChannel && a.fromParam ? '' : t('Choose a datapoint.');
    }
    return valueProblem(lineKind(a, env), a.to) || onForProblem(a, env);
}

/** What is wrong with the draft, by where: "name", "l<id>" a line, "a<id>" an activity. Empty when it can be saved. */
export function draftProblems(d: Draft, env: Env): Record<string, string> {
    const out: Record<string, string> = {};
    const put = (where: string, what: string) => {
        if (what) out[where] = what;
    };
    if (d.name.trim() === '') put('name', t('A program needs a name.'));
    else if ([...d.name.trim()].length > 200) put('name', t('The name is too long: 200 characters at most.'));
    for (const r of d.rules) {
        for (const g of r.groups) for (const l of g.lines) put('l' + l.id, lineProblem(l, env));
        for (const a of r.then) put('a' + a.id, actionProblem(a, d, env));
    }
    return out;
}

/** What the draft will do that its author may not expect - no errors, hints. */
export function draftHints(d: Draft): string[] {
    const out: string[] = [];
    const lines = d.rules.flatMap((r) => r.groups.flatMap((g) => g.lines));
    if (lines.length > 0 && !lines.some((l) => l.trigger !== 'check' || l.kind === 'time')) {
        out.push(t('No line triggers this program: it only runs when it is started by hand, by another program or at a system start.'));
    }
    const later = d.rules.slice(1).some((r) => r.then.length > 0);
    if (later && lines.some((l) => l.kind === 'device' && l.trigger === 'update')) {
        out.push(t('"Trigger on update" evaluates the program at every report of the device, the same value included: "Else" then runs each time the first condition is not true.'));
    } else if (later && lines.some((l) => l.trigger === 'change')) {
        out.push(t('"Trigger on change" also runs the program when a line stops being true: then "Else" runs.'));
    }
    if (d.rules.some((r) => r.then.some((a) => a.source === 'special'))) out.push(t('An activity with a special value of the CCU is skipped until it gets a value here.'));
    return out;
}

// --- moving, copying, removing ---

function findLine(d: Draft, id: number): {rule: DraftRule; group: DraftGroup; index: number} | null {
    for (const rule of d.rules) {
        for (const group of rule.groups) {
            const index = group.lines.findIndex((l) => l.id === id);
            if (index >= 0) return {rule, group, index};
        }
    }
    return null;
}

const prune = (d: Draft) => {
    for (const r of d.rules) r.groups = r.groups.filter((g) => g.lines.length > 0);
};

/** Puts a line where another one is (dragged onto it): before it, or after it when it came from above in the same group. */
export function moveLine(d: Draft, id: number, onto: number): void {
    const from = findLine(d, id);
    const to = findLine(d, onto);
    if (!from || !to || id === onto) return;
    const [line] = from.group.lines.splice(from.index, 1);
    to.group.lines.splice(to.index, 0, line!);
    prune(d);
}

/** Moves a line one place up or down in its rule: inside its group, or over the edge into the group beside it. */
export function stepLine(d: Draft, id: number, delta: -1 | 1): void {
    const at = findLine(d, id);
    if (!at) return;
    const {rule, group, index} = at;
    const target = index + delta;
    if (target >= 0 && target < group.lines.length) {
        [group.lines[index], group.lines[target]] = [group.lines[target]!, group.lines[index]!];
        return;
    }
    const next = rule.groups[rule.groups.indexOf(group) + delta];
    if (!next) return;
    const [line] = group.lines.splice(index, 1);
    if (delta < 0) next.lines.push(line!);
    else next.lines.unshift(line!);
    prune(d);
}

export function removeLine(d: Draft, id: number): void {
    const at = findLine(d, id);
    if (!at) return;
    at.group.lines.splice(at.index, 1);
    prune(d);
}

/** A new line at the end of a group, or - without one - in a new group at the end of the rule. */
export function addLine(rule: DraftRule, group?: DraftGroup): DraftLine {
    const line = newLine();
    if (group) group.lines.push(line);
    else rule.groups.push({id: nextId(), op: 'all', lines: [line]});
    return line;
}

function findAction(d: Draft, id: number): {rule: DraftRule; index: number} | null {
    for (const rule of d.rules) {
        const index = rule.then.findIndex((a) => a.id === id);
        if (index >= 0) return {rule, index};
    }
    return null;
}

/** Puts an activity where another one is, in its rule or another. */
export function moveAction(d: Draft, id: number, onto: number): void {
    const from = findAction(d, id);
    const to = findAction(d, onto);
    if (!from || !to || id === onto) return;
    const [action] = from.rule.then.splice(from.index, 1);
    to.rule.then.splice(to.index, 0, action!);
}

export function stepAction(d: Draft, id: number, delta: -1 | 1): void {
    const at = findAction(d, id);
    if (!at) return;
    const target = at.index + delta;
    if (target < 0 || target >= at.rule.then.length) return;
    const list = at.rule.then;
    [list[at.index], list[target]] = [list[target]!, list[at.index]!];
}

export function removeAction(d: Draft, id: number): void {
    const at = findAction(d, id);
    if (at) at.rule.then.splice(at.index, 1);
}

/** A copy of a rule, as the "Else if…" right after it. */
export function copyRule(d: Draft, id: number): void {
    const index = d.rules.findIndex((r) => r.id === id);
    const rule = d.rules[index];
    if (!rule || rule.else) return;
    const copy: DraftRule = {
        ...rule,
        id: nextId(),
        groups: rule.groups.map((g) => ({...g, id: nextId(), lines: g.lines.map((l) => ({...l, id: nextId(), time: l.time ? {...l.time} : null}))})),
        then: rule.then.map((a) => ({...a, id: nextId()})),
    };
    d.rules.splice(index + 1, 0, copy);
}

/** A new "Else if…" before the "Else…", or the "Else…" itself (there is one at most). */
export function addRule(d: Draft, isElse: boolean): void {
    const hasElse = d.rules.some((r) => r.else);
    if (isElse) {
        if (!hasElse) d.rules.push(newRule(true));
        return;
    }
    d.rules.splice(hasElse ? d.rules.length - 1 : d.rules.length, 0, newRule());
}

export function removeRule(d: Draft, id: number): void {
    d.rules = d.rules.filter((r) => r.id !== id);
}

/** Moves an "If…" or "Else if…" one place; the "Else…" stays last. */
export function stepRule(d: Draft, id: number, delta: -1 | 1): void {
    const index = d.rules.findIndex((r) => r.id === id);
    const target = index + delta;
    if (index < 0 || target < 0 || target >= d.rules.length || d.rules[index]!.else || d.rules[target]!.else) return;
    [d.rules[index], d.rules[target]] = [d.rules[target]!, d.rules[index]!];
}

/** The channels a draft names, each once. */
export function draftChannels(d: Draft): string[] {
    const out = new Set<string>();
    for (const r of d.rules) {
        for (const g of r.groups) for (const l of g.lines) if (l.kind === 'device' && l.channel) out.add(l.channel);
        for (const a of r.then) {
            if ((a.kind === 'set' || a.kind === 'channel' || a.kind === 'display') && a.channel) out.add(a.channel);
            if (a.fromChannel) out.add(a.fromChannel);
            if (a.kind === 'command') for (const arg of a.command.args) if (arg.kind === 'device' && arg.channel) out.add(arg.channel);
        }
    }
    return [...out];
}

// --- the device picker ---

export interface PickerRow {
    /** "Interface/ADDRESS:CH". */
    channel: string;
    name: string;
    device: string;
    deviceType: string;
    channelType: string;
    interface: string;
    address: string;
    rooms: string[];
}

export interface PickerFilter {
    query: string;
    interface: string;
    room: string;
    type: string;
}

/** Every channel of the devices as a row of the picker. */
export function pickerRows(devices: Device[]): PickerRow[] {
    return devices.flatMap((d) =>
        d.channels.map((c) => ({channel: `${d.interface}/${c.address}`, name: c.name || d.name || '', device: d.name || d.address, deviceType: d.type, channelType: c.type, interface: d.interface,
            address: c.address, rooms: c.rooms ?? []})),
    );
}

/** What the picker's filters offer: the interfaces, rooms and device types there are. */
export function pickerOptions(rows: PickerRow[]): {interfaces: string[]; rooms: string[]; types: string[]} {
    const sorted = (xs: string[]) => [...new Set(xs)].sort((a, b) => a.localeCompare(b));
    return {interfaces: sorted(rows.map((r) => r.interface)), rooms: sorted(rows.flatMap((r) => r.rooms)), types: sorted(rows.map((r) => r.deviceType))};
}

export function filterRows(rows: PickerRow[], f: PickerFilter): PickerRow[] {
    return rows.filter(
        (r) =>
            (!f.interface || r.interface === f.interface) &&
            (!f.room || r.rooms.includes(f.room)) &&
            (!f.type || r.deviceType === f.type) &&
            matches(f.query, r.name, r.device, r.deviceType, r.channelType, r.address, ...r.rooms),
    );
}

/** The names of the channels the devices' list knows, by "Interface/ADDRESS:CH". */
export function channelNames(devices: Device[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const r of pickerRows(devices)) if (r.name) out[r.channel] = r.name;
    return out;
}

/** The types of the channels the devices' list knows (DIMMER_VIRTUAL_RECEIVER …), by "Interface/ADDRESS:CH". */
export function channelTypes(devices: Device[]): Record<string, string> {
    const out: Record<string, string> = {};
    for (const r of pickerRows(devices)) out[r.channel] = r.channelType;
    return out;
}

// --- a time module's form ---

export const TIME_KINDS = ['once', 'interval', 'daily', 'weekly', 'monthly', 'yearly'];
export const ASTRO_KINDS = ['sunrise', 'before_sunrise', 'day', 'sunset', 'before_sunset', 'night'];
export const TWILIGHTS = ['civil', 'nautical', 'astronomical'];
export const WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

export interface TimeForm {
    kind: string;
    /** at, all_day, or one of the astro kinds. */
    when: string;
    at: string;
    offsetMin: string;
    ranged: boolean;
    durationN: string;
    durationUnit: string;
    intervalN: string;
    intervalUnit: string;
    every: string;
    /** daily: all, workdays, weekend. */
    days: string;
    weekdays: string[];
    /** monthly and yearly: day (the n-th day of the month) or weekday (the n-th of one weekday). */
    by: string;
    day: string;
    nth: string;
    weekday: string;
    month: string;
    date: string;
    from: string;
    until: string;
    count: string;
    /** An astro module as a point instead of a range, and the twilight ('', civil, nautical, astronomical) its sunrise and sunset mean. */
    point: boolean;
    twilight: string;
}

export function timeFormOf(m: TimeModule | null): TimeForm {
    const duration = splitDuration(m?.duration);
    const interval = splitDuration(m?.interval);
    const offset = splitDuration(m?.offset);
    const set = (m?.weekdays ?? []).join(',');
    const byWeekday = !!m?.nth;
    return {
        kind: m?.kind ?? 'daily',
        when: m?.astro ?? (m?.all_day ? 'all_day' : 'at'),
        at: m?.at ?? (m ? '' : '07:00'),
        offsetMin: m?.offset ? String(Number(offset.n) * (offset.unit === 'h' ? 60 : offset.unit === 'min' ? 1 : 1 / 60)) : '',
        ranged: !!m?.duration,
        durationN: duration.n,
        durationUnit: m?.duration ? duration.unit : 'h',
        intervalN: interval.n,
        intervalUnit: m?.interval ? interval.unit : 'min',
        every: m?.every ? String(m.every) : '',
        days: m?.kind === 'daily' && set === 'mon,tue,wed,thu,fri' ? 'workdays' : m?.kind === 'daily' && set === 'sat,sun' ? 'weekend' : 'all',
        weekdays: m?.kind === 'weekly' ? [...(m.weekdays ?? [])] : [],
        by: byWeekday ? 'weekday' : 'day',
        day: m?.day ? String(m.day) : '1',
        nth: m?.nth ? String(m.nth) : '1',
        weekday: byWeekday ? (m?.weekdays?.[0] ?? 'mon') : 'mon',
        month: m?.month ? String(m.month) : '1',
        date: m?.date ?? '',
        from: m?.from ?? '',
        until: m?.until ?? '',
        count: m?.count ? String(m.count) : '',
        point: !!m?.point,
        twilight: m?.twilight ?? '',
    };
}

/** Whether the "when" of a form is the sun's and takes an offset. */
export const astroWithOffset = (when: string) => ['before_sunrise', 'day', 'before_sunset', 'night'].includes(when);

/** The time module a form says. Fields that hold nothing usable are left out: timeProblem says what is missing. */
export function timeOf(f: TimeForm): TimeModule {
    const m: TimeModule = {kind: f.kind};
    const int = (s: string) => wholeNumber(s) ?? undefined;
    if (f.when === 'at') {
        m.at = f.at;
        if (f.ranged && f.kind !== 'interval') m.duration = joinDuration(f.durationN, f.durationUnit) || '0s';
    } else if (f.when === 'all_day') {
        m.all_day = true;
    } else {
        m.astro = f.when;
        if (astroWithOffset(f.when) && !['', '0'].includes(f.offsetMin.trim())) m.offset = joinDuration(f.offsetMin, 'min') || '0s';
        if (f.ranged) m.duration = joinDuration(f.durationN, f.durationUnit) || '0s';
        else if (f.point) m.point = true;
        if (f.twilight) m.twilight = f.twilight;
    }
    if (f.kind === 'interval') m.interval = joinDuration(f.intervalN, f.intervalUnit) || '0s';
    if (['daily', 'weekly', 'monthly'].includes(f.kind) && f.every.trim() !== '' && f.every.trim() !== '1') m.every = int(f.every) ?? -1;
    if (f.kind === 'daily' && f.days !== 'all') m.weekdays = f.days === 'workdays' ? ['mon', 'tue', 'wed', 'thu', 'fri'] : ['sat', 'sun'];
    if (f.kind === 'weekly') m.weekdays = WEEKDAYS.filter((d) => f.weekdays.includes(d));
    if (f.kind === 'monthly' || f.kind === 'yearly') {
        if (f.by === 'weekday') Object.assign(m, {nth: int(f.nth) ?? -1, weekdays: [f.weekday]});
        else m.day = int(f.day) ?? -1;
        if (f.kind === 'yearly') m.month = int(f.month) ?? -1;
    }
    if (f.kind === 'once') m.date = f.date;
    if (f.from.trim()) m.from = f.from.trim();
    if (f.until.trim()) m.until = f.until.trim();
    if (f.count.trim() !== '') m.count = int(f.count) ?? -1;
    return m;
}

const CLOCK = /^([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?$/;
const DATE = /^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])( ([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?)?$/;
const seconds = (d: string | undefined) => {
    const s = splitDuration(d);
    return Number(s.n || 0) * (s.unit === 'h' ? 3600 : s.unit === 'min' ? 60 : 1);
};
const pad = (s: string) => (s.length === 10 ? s + ' 00:00' : s);

/** What is wrong with a time module, as the scheduler would say; '' when it can run. */
export function timeProblem(m: TimeModule): string {
    if (!TIME_KINDS.includes(m.kind)) return t('Choose how the time repeats.');
    if (m.at !== undefined && !m.astro && !m.all_day && !CLOCK.test(m.at)) return t('Enter the time of day as HH:MM.');
    if (m.kind === 'interval' && seconds(m.interval) <= 0) return t('Enter the interval as a whole number.');
    if (m.duration !== undefined && (seconds(m.duration) <= 0 || seconds(m.duration) > 7 * 86400)) return t('Enter how long the range lasts: a whole number, seven days at most.');
    if (m.offset !== undefined && (seconds(m.offset) <= 0 || seconds(m.offset) >= 86400)) return t('Enter the offset in minutes: a whole number, less than a day.');
    if (m.every !== undefined && (m.every < 1 || m.every > 10000)) return t('Enter "every" as a whole number.');
    if (m.kind === 'once' && !(DATE.test(m.date ?? '') && m.date!.length === 10)) return t('Enter the date as YYYY-MM-DD.');
    if (m.kind === 'weekly' && (m.weekdays ?? []).length === 0) return t('Choose at least one weekday.');
    if ((m.kind === 'monthly' || m.kind === 'yearly') && m.day !== undefined && (m.day < 1 || m.day > 31)) return t('The day of the month is 1 to 31.');
    if ((m.kind === 'monthly' || m.kind === 'yearly') && m.nth !== undefined && (m.nth < 1 || m.nth > 5)) return t('The weekday\'s number in the month is 1 to 5.');
    if (m.kind === 'yearly' && (m.month === undefined || m.month < 1 || m.month > 12)) return t('The month is 1 to 12.');
    if (m.from !== undefined && !DATE.test(m.from)) return t('Enter "valid from" as YYYY-MM-DD or YYYY-MM-DD HH:MM.');
    if (m.until !== undefined && !DATE.test(m.until)) return t('Enter "valid until" as YYYY-MM-DD or YYYY-MM-DD HH:MM.');
    if (m.count !== undefined && (m.count < 1 || m.count > 1000000)) return t('Enter the number of times as a whole number.');
    if (((m.every ?? 0) > 1 || (m.count ?? 0) > 0) && !m.from) return t('"Every" and the number of times are counted from "valid from": enter it.');
    // ("valid until" a date without a time includes that day)
    if (m.from && m.until && (m.until.length === 10 ? m.until + ' 24:00' : m.until) <= pad(m.from)) return t('"Valid until" must be after "valid from".');
    return '';
}
