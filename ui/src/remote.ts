// The history's remote on the Settings tab: what the addon answers about it, and the words for
// its state, for how the two histories relate and for what a fetch would change.
import {t} from 'occulite-ui/i18n';

import {wallTime} from './model.js';
import {messageText, type Version} from './versions.js';

export interface RemoteInfo {
    url: string;
    branch: string;
    username: string;
    /** 'https', 'ssh' or '' when there is no remote. */
    kind: string;
    has_token: boolean;
    public_key?: string;
    host_key?: string;
    pending: boolean;
    last_push?: string;
    error?: string;
    error_code?: string;
}

export interface RemoteChange {
    path: string;
    kind: 'added' | 'changed' | 'removed';
    what?: 'program' | 'variable' | 'settings';
    name?: string;
    problem?: string;
}

export interface RemotePreview {
    relation: 'empty' | 'same' | 'ahead' | 'behind' | 'new' | 'diverged';
    local: string;
    remote: string;
    entries: Version[];
    more?: boolean;
    changes: RemoteChange[];
    problems: string[];
    can_apply: boolean;
}

export const NO_REMOTE: RemoteInfo = {url: '', branch: '', username: '', kind: '', has_token: false, pending: false};

/** The transport an address would use, as the addon reads it: 'ssh', 'https' or '' when it is neither. */
export function addressKind(url: string): '' | 'https' | 'ssh' {
    const u = url.trim();
    if (/^https:\/\/[^/\s@]+\/\S+$/.test(u) || /^http:\/\/(localhost|127\.0\.0\.1|\[::1\])(:\d+)?\/\S+$/.test(u)) return 'https';
    if (/^ssh:\/\/([^/\s@]+@)?[^/\s@]+\/\S+$/.test(u) || /^[^/\s@:]+@[^/\s@:]+:\S+$/.test(u)) return 'ssh';
    return '';
}

/** What is wrong with the form before it is sent ('' when nothing is). */
export function remoteProblem(url: string, branch: string): string {
    const u = url.trim();
    if (u === '') return t('Enter the address of the repository.');
    if (/^http:\/\//.test(u) && addressKind(u) === '') return t('http:// would send the token unprotected: use https:// or ssh.');
    if (addressKind(u) === '') return t('The address must be https://…, ssh://… or user@host:path.');
    if (branch.trim() !== '' && !/^[^\s~^:?*[\\-][^\s~^:?*[\\]*$/.test(branch.trim())) return t('That cannot be a branch\'s name.');
    return '';
}

/** The words for what the last push or fetch answered. */
export function remoteErrorText(info: RemoteInfo): string {
    switch (info.error_code) {
        case 'auth':
            return info.kind === 'ssh' ? t('The remote does not accept the key: enter the public key below as a deploy key with write access.') : t('The remote does not accept the token.');
        case 'not-found':
            return t('The remote has no such repository, or the token or key may not see it.');
        case 'host-key':
            return t('The host presents another key than the one remembered. If the host was set up anew, forget its key.');
        case 'rejected':
            return t('The remote did not take the push: it has entries this system does not have, or it protects the branch. Fetch to see.');
        case 'unreachable':
            return t('The remote cannot be reached.');
        default:
            return info.error ?? '';
    }
}

/** The state line: one sentence about where the push stands. */
export function remoteState(info: RemoteInfo, language: string): {text: string; bad: boolean} {
    if (info.kind === '') return {text: t('No remote is set: the history stays on this system.'), bad: false};
    if (info.error) return {text: remoteErrorText(info), bad: true};
    if (info.pending) return {text: t('A push is under way.'), bad: false};
    if (info.last_push) return {text: t('In step with the remote; last pushed {time}.', {time: wallTime(info.last_push, language)}), bad: false};
    return {text: t('In step with the remote.'), bad: false};
}

/** How the two histories relate, in a sentence. */
export function relationText(p: RemotePreview): string {
    switch (p.relation) {
        case 'empty':
            return t('The remote has no entries on this branch yet: the next push creates it.');
        case 'same':
            return t('The remote and this system have the same history.');
        case 'ahead':
            return t('This system has entries the remote lacks: a push sends them.');
        case 'behind':
            return t('The remote has {n} entries this system lacks.', {n: p.more ? `${p.entries.length}+` : p.entries.length}, p.entries.length);
        case 'new':
            return t('This system has no history yet: the remote\'s can be taken over as it is.');
        default:
            return t('The remote and this system have each gone their own way: nothing can be taken over. Bring the two together with git elsewhere, then fetch again.');
    }
}

/** A change's kind, for its pill. */
export function changeKind(c: RemoteChange): {label: string; kind: string} {
    if (c.problem) return {label: t('not possible'), kind: 'bad'};
    switch (c.kind) {
        case 'added':
            return {label: t('new'), kind: 'ok'};
        case 'removed':
            return {label: t('removed'), kind: 'warn'};
        default:
            return {label: t('changed'), kind: ''};
    }
}

/** What a change is about: the program, the variable, the settings, or the file. */
export function changeText(c: RemoteChange): string {
    switch (c.what) {
        case 'program':
            return t('Program {name}', {name: c.name || c.path});
        case 'variable':
            return t('System variable {name}', {name: c.name || c.path});
        case 'settings':
            return t('The settings');
        default:
            return c.path;
    }
}

/** A fetched entry in a line: when, who, what. */
export function entryText(v: Version, language: string): string {
    return `${wallTime(v.time, language)} · ${v.author} · ${messageText(v.message)}`;
}
