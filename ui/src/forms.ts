// The forms of a "channel" activity: a comfortable layout over "several values in one command"
// for the channels of a signal light, a sound, a siren, a coloured light and a blind with slats.
// A form is chosen by the channel's type, offers what the device's own description lists, and
// reads and writes the same values the plain form does - nothing else is stored. The strings a
// CCU program keeps in COMBINED_PARAMETER are read here too, for "convert into a form".
import {t} from 'occulite-ui/i18n';

import type {Param} from './editor.js';
import type {Value} from './model.js';

export type FormKind = 'signal' | 'sound' | 'siren' | 'light' | 'blind';

export interface Stored {
    param: string;
    value: Value;
}

/** The channel types that have a form, and the datapoints a channel must take for it. */
const CLASSES: {types: string[]; kind: FormKind; needs: string[]}[] = [
    {types: ['DIMMER_VIRTUAL_RECEIVER', 'OPTICAL_SIGNAL_RECEIVER'], kind: 'signal', needs: ['COLOR', 'LEVEL']},
    {types: ['ACOUSTIC_SIGNAL_VIRTUAL_RECEIVER'], kind: 'sound', needs: ['SOUNDFILE', 'LEVEL']},
    {types: ['ALARM_SWITCH_VIRTUAL_RECEIVER'], kind: 'siren', needs: ['ACOUSTIC_ALARM_SELECTION', 'OPTICAL_ALARM_SELECTION']},
    {types: ['UNIVERSAL_LIGHT_RECEIVER'], kind: 'light', needs: ['LEVEL']},
    {types: ['BLIND_VIRTUAL_RECEIVER'], kind: 'blind', needs: ['LEVEL', 'LEVEL_2']},
];

const writable = (params: Param[], id: string) => params.find((p) => p.id === id && p.write);

/** The form a channel of this type has, when its description has what the form writes; none otherwise. */
export function formKind(type: string | undefined, params: Param[] | undefined): FormKind | undefined {
    const c = CLASSES.find((x) => x.types.includes(type ?? ''));
    return c && params && c.needs.every((id) => writable(params, id)) ? c.kind : undefined;
}

export interface FormField {
    id: string;
    label: string;
    /** swatch, choice: an entry of a list; percent: a share shown as 0 to 100; number; span: a number with its unit; hue: hue and saturation. */
    type: 'swatch' | 'choice' | 'percent' | 'number' | 'span' | 'hue';
    /** The datapoints it writes, as the description has them: one, or value and unit, or hue and saturation. */
    defs: Param[];
    /** Whether it can be left out of the command. */
    optional: boolean;
    /** The fields that are not sent together with this one. */
    excludes: string[];
}

/** What a field shows: whether it is sent, its value, and its unit (a span) or saturation (hue). */
export interface FieldState {
    on: boolean;
    a: Value;
    b: Value;
}

interface Spec {
    id: string;
    label: () => string;
    type: FormField['type'];
    params: string[];
    optional?: boolean;
    excludes?: string[];
}

const DURATION: Spec = {id: 'duration', label: () => t('For'), type: 'span', params: ['DURATION_VALUE', 'DURATION_UNIT'], optional: true};
const RAMP: Spec = {id: 'ramp', label: () => t('Ramp'), type: 'span', params: ['RAMP_TIME_VALUE', 'RAMP_TIME_UNIT'], optional: true};

const SPECS: Record<FormKind, Spec[]> = {
    signal: [
        {id: 'color', label: () => t('Colour'), type: 'swatch', params: ['COLOR']},
        {id: 'level', label: () => t('Brightness'), type: 'percent', params: ['LEVEL']},
        {id: 'behaviour', label: () => t('Behaviour'), type: 'choice', params: ['COLOR_BEHAVIOUR']},
        DURATION,
        RAMP,
    ],
    sound: [
        {id: 'sound', label: () => t('Sound'), type: 'choice', params: ['SOUNDFILE']},
        {id: 'level', label: () => t('Volume'), type: 'percent', params: ['LEVEL']},
        {id: 'repetitions', label: () => t('Repetitions'), type: 'choice', params: ['REPETITIONS'], optional: true},
        DURATION,
        RAMP,
    ],
    siren: [
        {id: 'acoustic', label: () => t('Sound'), type: 'choice', params: ['ACOUSTIC_ALARM_SELECTION']},
        {id: 'optical', label: () => t('Light'), type: 'choice', params: ['OPTICAL_ALARM_SELECTION']},
        {...DURATION, optional: false},
    ],
    light: [
        {id: 'hue', label: () => t('Colour'), type: 'hue', params: ['HUE', 'SATURATION'], optional: true, excludes: ['temperature']},
        {id: 'temperature', label: () => t('Colour temperature'), type: 'number', params: ['COLOR_TEMPERATURE'], optional: true, excludes: ['hue']},
        {id: 'level', label: () => t('Brightness'), type: 'percent', params: ['LEVEL']},
        RAMP,
        DURATION,
        {id: 'effect', label: () => t('Effect'), type: 'choice', params: ['EFFECT'], optional: true},
    ],
    blind: [
        {id: 'level', label: () => t('Height'), type: 'percent', params: ['LEVEL']},
        {id: 'slats', label: () => t('Slats'), type: 'percent', params: ['LEVEL_2']},
    ],
};

