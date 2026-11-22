// "Gerät ersetzen" on the Settings tab: the devices programs and variables name, the devices
// that can take the place of one, and the words for what a replacement would do.
import {t} from 'occulite-ui/i18n';

export interface DeviceId {
    interface: string;
    address: string;
}

export interface Named {
    key: string;
    name: string;
}

/** A device programs or variables name. */
export interface UsedDevice extends DeviceId {
    name?: string;
    type?: string;
    /** The system has it. */
    present: boolean;
    datapoints: string[];
    programs: Named[];
    variables: Named[];
}

/** A device of the system, as the editor's picker lists it. */
export interface SystemDevice extends DeviceId {
    name?: string;
    type: string;
}

export interface Replacement {
    programs: Named[];
    variables: Named[];
    references: number;
    problems: string[];
    can_apply: boolean;
}

/** A device's id as one text, for a select's value. */
export function idOf(d: DeviceId): string {
    return `${d.interface}/${d.address}`;
}

/** A device in one line: its name where it has one, its address, its type. */
export function deviceLabel(d: DeviceId & {name?: string; type?: string}): string {
    const parts = [d.name ? `${d.name} (${idOf(d)})` : idOf(d)];
    if (d.type) parts.push(d.type);
    return parts.join(' · ');
}

/** A used device in one line: the device, whether it is missing, how many objects name it. */
export function usedLabel(d: UsedDevice, system: boolean): string {
    const parts = [deviceLabel(d)];
    if (system && !d.present) parts.push(t('not on this system'));
    const n = d.programs.length + d.variables.length;
    parts.push(t('named by {n} programs and variables', {n}, n));
    return parts.join(' · ');
}

/**
 * The devices that can take the place of a used one: every other device of the system of its
 * type - or, when the type of a device that is not here is not known, every other device.
 */
export function candidates(used: UsedDevice | undefined, devices: SystemDevice[]): SystemDevice[] {
    if (!used) return [];
    return devices
        .filter((d) => idOf(d) !== idOf(used) && (!used.type || d.type === used.type))
        .sort((a, b) => deviceLabel(a).localeCompare(deviceLabel(b)));
}

/** What a replacement would change, in a sentence. */
export function replacementText(r: Replacement): string {
    const names = [...r.programs, ...r.variables].map((n) => n.name || n.key).join(', ');
    return t('{n} places would name the new device, in: {names}', {n: r.references, names}, r.references);
}

/** What a replacement changed, in a sentence. */
export function replacedText(r: Replacement): string {
    return t('{n} places now name the new device.', {n: r.references}, r.references);
}
