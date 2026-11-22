import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {combinedDisplay, DISPLAY_ICONS, DISPLAY_SOUNDS, displayForm, displayOf, displayProblem, displayText, ICON_GLYPHS, previewText} from './display.js';
import {draftOf, draftProblems, programOf, type Env} from './editor.js';
import {MESSAGES} from './messages.js';
import {actionText, type Context, type Program} from './model.js';
import {runView} from './trace.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

const CH = 'HmIP-RF/W:3';
const ctx: Context = {variables: {fenster: {key: 'fenster', name: 'Fenster', type: 'string'}}, programs: {}, names: {[CH]: 'Wandtaster'}} as unknown as Context;
const env: Env = {ctx, params: {[CH]: [{id: 'COMBINED_PARAMETER', type: 'STRING', read: false, write: true}], 'HmIP-RF/W:1': [{id: 'PRESS_SHORT', type: 'ACTION', read: true, write: false}]}};

describe('the display as the form and as the file', () => {
    it('spells the defaults out in the form and leaves them out of the file', () => {
        const form = displayForm(undefined);
        expect(form.lines).toHaveLength(5);
        expect(form).toMatchObject({sound: '', repetitions: 0, interval: 5});
        expect(form.lines[0]).toEqual({text: '', icon: '', align: 'CENTER', color: 'BLACK', background: 'WHITE'});
        expect(displayOf(form)).toEqual({});
        const stored = {lines: [{text: 'Fenster {var:fenster}', icon: 'WINDOW_OPEN', align: 'LEFT', color: 'WHITE', background: 'BLACK'}, {}, {icon: 'BELL'}], sound: 'EVENT', repetitions: 2, interval: 10};
        const read = displayForm(stored);
        expect(read.lines[1]).toEqual({text: '', icon: '', align: 'CENTER', color: 'BLACK', background: 'WHITE'});
        expect(read.lines[2]).toMatchObject({icon: 'BELL', text: ''});
        expect(displayOf(read)).toEqual(stored);
        // what an empty line carries is not stored; a sound's defaults neither; without a sound its numbers go
        read.lines[1]!.align = 'RIGHT';
        read.lines[4]!.color = 'WHITE';
        expect(displayOf(read)).toEqual(stored);
        expect(displayOf({...read, repetitions: 0, interval: 5})).toEqual({lines: stored.lines, sound: 'EVENT'});
        expect(displayOf({...read, sound: ''})).toEqual({lines: stored.lines});
        expect(displayForm({sound: 'ERROR'})).toMatchObject({sound: 'ERROR', repetitions: 0, interval: 5});
    });
    it('says what is wrong', () => {
        const form = displayForm({lines: [{text: 'Hallo'}]});
        expect(displayProblem('', form, env)).toBe('Choose a device.');
        expect(displayProblem(CH, form, env)).toBe('');
        expect(displayProblem('HmIP-RF/NEW:3', form, env)).toBe('');
        expect(displayProblem('HmIP-RF/W:1', form, env)).toBe('This channel is no display.');
        expect(displayProblem(CH, displayForm(undefined), env)).toBe('Enter a text, choose an icon or a signal tone.');
        expect(displayProblem(CH, displayForm({sound: 'EVENT'}), env)).toBe('');
        expect(displayProblem(CH, displayForm({lines: [{}, {icon: 'BELL'}]}), env)).toBe('');
        const line = (text: string) => displayProblem(CH, displayForm({lines: [{text: 'a'}, {text}]}), env);
        expect(line('Tür & Tor ä ß')).toBe('');
        expect(line('1234567890123456')).toBe('');
        expect(line('12345678901234567')).toBe('Line 2: the display shows 16 characters of a line.');
        expect(line('{value}{value}{value}{value}{value}{value}{value}{value}{value}{value}')).toBe('Line 2: the display shows 16 characters of a line.');
        expect(line('{var:fenster} {value} {time}')).toBe('');
        expect(line('{var:nope}')).toBe('Line 2: the variable nope does not exist.');
        expect(line('{secret:pin}')).toBe('Line 2: a secret is not shown on a display.');
        expect(line('a, b')).toBe('Line 2: the display cannot show ",".');
        expect(line('{wert}')).toBe('Line 2: the display cannot show "{".');
        expect(line('5 €')).toBe('Line 2: the display cannot show "€".');
        // in a draft
        const p: Program = {key: 'p', name: 'P', active: true, visible: true, operable: true, rules: [{when: {op: 'all', groups: []}, then: [{kind: 'display', channel: CH, display: {lines: [{text: 'a=b'}]}}]}]} as unknown as Program;
        const draft = draftOf(p);
        expect(Object.values(draftProblems(draft, env))).toEqual(['Line 1: the display cannot show "=".']);
        draft.rules[0]!.then[0]!.display.lines[0]!.text = 'ab';
        expect(draftProblems(draft, env)).toEqual({});
        expect(programOf(draft).rules[0]!.then[0]).toEqual({kind: 'display', channel: CH, display: {lines: [{text: 'ab'}]}});
    });
    it('says it in words', () => {
        expect(displayText(undefined)).toBe('');
        expect(displayText({lines: [{text: 'Fenster', icon: 'WINDOW_OPEN'}, {}, {icon: 'BELL'}], sound: 'EVENT'})).toBe('Window open Fenster | – | Bell · signal tone: Event');
        expect(displayText({sound: 'ERROR'})).toBe('signal tone: Error');
        expect(actionText({kind: 'display', channel: CH, display: {lines: [{text: 'Hallo'}]}}, ctx)).toMatchObject({kind: 'Display', subject: 'Wandtaster', unknown: false, what: 'Hallo'});
        expect(actionText({kind: 'display'}, ctx)).toMatchObject({subject: '', unknown: true, what: ''});
        expect(actionText({kind: 'display', channel: 'HmIP-RF/X:1'}, ctx)).toMatchObject({subject: 'HmIP-RF/X:1', unknown: true});
        expect(previewText('Wert {value} {var:fenster}')).toBe('Wert ‹value› ‹fe');
        expect(previewText('Hallo')).toBe('Hallo');
    });
    it('has a sign for every icon and words for every icon and tone', () => {
        expect(DISPLAY_ICONS).toHaveLength(32);
        for (const icon of DISPLAY_ICONS.slice(1)) expect(ICON_GLYPHS[icon], icon).toBeTruthy();
        for (const lang of ['de', 'en']) {
            i18n.language = lang;
            for (const icon of DISPLAY_ICONS) expect(displayText({lines: [{icon}]}), icon).not.toContain('icon.');
            for (const sound of DISPLAY_SOUNDS) expect(displayText({sound}), sound).not.toContain('entry.');
        }
        i18n.language = 'en';
    });
    it('shows the lines as they went out in the history', () => {
        const sent = '{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=Wert 21.5,DDID=1,DDC=true}';
        const view = runView({seq: 1, program: 'p', time: '2026-11-22T10:00:00Z', trigger: 'user', block: 0, lines: [], steps: [
            {what: 'sent', action: 1, def: {kind: 'display', channel: CH, display: {lines: [{text: 'Wert {value}'}]}}, value: sent, confirmed: '2026-11-22T10:00:01Z'},
            {what: 'sent', action: 2, def: {kind: 'display', channel: CH, display: {lines: [{text: 'Wert {value}'}]}}, value: 'not a display text'},
        ]}, ctx, 'en');
        expect(view.steps[0]!.text).toBe('Wandtaster Wert 21.5');
        expect(view.steps[1]!.text).toBe('Wandtaster Wert {value}');
    });
});