/** The fields of a form on a channel: those whose datapoints the channel takes. */
export function formFields(kind: FormKind, params: Param[]): FormField[] {
    const out: FormField[] = [];
    for (const s of SPECS[kind]) {
        const defs = s.params.map((id) => writable(params, id));
        if (defs.some((d) => !d)) continue;
        out.push({id: s.id, label: s.label(), type: s.type, defs: defs as Param[], optional: !!s.optional, excludes: s.excludes ?? []});
    }
    return out;
}

// --- the entries of the devices' lists, in words ---

/** The entries the page has words for; any other is shown by its name. */
export const ENTRIES = [
    'BLACK', 'BLUE', 'GREEN', 'TURQUOISE', 'RED', 'PURPLE', 'YELLOW', 'WHITE', 'RANDOM', 'OLD_VALUE', 'DO_NOT_CARE',
    'OFF', 'ON', 'BLINKING_SLOW', 'BLINKING_MIDDLE', 'BLINKING_FAST', 'FLASH_SLOW', 'FLASH_MIDDLE', 'FLASH_FAST', 'BILLOW_SLOW', 'BILLOW_MIDDLE', 'BILLOW_FAST',
    'NO_REPETITION', 'INFINITE_REPETITIONS', 'INTERNAL_SOUNDFILE', 'RANDOM_SOUNDFILE', 'NO_EFFECT',
    'DISABLE_ACOUSTIC_SIGNAL', 'FREQUENCY_RISING', 'FREQUENCY_FALLING', 'FREQUENCY_RISING_AND_FALLING', 'FREQUENCY_ALTERNATING_LOW_HIGH', 'FREQUENCY_ALTERNATING_LOW_MID_HIGH',
    'FREQUENCY_HIGHON_OFF', 'FREQUENCY_HIGHON_LONGOFF', 'FREQUENCY_LOWON_OFF_HIGHON_OFF', 'FREQUENCY_LOWON_LONGOFF_HIGHON_LONGOFF',
    'LOW_BATTERY', 'DISARMED', 'INTERNALLY_ARMED', 'EXTERNALLY_ARMED', 'DELAYED_INTERNALLY_ARMED', 'DELAYED_EXTERNALLY_ARMED', 'EVENT', 'ERROR',
    'DISABLE_OPTICAL_SIGNAL', 'BLINKING_ALTERNATELY_REPEATING', 'BLINKING_BOTH_REPEATING', 'DOUBLE_FLASHING_REPEATING', 'FLASHING_BOTH_REPEATING',
    'CONFIRMATION_SIGNAL_0', 'CONFIRMATION_SIGNAL_1', 'CONFIRMATION_SIGNAL_2',
];

const UNITS: Record<string, string> = {S: 's', M: 'min', H: 'h', '10MS': '× 10 ms'};

/** An entry of a device's list in words: "RED" is "Red", "SOUNDFILE_012" is "Sound file 12"; one the page has no words for keeps its name. */
export function entryLabel(name: string): string {
    if (ENTRIES.includes(name)) return t('entry.' + name);
    let m = /^SOUNDFILE_(\d+)$/.exec(name);
    if (m) return t('Sound file {n}', {n: Number(m[1])});
    m = /^REPETITIONS_(\d+)$/.exec(name);
    if (m) return t('{n} repetitions', {n: Number(m[1])}, Number(m[1]));
    m = /^EFFECT_(\d+)_(END|INTERRUPT)_CURRENT_PROFILE$/.exec(name);
    if (m) return m[2] === 'END' ? t('Effect {n}, after the running one', {n: Number(m[1])}) : t('Effect {n}, at once', {n: Number(m[1])});
    return UNITS[name] ?? name;
}

