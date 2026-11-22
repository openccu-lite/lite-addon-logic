import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import {collapse, diffStats, lineDiff, messageText, shortHash} from './versions.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const show = (older: string, newer: string, context?: number) => (context === undefined ? lineDiff(older, newer) : collapse(lineDiff(older, newer), context)).map((l) => (l.kind === '…' ? `… ${l.n}` : l.kind + l.text));

describe('two versions of a file compared', () => {
    it('line by line', () => {
        expect(show('name: A\nactive: true\nrules: []\n', 'name: A\nactive: false\nnote: x\nrules: []\n')).toEqual([' name: A', '-active: true', '+active: false', '+note: x', ' rules: []']);
        expect(show('a\nb\n', 'a\nb\n')).toEqual([' a', ' b']);
        expect(show('', 'a\nb\n')).toEqual(['+a', '+b']);
        expect(show('a\nb\n', '')).toEqual(['-a', '-b']);
        expect(show('', '')).toEqual([]);
        // a file without a line end at its end is the same lines
        expect(show('a\nb', 'a\nb\n')).toEqual([' a', ' b']);
        expect(show('a\nb\nc\n', 'b\nc\nd\n')).toEqual(['-a', ' b', ' c', '+d']);
    });
    it('with the line numbers of both sides', () => {
        expect(lineDiff('a\nb\n', 'a\nx\nb\n').map((l) => [l.a, l.b])).toEqual([[1, 1], [0, 2], [2, 3]]);
        expect(lineDiff('a\nb\n', 'b\n').map((l) => [l.a, l.b])).toEqual([[1, 0], [2, 1]]);
    });
    it('with the unchanged stretches left out', () => {
        const older = '1\n2\n3\n4\n5\n6\n7\n8\n9\n';
        expect(show(older, older.replace('5', 'five'), 1)).toEqual(['… 3', ' 4', '-5', '+five', ' 6', '… 3']);
        expect(show(older, older.replace('1\n', 'one\n').replace('9', 'nine'), 2)).toEqual(['-1', '+one', ' 2', ' 3', '… 3', ' 7', ' 8', '-9', '+nine']);
        // a gap of one line is that line
        expect(show('1\n2\n3\n4\n5\n', '1\n2\n3\n4\nx\n', 2)).toEqual(['… 2', ' 3', ' 4', '-5', '+x']);
        expect(show('1\n2\n3\n4\n5\n', '1\n2\n3\n4\nx\n', 1)).toEqual(['… 3', ' 4', '-5', '+x']);
        expect(show('1\n2\n3\n4\n', '1\n2\n3\nx\n', 1)).toEqual(['… 2', ' 3', '-4', '+x']);
        expect(show('1\n2\n3\n', '1\n2\nx\n', 1)).toEqual([' 1', ' 2', '-3', '+x']);
        // nothing changed: nothing but the gap
        expect(show(older, older, 3)).toEqual(['… 9']);
        expect(collapse(lineDiff('a\n', 'b\n')).map((l) => l.kind)).toEqual(['-', '+']);
    });
    it('counts what came and went', () => {
        expect(diffStats(lineDiff('a\nb\nc\n', 'a\nx\ny\nc\nz\n'))).toEqual({added: 3, removed: 1});
    });
});

describe('a history entry', () => {
    it('is said in the reader\'s language', () => {
        expect(messageText('Program Markise bei Regen changed')).toBe('Program Markise bei Regen changed');
        i18n.language = 'de';
        expect(messageText('Program Markise bei Regen changed')).toBe('Programm Markise bei Regen geändert');
        expect(messageText('Program A created')).toBe('Programm A angelegt');
        expect(messageText('Variable Regen removed')).toBe('Variable Regen entfernt');
        expect(messageText('Variable Regen restored to its version 07cc47f')).toBe('Variable Regen auf die Fassung 07cc47f zurückgesetzt');
        expect(messageText('Program A restored to its version 07cc47f')).toBe('Programm A auf die Fassung 07cc47f zurückgesetzt');
        // what the addon did not write itself stays as it is
        expect(messageText('von Hand geändert')).toBe('von Hand geändert');
        i18n.language = 'en';
        expect(shortHash('07cc47fdefcdd81bfd073a8b986b3e191b43e6cb')).toBe('07cc47f');
    });
});
