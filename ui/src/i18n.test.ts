import {readdirSync, readFileSync} from 'node:fs';
import {describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';

const has = (key: string) => {
    const e = MESSAGES[key] as {de?: string; en?: string} | undefined;
    return !!e?.de && !!e?.en;
};

// every t('…') of the page has its German and English text
describe('messages', () => {
    it('covers every key of the page', () => {
        const dir = process.cwd() + '/src/';
        const missing: string[] = [];
        for (const f of readdirSync(dir).filter((f) => f.endsWith('.svelte') || (f.endsWith('.ts') && !f.endsWith('.test.ts')))) {
            const src = readFileSync(dir + f, 'utf8');
            for (const m of src.matchAll(/\bt\((?:'((?:[^'\\]|\\.)*)'|"((?:[^"\\]|\\.)*)")/g)) {
                const key = (m[1] ?? m[2]!).replace(/\\'/g, "'");
                // a dotted prefix is completed by a value: model.test.ts covers those
                if (/^[a-z]+\.$/.test(key)) continue;
                if (!has(key)) missing.push(`${f}: ${key}`);
            }
        }
        expect(missing).toEqual([]);
    });
    it('never says "box"', () => {
        for (const [k, v] of Object.entries(MESSAGES)) {
            const e = v as {de: string; en: string};
            expect(`${k} ${e.de} ${e.en}`).not.toMatch(/\bbox\b/i);
        }
    });
});
