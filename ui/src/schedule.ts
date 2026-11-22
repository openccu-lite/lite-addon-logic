// The schedule page's own logic: what is armed - the time modules and the delayed activities
// that are not due yet - as rows of one list, when each fires next and how far away that is.
import {t} from 'occulite-ui/i18n';

import {actionText, durationText, edgeText, lineText, matches, timeText, type Action, type Context, type Line, type TimeEdge, type TimeModule} from './model.js';

export interface Timer {
    program: string;
    line: number;
    time: TimeModule;
    armed: boolean;
    /** Why it is not armed: inactive, faulty, no-clock, no-location. */
    off?: string;
    active: boolean;
    next?: TimeEdge;
    last?: TimeEdge;
}

export interface PendingAction {
    id: number;
    due: string;
    set: string;
    program: string;
    rule: number;
    action: Action;
    by: string;
}

/** A condition line with "for" whose comparison holds: counting, or true since it reached its time. */
export interface Hold {
    program: string;
    line: number;
    of: Line;
    for: string;
    since: string;
    due: string;
    true: boolean;
    /** The program is inactive: the count goes on, its end runs nothing. */
    inactive?: boolean;
}

export interface Schedule {
    timers: Timer[];
    pending: PendingAction[];
    /** Absent in an answer of an older addon. */
    holds?: Hold[];
    /** The engine's time, with the system's own offset. */
    now: string;
}

export interface ScheduleRow {
    id: string;
    kind: 'time' | 'delay' | 'hold';
    what: string;
    programKey: string;
    program: string;
    /** A delay's target: what the activity writes or runs. */
    target: string;
    next?: string;
    /** What happens then, in a word: begin, end, repeat - nothing for a point and a delay. */
    nextNote: string;
    last?: string;
    lastNote: string;
    /** Why a time module is not armed, in words; '' when it is. */
    off: string;
    /** A range that is under way. */
    active: boolean;
    /** How much of the way to the next firing is gone, 0 to 1; null when that cannot be said. */
    progress: number | null;
    /** A delay's id, for "run now" and "cancel". */
    pending?: number;
}

const ms = (iso: string | undefined) => (iso ? Date.parse(iso) : NaN);

/** How far away an instant is, in words: "in 4 min", "11 min ago", "now". */
export function relative(now: string, at: string | undefined): string {
    const diff = Math.round((ms(at) - ms(now)) / 1000);
    if (Number.isNaN(diff)) return '';
    const s = Math.abs(diff);
    let text: string;
    if (s < 1) return t('now');
    if (s < 60) text = `${s} s`;
    else if (s < 3600) text = `${Math.floor(s / 60)} min`;
    else if (s < 86400) text = `${Math.floor(s / 3600)} h` + (Math.floor((s % 3600) / 60) ? ` ${Math.floor((s % 3600) / 60)} min` : '');
    else text = `${Math.floor(s / 86400)} d` + (Math.floor((s % 86400) / 3600) ? ` ${Math.floor((s % 86400) / 3600)} h` : '');
    return diff > 0 ? t('in {t}', {t: text}) : t('{t} ago', {t: text});
}

const offsetOf = (iso: string | undefined) => /([+-]\d\d:\d\d|Z)$/.exec(iso ?? '')?.[1] ?? '';

/**
 * The clock's offset at an instant when it is not the one the clock has now - a time on the
 * other side of a clock change: "UTC+2". Empty otherwise.
 */
export function offsetNote(at: string | undefined, now: string): string {
    const o = offsetOf(at);
    if (!o || o === offsetOf(now)) return '';
    if (o === 'Z') return 'UTC';
    const [h = '', m = ''] = o.slice(1).split(':');
    return 'UTC' + o[0] + String(Number(h)) + (m === '00' ? '' : ':' + m);
}

/** How much of the way from one instant to another is gone at a third, 0 to 1. */
export function progress(from: string | undefined, to: string | undefined, now: string): number | null {
    const a = ms(from);
    const b = ms(to);
    const n = ms(now);
    if (Number.isNaN(a) || Number.isNaN(b) || Number.isNaN(n) || b <= a || a < Date.parse('2000-01-01')) return null;
    return Math.min(1, Math.max(0, (n - a) / (b - a)));
}

const note = (edge: TimeEdge | undefined) => (edge && edge.kind !== 'point' ? edgeText(edge.kind) : '');

/** Why a time module is not armed, in words. */
export function offText(off: string | undefined): string {
    switch (off) {
        case 'inactive':
            return t('the program is inactive');
        case 'faulty':
            return t('the program is faulty');
        case 'no-clock':
            return t('the clock of the system is not set');
        case 'no-location':
            return t('the location of the system is not set');
    }
    return '';
}

/** The schedule as rows: what fires next first, what is not armed last; filtered and searched. */
export function scheduleRows(s: Schedule, ctx: Context, filter: string, query: string): ScheduleRow[] {
    const rows: ScheduleRow[] = [];
    if (filter === 'all' || filter === 'time') {
        for (const tm of s.timers) {
            rows.push({id: `t-${tm.program}-${tm.line}`, kind: 'time', what: timeText(tm.time), programKey: tm.program, program: ctx.programs[tm.program] ?? tm.program, target: '',
                next: tm.next?.at, nextNote: note(tm.next), last: tm.last?.at, lastNote: note(tm.last), off: offText(tm.off), active: tm.active,
                progress: progress(tm.last?.at, tm.next?.at, s.now)});
        }
    }
    if (filter === 'all' || filter === 'hold') {
        for (const h of s.holds ?? []) {
            const text = lineText(h.of, ctx);
            rows.push({id: `h-${h.program}-${h.line}`, kind: 'hold', what: (text.subject + ' ' + text.param).trim() + ' ' + text.comparison, programKey: h.program, program: ctx.programs[h.program] ?? h.program,
                target: '', next: h.true ? undefined : h.due, nextNote: '', last: h.true ? h.due : undefined, lastNote: '', off: h.inactive ? offText('inactive') : '', active: h.true,
                progress: h.true ? null : progress(h.since, h.due, s.now)});
        }
    }
    if (filter === 'all' || filter === 'delay') {
        for (const p of s.pending) {
            const text = actionText(p.action, ctx);
            rows.push({id: `p-${p.id}`, kind: 'delay', what: t('delay of {d}', {d: durationText(p.action.after)}), programKey: p.program, program: ctx.programs[p.program] ?? p.program,
                target: (text.subject + ' ' + text.param).trim(), next: p.due, nextNote: '', lastNote: '', off: '', active: false, progress: progress(p.set, p.due, s.now), pending: p.id});
        }
    }
    return rows
        .filter((r) => matches(query, r.what, r.program, r.target))
        .sort((a, b) => {
            if (!a.next !== !b.next) return a.next ? -1 : 1;
            if (a.next && b.next && ms(a.next) !== ms(b.next)) return ms(a.next) - ms(b.next);
            return a.program.localeCompare(b.program) || a.id.localeCompare(b.id);
        });
}
