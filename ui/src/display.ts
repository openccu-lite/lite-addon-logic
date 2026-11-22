// The display composer's own logic: the five lines of a display channel (the HmIP-WRCD) with
// text, icon, alignment and colours, and an optional signal tone - as the form edits them, as
// the file stores them, what is wrong with them, how the preview shows them, and how a CCU
// program's finished text for such a channel is read back into lines.
import {t} from 'occulite-ui/i18n';

import type {Env} from './editor.js';
import type {Display, DisplayLine} from './model.js';

export const DISPLAY_LINES = 5;
/** How many characters of a line the display shows. */
export const DISPLAY_WIDTH = 16;
const MAX_STORED = 64;

/** The display's icons and signal tones, in the order of the device's own description. */
export const DISPLAY_ICONS = [
    'NO_ICON', 'LAMP_OFF', 'LAMP_ON', 'PADLOCK_OPEN', 'PADLOCK_CLOSED', 'ERROR', 'EVERYTHING_OKAY', 'INFORMATION', 'NEW_MESSAGE', 'SERVICE_MESSAGE',
    'SUN', 'MOON', 'WIND', 'CLOUD', 'THUNDERSTORM', 'DRIZZLE', 'CLOUD_AND_MOOON', 'RAIN', 'SNOW', 'CLOUD_AND_SUN', 'CLOUD_SUN_AND_RAIN', 'SNOWFLAKE', 'RAINDROP', 'FLAME',
    'WINDOW_OPEN', 'SHUTTERS', 'ECO', 'PROTECTION_DEACTIVATED', 'EXTERNAL_PROTECTION', 'INTERNAL_PROTECTION', 'BELL', 'CLOCK',
];
export const DISPLAY_SOUNDS = ['LOW_BATTERY', 'DISARMED', 'INTERNALLY_ARMED', 'EXTERNALLY_ARMED', 'DELAYED_INTERNALLY_ARMED', 'DELAYED_EXTERNALLY_ARMED', 'EVENT', 'ERROR'];
export const DISPLAY_INTERVALS = [5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 75, 80];

/** What the preview draws for an icon: a sign that comes close, not the device's own picture. */
export const ICON_GLYPHS: Record<string, string> = {
    LAMP_OFF: '○', LAMP_ON: '💡', PADLOCK_OPEN: '🔓', PADLOCK_CLOSED: '🔒', ERROR: '✖', EVERYTHING_OKAY: '✔', INFORMATION: 'ℹ', NEW_MESSAGE: '✉', SERVICE_MESSAGE: '🔧',
    SUN: '☀', MOON: '☾', WIND: '≋', CLOUD: '☁', THUNDERSTORM: '⛈', DRIZZLE: '🌦', CLOUD_AND_MOOON: '☁☾', RAIN: '🌧', SNOW: '🌨', CLOUD_AND_SUN: '⛅', CLOUD_SUN_AND_RAIN: '🌦',
    SNOWFLAKE: '❄', RAINDROP: '💧', FLAME: '🔥', WINDOW_OPEN: '🪟', SHUTTERS: '▤', ECO: '🍃', PROTECTION_DEACTIVATED: '⛨', EXTERNAL_PROTECTION: '🛡', INTERNAL_PROTECTION: '🏠', BELL: '🔔', CLOCK: '🕒',
};

/** A line as the form has it: every field set, the defaults spelled out. */
export interface LineForm {
    text: string;
    /** '' for none. */
    icon: string;
    align: string;
    color: string;
    background: string;
}

export interface DisplayForm {
    /** Always five. */
    lines: LineForm[];
    /** '' for none. */
    sound: string;
    repetitions: number;
    interval: number;
}

const newLine = (): LineForm => ({text: '', icon: '', align: 'CENTER', color: 'BLACK', background: 'WHITE'});

export function displayForm(d: Display | undefined): DisplayForm {
    const lines = Array.from({length: DISPLAY_LINES}, (_, i) => {
        const l = d?.lines?.[i];
        return l ? {text: l.text ?? '', icon: l.icon ?? '', align: l.align || 'CENTER', color: l.color || 'BLACK', background: l.background || 'WHITE'} : newLine();
    });
    return {lines, sound: d?.sound ?? '', repetitions: d?.repetitions ?? 0, interval: d?.interval || 5};
}

const bare = (l: LineForm) => l.text === '' && l.icon === '';

/** The display as the file stores it: the lines up to the last with something in it, what is the default left out. */
export function displayOf(f: DisplayForm): Display {
    let last = f.lines.length;
    while (last > 0 && bare(f.lines[last - 1]!)) last--;
    const out: Display = {};
    const lines = f.lines.slice(0, last).map((l) => {
        const line: DisplayLine = {};
        if (bare(l)) return line;
        if (l.text) line.text = l.text;
        if (l.icon) line.icon = l.icon;
        if (l.align !== 'CENTER') line.align = l.align;
        if (l.color !== 'BLACK') line.color = l.color;
        if (l.background !== 'WHITE') line.background = l.background;
        return line;
    });
    if (lines.length > 0) out.lines = lines;
    if (f.sound) {
        out.sound = f.sound;
        if (f.repetitions) out.repetitions = f.repetitions;
        if (f.interval && f.interval !== 5) out.interval = f.interval;
    }
    return out;
}

