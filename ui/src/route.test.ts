import {describe, expect, it} from 'vitest';

import {hashFor, tabFromHash} from './route.js';

const TABS = ['programs', 'variables', 'import', 'settings'];

describe('tabFromHash', () => {
    it.each([
        ['#/import', 'import'],
        ['#import', 'import'],
        ['#/import/', 'import'],
        ['#/import?x=1', 'import'],
        ['#/settings', 'settings'],
        ['#/nothing', null],
        ['#/', null],
        ['', null],
        ['#/IMPORT', null],
    ])('%s -> %s', (hash, want) => {
        expect(tabFromHash(hash, TABS)).toBe(want);
    });
    it('round-trips', () => {
        for (const t of TABS) expect(tabFromHash(hashFor(t), TABS)).toBe(t);
    });
});
