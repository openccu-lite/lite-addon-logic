// Device helpers: what the page knows of them - the ones there are, the channels that could
// have one, and the names a new helper's variables get.
import {t} from 'occulite-ui/i18n';

/** A variable's place in a device helper. */
export interface Helper {
    kind: string;
    channel: string;
    /** total, today or yesterday. */
    role: string;
    /** Said on the helper's leading variable: the helper does not count. */
    off?: boolean;
}

/** A device helper as the API lists it. */
export interface HelperView {
    kind: string;
    channel: string;
    /** The helper's variables' keys by role. */
    variables: Record<string, string>;
    off?: boolean;
    /** The channel's name on this system. */
    name?: string;
}

/** A channel that can have a helper of a kind and has none. */
export interface Candidate {
    kind: string;
    channel: string;
    name?: string;
    /** The device's type. */
    type: string;
    unit?: string;
    roles: string[];
}

export interface HelperList {
    helpers: HelperView[];
    candidates: Candidate[];
    /** Whether the system's devices could be asked for the candidates. */
    system: boolean;
}

/** The kinds of device helper, as the addon's model names them. */
export const HELPER_KINDS = ['energy', 'feed-in', 'esi', 'energy-hm', 'energy-wm', 'gas-wm', 'iec-wm', 'rain', 'sunshine', 'water', 'rain-hm'];

/** The roles in the order they are shown. */
export const ROLES = ['total', 'today', 'yesterday'];

/** What a kind counts, in a word: "Energy", "Rain". */
export const kindNoun = (kind: string): string => t('helpernoun.' + kind);

/** The role of a kind's leading variable: the total, or today's amount where the kind has no total. */
export const leadRole = (roles: string[]): string => roles[0] ?? 'total';

/** A candidate in the list to choose from: "Waschmaschine Messung - Energy (HmIP-PSM)". */
export function candidateLabel(c: Candidate): string {
    const address = c.channel.split('/')[1] ?? c.channel;
    return `${c.name || address} – ${kindNoun(c.kind)} (${c.type})`;
}

/** The key a candidate is chosen by. */
export const candidateId = (c: Candidate): string => c.kind + '@' + c.channel;

/** The name a candidate's helper is called by unless the user says otherwise: the channel's, or its address. */
export function baseName(c: Candidate): string {
    return c.name || (c.channel.split('/')[1] ?? c.channel);
}

/**
 * The names of a new helper's variables, by role: "Energiezähler Waschmaschine", "Energie
 * heute Waschmaschine", "Energie gestern Waschmaschine". Without the day's amounts only the
 * leading variable is made.
 */
export function helperNames(c: Candidate, base: string, days: boolean): Record<string, string> {
    const name = base.trim();
    const out: Record<string, string> = {};
    const lead = leadRole(c.roles);
    for (const role of ROLES) {
        if (!c.roles.includes(role) || (!days && role !== lead)) continue;
        if (role === 'total') out[role] = t('helpertotal.' + c.kind, {name});
        else out[role] = t(role === 'today' ? '{noun} today {name}' : '{noun} yesterday {name}', {noun: kindNoun(c.kind), name});
    }
    return out;
}

/** Whether a kind has day's amounts beside its leading variable. */
export const hasDays = (c: Candidate): boolean => c.roles.some((r) => r !== leadRole(c.roles));

/** What is wrong with the dialog's input; empty when the helper can be made. */
export function helperProblem(c: Candidate | undefined, base: string): string {
    if (!c) return t('Choose a device.');
    if (base.trim() === '') return t('The helper needs a name.');
    if (Object.values(helperNames(c, base, true)).some((n) => n.length > 120)) return t('The name is too long.');
    return '';
}

/** A helper's variable in words, for its mark's tooltip: "Kept by a device helper (Energy, total) on Waschmaschine Messung." */
export function helperTitle(h: Helper, channelName: string): string {
    const text = t('Kept by a device helper: {noun}, {role}, on {channel}.', {noun: kindNoun(h.kind), role: t('role.' + h.role), channel: channelName});
    if (h.role === 'total') return text + ' ' + t('Setting its value sets the meter reading; counting goes on from there.');
    return text + ' ' + t('Nobody else writes it.');
}