/** The colours of a signal light as the page paints them; an entry that is no colour has none. */
export const SWATCHES: Record<string, string> = {
    BLACK: '#111111',
    BLUE: '#1e6fff',
    GREEN: '#1faa3c',
    TURQUOISE: '#19c2c2',
    RED: '#e0271f',
    PURPLE: '#a03cc8',
    YELLOW: '#f2c40f',
    WHITE: '#ffffff',
};

/** A hue (0 to 360) and a saturation (0 to 100) as a colour of full brightness, for the preview. */
export function hueColor(hue: number, saturation: number): string {
    const h = Number.isFinite(hue) ? ((hue % 360) + 360) % 360 : 0;
    const s = Number.isFinite(saturation) ? Math.min(100, Math.max(0, saturation)) : 0;
    return `hsl(${Math.round(h)} 100% ${Math.round(100 - s / 2)}%)`;
}

// --- between the stored values and the form ---

const share = (v: Value) => (typeof v === 'number' ? Math.round(v * 1000) / 10 : v);
const entry = (def: Param, v: Value) => (typeof v === 'number' && def.values?.[v] ? def.values[v] : v);
const within = (def: Param, n: number) => Math.min(def.max ?? n, Math.max(def.min ?? n, n));
const firstOf = (def: Param, ...wanted: string[]) => wanted.find((w) => def.values?.includes(w)) ?? def.values?.find((x) => x !== '') ?? 0;

function fresh(f: FormField): FieldState {
    const [d, e] = f.defs as [Param, Param | undefined];
    switch (f.type) {
        case 'swatch':
            return {on: true, a: firstOf(d, 'WHITE'), b: undefined};
        case 'choice':
            return {on: !f.optional, a: firstOf(d, 'ON', 'SOUNDFILE_001', 'FREQUENCY_RISING', 'BLINKING_ALTERNATELY_REPEATING'), b: undefined};
        case 'percent':
            return {on: true, a: 100, b: undefined};
        case 'number':
            return {on: false, a: within(d, 2700), b: undefined};
        case 'span':
            return {on: !f.optional, a: 10, b: firstOf(e!, 'S')};
        case 'hue':
            return {on: false, a: 0, b: 100};
    }
}

/** What the form shows for the stored values: every field's state, and the values no field has. */
export function formState(fields: FormField[], values: Stored[]): {state: Record<string, FieldState>; rest: Stored[]} {
    const state: Record<string, FieldState> = {};
    const used = new Set<string>();
    const get = (d: Param) => values.find((v) => v.param === d.id);
    for (const f of fields) {
        const have = f.defs.map(get);
        const s = fresh(f);
        if (have.every((h) => h)) {
            for (const d of f.defs) used.add(d.id);
            const [a, b] = have as [Stored, Stored | undefined];
            s.on = true;
            s.a = f.type === 'percent' ? share(a.value) : f.type === 'swatch' || f.type === 'choice' ? entry(f.defs[0]!, a.value) : a.value;
            if (b) s.b = f.type === 'hue' ? share(b.value) : entry(f.defs[1]!, b.value);
        }
        state[f.id] = s;
    }
    // a new activity on a coloured light starts with a colour
    if (values.length === 0 && state.hue) state.hue.on = true;
    return {state, rest: values.filter((v) => !used.has(v.param))};
}

/** The values the form's state stands for, in the fields' order, then the ones no field has. */
export function formValues(fields: FormField[], state: Record<string, FieldState>, rest: Stored[]): Stored[] {
    const out: Stored[] = [];
    const part = (v: Value) => (typeof v === 'number' ? Math.round(v * 10) / 1000 : v);
    for (const f of fields) {
        const s = state[f.id];
        if (!s?.on) continue;
        out.push({param: f.defs[0]!.id, value: f.type === 'percent' ? part(s.a) : s.a});
        if (f.defs[1]) out.push({param: f.defs[1].id, value: f.type === 'hue' ? part(s.b) : s.b});
    }
    const mine = new Set(out.map((v) => v.param));
    return [...out, ...rest.filter((v) => !mine.has(v.param))];
}

function numberProblem(v: Value, min: number, max: number, whole: boolean): string {
    if (typeof v !== 'number' || !Number.isFinite(v)) return t('Enter a number.');
    if (whole && !Number.isInteger(v)) return t('Enter a whole number.');
    return v < min || v > max ? t('outside {min} to {max}', {min, max}) : '';
}

