// The versions of a program's or a variable's file: the history's entries in the reader's
// language, and a readable comparison of two versions of the file, line by line.
import {t} from 'occulite-ui/i18n';

export interface Version {
    hash: string;
    time: string;
    author: string;
    message: string;
}

export interface VersionList {
    versions: Version[];
    /** The file as it is now. */
    current: string;
}

/** One line of a comparison: unchanged (' '), only in the newer ('+') or only in the older ('-') text; a gap ('…') stands for n unchanged lines left out. */
export interface DiffLine {
    kind: ' ' | '+' | '-' | '…';
    text: string;
    /** The line's number in the older and in the newer text (0: not there). */
    a: number;
    b: number;
    /** A gap: how many lines it stands for. */
    n?: number;
}

const lines = (text: string) => (text === '' ? [] : text.replace(/\n$/, '').split('\n'));

/** The lines of two texts compared: what stayed, what went, what came (a longest common subsequence; the files are small). */
export function lineDiff(older: string, newer: string): DiffLine[] {
    const a = lines(older);
    const b = lines(newer);
    // lcs[i][j]: the longest common run of a[i..] and b[j..]
    const lcs: number[][] = Array.from({length: a.length + 1}, () => new Array<number>(b.length + 1).fill(0));
    for (let i = a.length - 1; i >= 0; i--) {
        for (let j = b.length - 1; j >= 0; j--) {
            lcs[i]![j] = a[i] === b[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
        }
    }
    const out: DiffLine[] = [];
    let i = 0;
    let j = 0;
    while (i < a.length || j < b.length) {
        if (i < a.length && j < b.length && a[i] === b[j]) {
            out.push({kind: ' ', text: a[i]!, a: ++i, b: ++j});
        } else if (i < a.length && (j >= b.length || lcs[i + 1]![j]! >= lcs[i]![j + 1]!)) {
            out.push({kind: '-', text: a[i]!, a: ++i, b: 0});
        } else {
            out.push({kind: '+', text: b[j]!, a: 0, b: ++j});
        }
    }
    return out;
}

/** A comparison with the long unchanged stretches left out: around every change `context` lines stay, a gap says how many are left out. */
export function collapse(diff: DiffLine[], context = 3): DiffLine[] {
    const keep = new Array<boolean>(diff.length).fill(false);
    diff.forEach((l, i) => {
        if (l.kind === ' ') return;
        for (let k = Math.max(0, i - context); k <= Math.min(diff.length - 1, i + context); k++) keep[k] = true;
    });
    const out: DiffLine[] = [];
    for (let i = 0; i < diff.length; ) {
        if (keep[i]) {
            out.push(diff[i++]!);
            continue;
        }
        let n = 0;
        while (i < diff.length && !keep[i]) {
            i++;
            n++;
        }
        // a gap of one line is shown as that line: a mark would take the same room
        if (n === 1) out.push(diff[i - 1]!);
        else out.push({kind: '…', text: '', a: 0, b: 0, n});
    }
    return out;
}

/** How many lines came and went. */
export function diffStats(diff: DiffLine[]): {added: number; removed: number} {
    return {added: diff.filter((l) => l.kind === '+').length, removed: diff.filter((l) => l.kind === '-').length};
}

/** What a history entry says happened, in the reader's language: the addon writes its entries in English. */
export function messageText(message: string): string {
    let m = /^(Program|Variable) (.*) (created|changed|removed)$/.exec(message);
    if (m) return t(`${m[1]} {name} ${m[3]}`, {name: m[2]!});
    m = /^(Program|Variable) (.*) restored to its version ([0-9a-f]+)$/.exec(message);
    if (m) return t(`${m[1]} {name} restored to its version {version}`, {name: m[2]!, version: m[3]!});
    m = /^Device (\S+) replaced by (\S+)$/.exec(message);
    if (m) return t('Device {old} replaced by {new}', {old: m[1]!, new: m[2]!});
    return message;
}

/** A version's short name. */
export function shortHash(hash: string): string {
    return hash.slice(0, 7);
}
