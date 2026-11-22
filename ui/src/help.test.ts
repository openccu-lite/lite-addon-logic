import {readdirSync, readFileSync} from 'node:fs';
import {describe, expect, it} from 'vitest';

import {docUrl, TOPICS} from './help.js';

describe('help links', () => {
    it('point at the guide in the page\'s language', () => {
        expect(docUrl('de', 'variables')).toBe('docs/de/#variables');
        expect(docUrl('en', 'variables')).toBe('docs/en/#variables');
        expect(docUrl('fr', 'import')).toBe('docs/en/#import');
    });
    it('use only topics the list names, and the guide has each in both languages', () => {
        const dir = process.cwd() + '/src/';
        const used = new Set<string>();
        for (const f of readdirSync(dir).filter((f) => f.endsWith('.svelte'))) {
            for (const m of readFileSync(dir + f, 'utf8').matchAll(/<HelpLink topic="([^"]+)"/g)) used.add(m[1]!);
        }
        for (const u of used) expect(TOPICS as readonly string[]).toContain(u);
        for (const file of ['../docs/de/handbuch.md', '../docs/en/guide.md']) {
            const text = readFileSync(process.cwd() + '/' + file, 'utf8');
            for (const topic of TOPICS) expect(text, `${file} #${topic}`).toContain(`<a id="${topic}"></a>`);
        }
    });
});