describe('a CCU program\'s finished text for a display', () => {
    it('is read as lines and a signal tone', () => {
        expect(combinedDisplay('{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=Zeile 1,DDID=1},{DDBC=BLACK,DDTC=WHITE,DDI=24,DDA=LEFT,DDS=T³r ] Tor ²,DDID=3,DDC=true},{R=2,IN=10,ANS=6}')).toEqual({
            lines: [{text: 'Zeile 1'}, {}, {text: 'Tür & Tor ä', icon: 'WINDOW_OPEN', align: 'LEFT', color: 'WHITE', background: 'BLACK'}],
            sound: 'EVENT', repetitions: 2, interval: 10,
        });
        // the short form of the forum, an icon alone, a sound alone
        expect(combinedDisplay(' {DDS=Test,DDID=2,DDC=true} ')).toEqual({lines: [{}, {text: 'Test'}]});
        expect(combinedDisplay('{DDI=30,DDID=1,DDS=}')).toEqual({lines: [{icon: 'BELL'}]});
        expect(combinedDisplay('{R=0,IN=5,ANS=7}')).toEqual({sound: 'ERROR'});
        expect(combinedDisplay('{ANS=0}')).toEqual({sound: 'LOW_BATTERY'});
    });
    it('stays as it is when it cannot be read whole', () => {
        for (const text of [
            '', 'L=50,C=4', '{DDS=XXX,DDID=1,DDC=true}', '{DDS=a,DDID=6}', '{DDS=a,DDID=0}', '{DDS=a}', '{DDS=a,DDID=1},{DDS=b,DDID=1}', '{DDS=a,DDID=1,DDX=1}', '{DDS=a,DDID=1,DDS=b}',
            '{DDS=a,DDID=1,DDI=99}', '{DDS=a,DDID=1,DDI=x}', '{DDS=a,DDID=1,DDA=MIDDLE}', '{DDS=a,DDID=1,DDTC=RED}', '{DDS=a,DDID=1,DDBC=RED}', '{DDS=5 €,DDID=1}', '{DDS=12345678901234567,DDID=1}',
            '{DDID=1}', '{DDS=a,DDID=1,nonsense}', '{=a,DDID=1}', '{R=1,IN=10,ANS=9}', '{R=1,IN=10,ANS=x}', '{R=16,IN=10,ANS=1}', '{R=1.5,IN=10,ANS=1}', '{R=1,IN=7,ANS=1}', '{R=1,IN=10,ANS=1,X=1}',
            '{R=1,IN=10,ANS=1},{R=1,IN=10,ANS=2}',
        ]) {
            expect(combinedDisplay(text), text).toBeUndefined();
        }
    });
});