/** What is wrong with the form as it is filled; the field it is about and the sentence, or null. */
export function formProblem(fields: FormField[], state: Record<string, FieldState>): {field: string; text: string} | null {
    for (const f of fields) {
        const s = state[f.id];
        if (!s?.on) continue;
        const other = f.excludes.find((id) => state[id]?.on);
        const d = f.defs[0]!;
        let text = '';
        if (other) text = t('Either a colour or a colour temperature, not both.');
        else if (f.type === 'percent') text = numberProblem(s.a, 0, 100, false);
        else if (f.type === 'number' || f.type === 'span') text = numberProblem(s.a, d.min ?? 0, d.max ?? Number.MAX_SAFE_INTEGER, d.type === 'INTEGER');
        else if (f.type === 'hue') text = numberProblem(s.a, 0, 360, true) || numberProblem(s.b, 0, 100, false);
        if (text) return {field: f.id, text: f.label + ': ' + text};
    }
    return null;
}

const plain = (v: Value) => (v === undefined || v === null ? '' : String(v));

/** The form's state in one line: "Red, brightness 50 %, blinking, for 10 min". */
export function formText(fields: FormField[], state: Record<string, FieldState>): string {
    const out: string[] = [];
    for (const f of fields) {
        const s = state[f.id];
        if (!s?.on) continue;
        switch (f.type) {
            case 'swatch':
            case 'choice':
                out.push(entryLabel(plain(s.a)));
                break;
            case 'percent':
                out.push(`${f.label} ${plain(s.a)} %`);
                break;
            case 'number':
                out.push(`${plain(s.a)} ${f.defs[0]!.unit ?? ''}`.trim());
                break;
            case 'span':
                out.push(`${f.label} ${plain(s.a)} ${entryLabel(plain(s.b))}`);
                break;
            case 'hue':
                out.push(t('hue {h}°, saturation {s} %', {h: plain(s.a), s: plain(s.b)}));
                break;
        }
    }
    return out.join(', ');
}

/** The values of a "channel" activity in the form's words; '' when the channel has no form or the values are not all the form's. */
export function formWords(type: string | undefined, params: Param[] | undefined, values: Stored[]): string {
    const kind = formKind(type, params);
    if (!kind || values.length === 0) return '';
    const fields = formFields(kind, params!);
    const {state, rest} = formState(fields, values);
    if (rest.length > 0 || fields.some((f) => !f.optional && !f.defs.every((d) => values.some((v) => v.param === d.id)))) return '';
    return formText(fields, state);
}

// --- the strings of COMBINED_PARAMETER ---

/** The short names a CCU's combined string uses, and the datapoints they stand for; share: a value of 0 to 100 for a datapoint of 0 to 1. */
const TOKENS: Record<string, {param: string; share?: boolean}> = {
    L: {param: 'LEVEL', share: true},
    L2: {param: 'LEVEL_2', share: true},
    C: {param: 'COLOR'},
    CB: {param: 'COLOR_BEHAVIOUR'},
    DV: {param: 'DURATION_VALUE'},
    DU: {param: 'DURATION_UNIT'},
    RTV: {param: 'RAMP_TIME_VALUE'},
    RTU: {param: 'RAMP_TIME_UNIT'},
    R: {param: 'REPETITIONS'},
    SL: {param: 'SOUNDFILE'},
    H: {param: 'HUE'},
    SAT: {param: 'SATURATION', share: true},
    TC: {param: 'COLOR_TEMPERATURE'},
    E: {param: 'EFFECT'},
};

/**
 * The values a COMBINED_PARAMETER string of a CCU program stands for ("L=50,C=4,DV=10,DU=1"),
 * as datapoints of the channel; undefined for a string that cannot be read whole: a short name
 * that is not known, a datapoint the channel does not take, a value outside what it takes.
 */
export function combinedValues(text: string, params: Param[] | undefined): Stored[] | undefined {
    if (!params || text.trim() === '') return undefined;
    const out: Stored[] = [];
    for (const piece of text.split(',')) {
        const m = /^\s*([A-Z0-9]+)\s*=\s*(-?\d+(?:\.\d+)?)\s*$/.exec(piece);
        const token = m && TOKENS[m[1]!];
        const def = token && writable(params, token.param);
        if (!m || !token || !def || out.some((v) => v.param === def.id)) return undefined;
        let n = Number(m[2]);
        if (def.type === 'ENUM') {
            const name = Number.isInteger(n) ? def.values?.[n] : undefined;
            if (!name) return undefined;
            out.push({param: def.id, value: name});
            continue;
        }
        if (token.share) n = Math.round(n * 10) / 1000;
        if ((def.type !== 'FLOAT' && def.type !== 'INTEGER') || (def.type === 'INTEGER' && !Number.isInteger(n))) return undefined;
        if ((def.min !== undefined && n < def.min) || (def.max !== undefined && n > def.max)) return undefined;
        out.push({param: def.id, value: n});
    }
    return out;
}
