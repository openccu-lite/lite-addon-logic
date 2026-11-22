// The triggers page's own logic: what triggers what. Every source some program looks at - a
// device datapoint, a system variable, a time module - with the programs that listen to it, how
// each listens, and the source's last event.
import {t} from 'occulite-ui/i18n';

import {channelName, matches, refText, timeText, triggerLabel, valueText, variableName, type Context, type Overview, type Value} from './model.js';

export interface DeviceState {
    value: Value;
    /** The last report since the addon started. */
    time?: string;
    /** How often the datapoint reports, on average. */
    every_seconds?: number;
}

export interface Sources {
    devices: Record<string, DeviceState>;
    now: string;
}

export interface Listener {
    programKey: string;
    program: string;
    /** The line's mode in the CCU's words. */
    label: string;
    /** The line triggers the program (a "check only" line is only looked at). */
    triggers: boolean;
    /** The program is active. */
    active: boolean;
    /** What this way of listening means, where it surprises. */
    warn: string;
}

export interface SourceGroup {
    id: string;
    kind: 'variable' | 'device' | 'time';
    title: string;
    /** A device's datapoint and address. */
    detail: string;
    /** The last event (ISO) and the value it brought, in words; '' where there is none. */
    last: string;
    value: string;
    /** "reports about every 3 min", once that is known. */
    every: string;
    listeners: Listener[];
}

export interface TriggerFilter {
    query: string;
    /** '' for all kinds. */
    kind: string;
    /** Only this program's sources and lines; '' for all programs. */
    program: string;
}

/** A length in seconds, roughly: "45 s", "3 min", "2 h". */
export function roughly(seconds: number): string {
    if (seconds < 120) return `${seconds} s`;
    if (seconds < 7200) return `${Math.round(seconds / 60)} min`;
    return `${Math.round(seconds / 3600)} h`;
}

const ORDER = {variable: 0, device: 1, time: 2};

/** The sources with their listeners: variables, then devices, then time modules, each by name. */
export function triggerGroups(o: Overview, src: Sources | null, ctx: Context, f: TriggerFilter): SourceGroup[] {
    const groups = new Map<string, SourceGroup>();
    const values = new Map(o.variables.map((v) => [v.def.key, v]));
    for (const p of o.programs) {
        if (f.program && p.program.key !== f.program) continue;
        let n = 0;
        for (const rule of p.program.rules) {
            for (const g of rule.when?.groups ?? []) {
                for (const line of g.lines) {
                    n++;
                    let group: SourceGroup;
                    if (line.time) {
                        group = {id: `t:${p.program.key}:${n}`, kind: 'time', title: timeText(line.time), detail: '', last: '', value: '', every: '', listeners: []};
                    } else if (line.device) {
                        const ref = refText(line.device);
                        const st = src?.devices[ref];
                        group = {id: 'd:' + ref, kind: 'device', title: channelName(line.device, ctx), detail: `${line.device.param} · ${line.device.address}`, last: st?.time ?? '',
                            value: st ? valueText(undefined, st.value) : '', every: st?.every_seconds ? t('reports about every {d}', {d: roughly(st.every_seconds)}) : '', listeners: []};
                    } else {
                        const key = line.variable ?? '';
                        const view = values.get(key);
                        group = {id: 'v:' + key, kind: 'variable', title: view ? variableName(view.def) : key, detail: '', last: view?.time ?? '', value: view ? valueText(view.def, view.value) : '',
                            every: '', listeners: []};
                    }
                    const have = groups.get(group.id) ?? group;
                    groups.set(group.id, have);
                    const label = triggerLabel(line.trigger, !!line.time);
                    if (have.listeners.some((l) => l.programKey === p.program.key && l.label === label)) continue;
                    have.listeners.push({programKey: p.program.key, program: p.program.name, label, triggers: line.trigger !== 'check' || !!line.time, active: p.program.active,
                        warn: line.device && line.trigger === 'update' ? t('runs at every report') : ''});
                }
            }
        }
    }
    const out = [...groups.values()].filter((g) => (!f.kind || g.kind === f.kind) && matches(f.query, g.title, g.detail, ...g.listeners.map((l) => l.program)));
    for (const g of out) g.listeners.sort((a, b) => Number(b.triggers) - Number(a.triggers) || a.program.localeCompare(b.program));
    return out.sort((a, b) => ORDER[a.kind] - ORDER[b.kind] || a.title.localeCompare(b.title) || a.id.localeCompare(b.id));
}

/** The kind of a source in a word. */
export function kindText(kind: string): string {
    return kind === 'variable' ? t('variable') : kind === 'device' ? t('device') : t('time module');
}