const PLACEHOLDER = /\{(program|trigger|value|time|var:[a-z0-9-]+|secret:[a-z0-9-]+)\}/g;
/** What a display shows besides plain ASCII, and the characters that stand for it in the text the device takes. */
const SPECIAL: Record<string, string> = {Ä: '[', Ö: '#', Ü: '$', ä: '²', ö: '|', ü: '³', ß: '_', '&': ']', "'": 'µ'};
const shows = (c: string) => c in SPECIAL || (c >= ' ' && c <= '~' && !',={}'.includes(c));

/** What is wrong with a display activity; '' when it can be saved. */
export function displayProblem(channel: string, f: DisplayForm, env: Env): string {
    if (!channel) return t('Choose a device.');
    const known = env.params[channel];
    if (known && !known.some((p) => p.id === 'COMBINED_PARAMETER' && p.write)) return t('This channel is no display.');
    for (const [i, l] of f.lines.entries()) {
        const n = i + 1;
        const names = [...l.text.matchAll(PLACEHOLDER)].map((m) => m[1]!);
        if (names.some((p) => p.startsWith('secret:'))) return t('Line {n}: a secret is not shown on a display.', {n});
        const missing = names.find((p) => p.startsWith('var:') && !env.ctx.variables[p.slice(4)]);
        if (missing) return t('Line {n}: the variable {key} does not exist.', {n, key: missing.slice(4)});
        const rest = l.text.replace(PLACEHOLDER, '');
        const odd = [...rest].find((c) => !shows(c));
        if (odd) return t('Line {n}: the display cannot show "{c}".', {n, c: odd});
        if ([...rest].length > DISPLAY_WIDTH || [...l.text].length > MAX_STORED) return t('Line {n}: the display shows {max} characters of a line.', {n, max: DISPLAY_WIDTH});
    }
    if (f.lines.every(bare) && !f.sound) return t('Enter a text, choose an icon or a signal tone.');
    return '';
}

/** A line's text as the preview shows it: a placeholder as its name in angle marks, cut to the display's width. */
export function previewText(text: string): string {
    const shown = text.replace(PLACEHOLDER, (_, name: string) => '‹' + name.replace(/^var:/, '') + '›');
    return [...shown].slice(0, DISPLAY_WIDTH).join('');
}

/** A display activity in one line: its lines with a bar between them, then the signal tone. */
export function displayText(d: Display | undefined): string {
    const parts = (d?.lines ?? []).map((l) => [l.icon ? t('icon.' + l.icon) : '', l.text ?? ''].filter(Boolean).join(' ') || '–');
    const lines = parts.join(' | ');
    return [lines, d?.sound ? t('signal tone: {name}', {name: t('entry.' + d.sound)}) : ''].filter(Boolean).join(' · ');
}

/**
 * The lines and the signal tone a CCU program's finished text for a display channel stands for
 * ("{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=Text,DDID=1,DDC=true},{R=1,IN=10,ANS=6}");
 * undefined for a text that cannot be read whole.
 */
export function combinedDisplay(text: string): Display | undefined {
    const s = text.trim();
    if (!s.startsWith('{') || !s.endsWith('}')) return undefined;
    const form = displayForm(undefined);
    const seen = new Set<number>();
    const back = Object.fromEntries(Object.entries(SPECIAL).map(([shown, sent]) => [sent, shown]));
    for (const block of s.slice(1, -1).split('},{')) {
        const fields: Record<string, string> = {};
        for (const piece of block.split(',')) {
            const at = piece.indexOf('=');
            if (at < 1 || piece.slice(0, at) in fields) return undefined;
            fields[piece.slice(0, at)] = piece.slice(at + 1);
        }
        const keys = Object.keys(fields);
        if ('ANS' in fields) {
            const sound = DISPLAY_SOUNDS[Number(fields.ANS)];
            const repetitions = Number(fields.R ?? '0');
            const interval = Number(fields.IN ?? '5');
            if (!/^\d+$/.test(fields.ANS!) || !sound || form.sound || keys.some((k) => !['ANS', 'R', 'IN'].includes(k))) return undefined;
            if (!Number.isInteger(repetitions) || repetitions < 0 || repetitions > 15 || !DISPLAY_INTERVALS.includes(interval)) return undefined;
            Object.assign(form, {sound, repetitions, interval});
            continue;
        }
        // the CCU's "reset the display" is no line
        if (fields.DDS === 'XXX' && fields.DDBC === undefined) return undefined;
        const n = Number(fields.DDID);
        if (!Number.isInteger(n) || n < 1 || n > DISPLAY_LINES || seen.has(n) || keys.some((k) => !['DDBC', 'DDTC', 'DDI', 'DDA', 'DDS', 'DDID', 'DDC'].includes(k))) return undefined;
        seen.add(n);
        const icon = fields.DDI === undefined || fields.DDI === '0' ? '' : /^\d+$/.test(fields.DDI) ? DISPLAY_ICONS[Number(fields.DDI)] : undefined;
        const line: LineForm = {text: [...(fields.DDS ?? '')].map((c) => back[c] ?? c).join(''), icon: icon ?? '', align: fields.DDA ?? 'CENTER', color: fields.DDTC ?? 'BLACK', background: fields.DDBC ?? 'WHITE'};
        if (icon === undefined || !['LEFT', 'CENTER', 'RIGHT'].includes(line.align) || ![line.color, line.background].every((c) => c === 'BLACK' || c === 'WHITE')) return undefined;
        if ([...line.text].some((c) => !shows(c)) || [...line.text].length > DISPLAY_WIDTH || bare(line)) return undefined;
        form.lines[n - 1] = line;
    }
    return displayOf(form);
}
