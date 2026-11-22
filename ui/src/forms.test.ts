import {readFileSync} from 'node:fs';
import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, describe, expect, it} from 'vitest';

import {channelTypes, type Param} from './editor.js';
import {combinedValues, entryLabel, ENTRIES, formFields, formKind, formProblem, formState, formText, formValues, formWords, hueColor, SWATCHES} from './forms.js';
import {MESSAGES} from './messages.js';

beforeAll(() => {
    addMessages(MESSAGES);
    i18n.language = 'en';
});

// The devices' own descriptions (internal/fakesys/descriptions, each file names its source), as
// the addon hands them to the page.
function described(name: string): Param[] {
    const file = JSON.parse(readFileSync(process.cwd() + `/../internal/fakesys/descriptions/${name}.json`, 'utf8')) as {values: Record<string, {TYPE: string; OPERATIONS: number; MIN?: unknown; MAX?: unknown; UNIT?: string; VALUE_LIST?: string[]}>};
    const n = (v: unknown) => (typeof v === 'number' ? v : undefined);
    return Object.entries(file.values)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([id, d]) => ({id, type: d.TYPE, read: (d.OPERATIONS & 5) !== 0, write: (d.OPERATIONS & 2) !== 0, min: n(d.MIN), max: n(d.MAX), unit: d.UNIT, values: d.VALUE_LIST}));
}

const bsl1 = described('bsl-1');
const bsl2 = described('bsl-2');
const sound = described('mp3p-sound');
const mp3pLight = described('mp3p-light');
const asir = described('asir');
const rgbw = described('rgbw');
const bbl = described('bbl');
const ids = (kind: Parameters<typeof formFields>[0], params: Param[]) => formFields(kind, params).map((f) => f.id);

describe('which channel has a form', () => {
    it('goes by the channel type and what the description takes', () => {
        expect(formKind('DIMMER_VIRTUAL_RECEIVER', bsl2)).toBe('signal');
        expect(formKind('DIMMER_VIRTUAL_RECEIVER', bsl1)).toBe('signal');
        expect(formKind('OPTICAL_SIGNAL_RECEIVER', bsl2)).toBe('signal');
        expect(formKind('DIMMER_VIRTUAL_RECEIVER', mp3pLight)).toBe('signal');
        expect(formKind('ACOUSTIC_SIGNAL_VIRTUAL_RECEIVER', sound)).toBe('sound');
        expect(formKind('ALARM_SWITCH_VIRTUAL_RECEIVER', asir)).toBe('siren');
        expect(formKind('UNIVERSAL_LIGHT_RECEIVER', rgbw)).toBe('light');
        expect(formKind('BLIND_VIRTUAL_RECEIVER', bbl)).toBe('blind');
        // a plain dimmer has no colour: no form; nor a type the page does not know, nor a channel without a description
        expect(formKind('DIMMER_VIRTUAL_RECEIVER', [{id: 'LEVEL', type: 'FLOAT', read: true, write: true}])).toBeUndefined();
        expect(formKind('DIMMER_VIRTUAL_RECEIVER', [{id: 'LEVEL', type: 'FLOAT', read: true, write: true}, {id: 'COLOR', type: 'ENUM', read: true, write: false}])).toBeUndefined();
        expect(formKind('SWITCH_VIRTUAL_RECEIVER', bsl2)).toBeUndefined();
        expect(formKind(undefined, bsl2)).toBeUndefined();
        expect(formKind('BLIND_VIRTUAL_RECEIVER', undefined)).toBeUndefined();
    });
    it('knows the channels\' types from the devices', () => {
        expect(channelTypes([{interface: 'HmIP-RF', address: 'A', type: 'HmIP-BSL', channels: [{address: 'A:0', type: 'MAINTENANCE'}, {address: 'A:8', type: 'DIMMER_VIRTUAL_RECEIVER'}]}])).toEqual({'HmIP-RF/A:0': 'MAINTENANCE', 'HmIP-RF/A:8': 'DIMMER_VIRTUAL_RECEIVER'});
    });
    it('offers the fields the device takes', () => {
        expect(ids('signal', bsl2)).toEqual(['color', 'level', 'behaviour', 'duration', 'ramp']);
        // a HmIP-BSL 1.x and the light of the HmIP-MP3P have no behaviour
        expect(ids('signal', bsl1)).toEqual(['color', 'level', 'duration', 'ramp']);
        expect(ids('signal', mp3pLight)).toEqual(['color', 'level', 'duration', 'ramp']);
        expect(ids('sound', sound)).toEqual(['sound', 'level', 'repetitions', 'duration', 'ramp']);
        expect(ids('siren', asir)).toEqual(['acoustic', 'optical', 'duration']);
        expect(ids('light', rgbw)).toEqual(['hue', 'temperature', 'level', 'ramp', 'duration', 'effect']);
        expect(ids('blind', bbl)).toEqual(['level', 'slats']);
        const f = formFields('siren', asir);
        expect(f[2]).toMatchObject({label: 'For', type: 'span', optional: false, excludes: []});
        expect(f[2]!.defs.map((d) => d.id)).toEqual(['DURATION_VALUE', 'DURATION_UNIT']);
        expect(formFields('light', rgbw)[0]).toMatchObject({optional: true, excludes: ['temperature']});
    });
});

describe('a form and the stored values', () => {
    it('starts a new activity with what makes sense', () => {
        const signal = formFields('signal', bsl2);
        const {state, rest} = formState(signal, []);
        expect(rest).toEqual([]);
        expect(state).toEqual({color: {on: true, a: 'WHITE', b: undefined}, level: {on: true, a: 100, b: undefined}, behaviour: {on: true, a: 'ON', b: undefined}, duration: {on: false, a: 10, b: 'S'}, ramp: {on: false, a: 10, b: 'S'}});
        expect(formValues(signal, state, [])).toEqual([{param: 'COLOR', value: 'WHITE'}, {param: 'LEVEL', value: 1}, {param: 'COLOR_BEHAVIOUR', value: 'ON'}]);
        expect(formState(formFields('sound', sound), []).state).toMatchObject({sound: {on: true, a: 'SOUNDFILE_001'}, repetitions: {on: false, a: 'NO_REPETITION'}});
        expect(formState(formFields('siren', asir), []).state).toEqual({acoustic: {on: true, a: 'FREQUENCY_RISING', b: undefined}, optical: {on: true, a: 'BLINKING_ALTERNATELY_REPEATING', b: undefined}, duration: {on: true, a: 10, b: 'S'}});
        // a coloured light starts with a colour, not with a colour temperature
        const light = formState(formFields('light', rgbw), []).state;
        expect(light).toMatchObject({hue: {on: true, a: 0, b: 100}, temperature: {on: false, a: 2700}, effect: {on: false, a: 'NO_EFFECT'}});
        expect(formValues(formFields('light', rgbw), light, [])).toEqual([{param: 'HUE', value: 0}, {param: 'SATURATION', value: 1}, {param: 'LEVEL', value: 1}]);
        expect(formState(formFields('blind', bbl), []).state).toEqual({level: {on: true, a: 100, b: undefined}, slats: {on: true, a: 100, b: undefined}});
        // a list without the entries the page would take first: its first; a list that is empty: the number 0
        const odd: Param[] = [{id: 'COLOR', type: 'ENUM', read: true, write: true, values: ['', 'AMBER']}, {id: 'LEVEL', type: 'FLOAT', read: true, write: true}, {id: 'COLOR_BEHAVIOUR', type: 'ENUM', read: true, write: true}];
        expect(formState(formFields('signal', odd), []).state).toMatchObject({color: {a: 'AMBER'}, behaviour: {a: 0}});
        // a colour temperature within what the device takes
        expect(formState(formFields('light', [{id: 'LEVEL', type: 'FLOAT', read: true, write: true}, {id: 'COLOR_TEMPERATURE', type: 'INTEGER', read: true, write: true, min: 3000, max: 6500}]), []).state.temperature).toMatchObject({a: 3000});
        expect(formState(formFields('light', [{id: 'LEVEL', type: 'FLOAT', read: true, write: true}, {id: 'COLOR_TEMPERATURE', type: 'INTEGER', read: true, write: true}]), []).state.temperature).toMatchObject({a: 2700});
    });
    it('reads the values back and writes the same ones', () => {
        const fields = formFields('signal', bsl2);
        const stored = [{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 0.5}, {param: 'COLOR_BEHAVIOUR', value: 'BLINKING_MIDDLE'}, {param: 'DURATION_VALUE', value: 10}, {param: 'DURATION_UNIT', value: 'M'}];
        const {state, rest} = formState(fields, stored);
        expect(state).toEqual({color: {on: true, a: 'RED', b: undefined}, level: {on: true, a: 50, b: undefined}, behaviour: {on: true, a: 'BLINKING_MIDDLE', b: undefined}, duration: {on: true, a: 10, b: 'M'}, ramp: {on: false, a: 10, b: 'S'}});
        expect(rest).toEqual([]);
        expect(formValues(fields, state, rest)).toEqual(stored);
        expect(formText(fields, state)).toBe('Red, Brightness 50 %, Blinking, medium, For 10 min');
        // an entry stored by its number is shown by its name; a share with a decimal keeps it
        const numbered = formState(fields, [{param: 'COLOR', value: 4}, {param: 'LEVEL', value: 0.125}, {param: 'COLOR_BEHAVIOUR', value: 1}, {param: 'RAMP_TIME_VALUE', value: 2}, {param: 'RAMP_TIME_UNIT', value: 1}]);
        expect(numbered.state).toMatchObject({color: {a: 'RED'}, level: {a: 12.5}, behaviour: {a: 'ON'}, ramp: {on: true, a: 2, b: 'M'}, duration: {on: false}});
        expect(formValues(fields, numbered.state, [])).toContainEqual({param: 'LEVEL', value: 0.125});
        // half a pair is not the field's: it stays beside the form, as does a datapoint the form has no field for
        const half = formState(fields, [{param: 'COLOR', value: 'RED'}, {param: 'DURATION_VALUE', value: 3}, {param: 'COMBINED_PARAMETER', value: 'x'}, {param: 'LEVEL', value: 'much'}]);
        expect(half.state).toMatchObject({duration: {on: false}, level: {on: true, a: 'much'}, behaviour: {on: true, a: 'ON'}});
        expect(half.rest).toEqual([{param: 'DURATION_VALUE', value: 3}, {param: 'COMBINED_PARAMETER', value: 'x'}]);
        // ticked in the form afterwards: the form's value goes, the kept one does not go twice
        half.state.duration!.on = true;
        expect(formValues(fields, half.state, half.rest)).toEqual([{param: 'COLOR', value: 'RED'}, {param: 'LEVEL', value: 'much'}, {param: 'COLOR_BEHAVIOUR', value: 'ON'}, {param: 'DURATION_VALUE', value: 10}, {param: 'DURATION_UNIT', value: 'S'}, {param: 'COMBINED_PARAMETER', value: 'x'}]);
        // a field that is not in the state is not written
        expect(formValues(fields, {}, [{param: 'X', value: 1}])).toEqual([{param: 'X', value: 1}]);
    });
    it('a coloured light: colour or colour temperature', () => {
        const fields = formFields('light', rgbw);
        const colour = formState(fields, [{param: 'HUE', value: 120}, {param: 'SATURATION', value: 0.8}, {param: 'LEVEL', value: 0.6}, {param: 'EFFECT', value: 'EFFECT_02_INTERRUPT_CURRENT_PROFILE'}]);
        expect(colour.state).toMatchObject({hue: {on: true, a: 120, b: 80}, temperature: {on: false}, level: {a: 60}, effect: {on: true}});
        expect(formText(fields, colour.state)).toBe('hue 120°, saturation 80 %, Brightness 60 %, Effect 2, at once');
        expect(formValues(fields, colour.state, [])).toEqual([{param: 'HUE', value: 120}, {param: 'SATURATION', value: 0.8}, {param: 'LEVEL', value: 0.6}, {param: 'EFFECT', value: 'EFFECT_02_INTERRUPT_CURRENT_PROFILE'}]);
        const white = formState(fields, [{param: 'COLOR_TEMPERATURE', value: 2700}, {param: 'LEVEL', value: 0.4}]);
        expect(white.state).toMatchObject({hue: {on: false}, temperature: {on: true, a: 2700}});
        expect(formText(fields, white.state)).toBe('2700 K, Brightness 40 %');
        expect(formProblem(fields, white.state)).toBeNull();
        white.state.hue!.on = true;
        expect(formProblem(fields, white.state)).toEqual({field: 'hue', text: 'Colour: Either a colour or a colour temperature, not both.'});
        white.state.temperature!.on = false;
        white.state.hue!.a = 361;
        expect(formProblem(fields, white.state)?.text).toBe('Colour: outside 0 to 360');
        white.state.hue!.a = 10;
        white.state.hue!.b = 101;
        expect(formProblem(fields, white.state)?.text).toBe('Colour: outside 0 to 100');
        white.state.hue!.b = 50;
        white.state.temperature!.on = true;
        white.state.hue!.on = false;
        white.state.temperature!.a = 500;
        expect(formProblem(fields, white.state)).toEqual({field: 'temperature', text: 'Colour temperature: outside 1000 to 10200'});
        white.state.temperature!.a = 2700.5;
        expect(formProblem(fields, white.state)?.text).toBe('Colour temperature: Enter a whole number.');
    });
    it('says what is wrong with a number', () => {
        const fields = formFields('blind', bbl);
        const {state} = formState(fields, [{param: 'LEVEL', value: 0.5}, {param: 'LEVEL_2', value: 0.25}]);
        expect(formProblem(fields, state)).toBeNull();
        expect(formText(fields, state)).toBe('Height 50 %, Slats 25 %');
        state.slats!.a = 120;
        expect(formProblem(fields, state)).toEqual({field: 'slats', text: 'Slats: outside 0 to 100'});
        state.slats!.a = undefined;
        expect(formProblem(fields, state)?.text).toBe('Slats: Enter a number.');
        expect(formText(fields, state)).toBe('Height 50 %, Slats  %');
        state.slats!.on = false;
        expect(formProblem(fields, state)).toBeNull();
        const siren = formFields('siren', asir);
        const s = formState(siren, []).state;
        s.duration!.a = 20000;
        expect(formProblem(siren, s)?.text).toBe('For: outside 0 to 16343');
        s.duration!.a = 1.5;
        expect(formProblem(siren, s)?.text).toBe('For: Enter a whole number.');
        // a number without limits in its description
        const free = formFields('light', [{id: 'LEVEL', type: 'FLOAT', read: true, write: true}, {id: 'COLOR_TEMPERATURE', type: 'FLOAT', read: true, write: true}]);
        const fs = formState(free, [{param: 'COLOR_TEMPERATURE', value: 99999.5}, {param: 'LEVEL', value: 1}]).state;
        expect(formProblem(free, fs)).toBeNull();
        expect(formText(free, fs)).toBe('99999.5, Brightness 100 %');
    });
    it('says an activity\'s values in the form\'s words where they are the form\'s', () => {
        expect(formWords('BLIND_VIRTUAL_RECEIVER', bbl, [{param: 'LEVEL', value: 0.5}, {param: 'LEVEL_2', value: 0.25}])).toBe('Height 50 %, Slats 25 %');
        expect(formWords('ALARM_SWITCH_VIRTUAL_RECEIVER', asir, [{param: 'ACOUSTIC_ALARM_SELECTION', value: 'FREQUENCY_FALLING'}, {param: 'OPTICAL_ALARM_SELECTION', value: 'DOUBLE_FLASHING_REPEATING'}, {param: 'DURATION_VALUE', value: 3}, {param: 'DURATION_UNIT', value: 'M'}]))
            .toBe('Falling tone, Double flashes, For 3 min');
        expect(formWords('ACOUSTIC_SIGNAL_VIRTUAL_RECEIVER', sound, [{param: 'SOUNDFILE', value: 'SOUNDFILE_012'}, {param: 'LEVEL', value: 0.3}, {param: 'REPETITIONS', value: 'REPETITIONS_002'}])).toBe('Sound file 12, Volume 30 %, 2 repetitions');
        // one of the form's own is missing, a datapoint beside the form, nothing set, no form: the plain words are the row's
        expect(formWords('BLIND_VIRTUAL_RECEIVER', bbl, [{param: 'LEVEL', value: 0.5}])).toBe('');
        expect(formWords('BLIND_VIRTUAL_RECEIVER', bbl, [{param: 'LEVEL', value: 0.5}, {param: 'LEVEL_2', value: 0.25}, {param: 'STOP', value: true}])).toBe('');
        expect(formWords('BLIND_VIRTUAL_RECEIVER', bbl, [])).toBe('');
        expect(formWords('SHUTTER_VIRTUAL_RECEIVER', bbl, [{param: 'LEVEL', value: 0.5}])).toBe('');
    });
});

describe('the entries of a device\'s lists', () => {
    it('are said in words', () => {
        i18n.language = 'de';
        expect(entryLabel('RED')).toBe('Rot');
        expect(entryLabel('BLINKING_MIDDLE')).toBe('Blinken, mittel');
        expect(entryLabel('SOUNDFILE_012')).toBe('Tondatei 12');
        expect(entryLabel('REPETITIONS_001')).toBe('1 Wiederholung');
        expect(entryLabel('REPETITIONS_014')).toBe('14 Wiederholungen');
        expect(entryLabel('EFFECT_03_END_CURRENT_PROFILE')).toBe('Effekt 3, nach dem laufenden');
        expect(entryLabel('EFFECT_10_INTERRUPT_CURRENT_PROFILE')).toBe('Effekt 10, sofort');
        i18n.language = 'en';
        expect(entryLabel('S')).toBe('s');
        expect(entryLabel('10MS')).toBe('× 10 ms');
        expect(entryLabel('SOMETHING_NEW')).toBe('SOMETHING_NEW');
    });
    it('cover the lists of the devices the forms were built against', () => {
        const lists = [bsl2, sound, asir, rgbw].flatMap((params) => params.filter((p) => p.write && p.type === 'ENUM').flatMap((p) => p.values ?? []));
        const unnamed = [...new Set(lists)].filter((x) => entryLabel(x) === x);
        expect(unnamed).toEqual([]);
        for (const e of ENTRIES) expect(entryLabel(e)).not.toBe('entry.' + e);
    });
    it('paint the colours a signal light has', () => {
        const colours = bsl2.find((p) => p.id === 'COLOR')!.values!;
        expect(colours.filter((c) => SWATCHES[c])).toEqual(['BLACK', 'BLUE', 'GREEN', 'TURQUOISE', 'RED', 'PURPLE', 'YELLOW', 'WHITE']);
        expect(hueColor(0, 100)).toBe('hsl(0 100% 50%)');
        expect(hueColor(120, 0)).toBe('hsl(120 100% 100%)');
        expect(hueColor(361, 50)).toBe('hsl(1 100% 75%)');
        expect(hueColor(-10, 200)).toBe('hsl(350 100% 50%)');
        expect(hueColor(Number.NaN, Number.NaN)).toBe('hsl(0 100% 100%)');
    });
});

describe('a COMBINED_PARAMETER string of a CCU program', () => {
    it('is read as the datapoints it stands for', () => {
        // the HmIP-BSL 2.x string of the forum: colour 4, 31 hours (the CCU's "without end"), behaviour 1
        expect(combinedValues('L=50,C=4,DV=31,DU=2,CB=1', bsl2)).toEqual([{param: 'LEVEL', value: 0.5}, {param: 'COLOR', value: 'RED'}, {param: 'DURATION_VALUE', value: 31}, {param: 'DURATION_UNIT', value: 'H'}, {param: 'COLOR_BEHAVIOUR', value: 'ON'}]);
        expect(combinedValues(' L = 100 , DV=10,DU=0,RTV=2,RTU=0,C=7', bsl1)).toEqual([{param: 'LEVEL', value: 1}, {param: 'DURATION_VALUE', value: 10}, {param: 'DURATION_UNIT', value: 'S'}, {param: 'RAMP_TIME_VALUE', value: 2}, {param: 'RAMP_TIME_UNIT', value: 'S'}, {param: 'COLOR', value: 'WHITE'}]);
        // the blind: height and slats; 100.5 is the CCU's "as before", 101 its "leave"
        expect(combinedValues('L=40,L2=100.5', bbl)).toEqual([{param: 'LEVEL', value: 0.4}, {param: 'LEVEL_2', value: 1.005}]);
        expect(combinedValues('L2=101,L=0', bbl)).toEqual([{param: 'LEVEL_2', value: 1.01}, {param: 'LEVEL', value: 0}]);
        expect(combinedValues('L=30,DU=0,DV=20,RTU=0,RTV=0,R=2,SL=5', sound)).toEqual([{param: 'LEVEL', value: 0.3}, {param: 'DURATION_UNIT', value: 'S'}, {param: 'DURATION_VALUE', value: 20}, {param: 'RAMP_TIME_UNIT', value: 'S'}, {param: 'RAMP_TIME_VALUE', value: 0}, {param: 'REPETITIONS', value: 'REPETITIONS_002'}, {param: 'SOUNDFILE', value: 'SOUNDFILE_005'}]);
        expect(combinedValues('L=80,DV=5,DU=1,RTV=0,RTU=0,H=120,SAT=100', rgbw)).toEqual([{param: 'LEVEL', value: 0.8}, {param: 'DURATION_VALUE', value: 5}, {param: 'DURATION_UNIT', value: 'M'}, {param: 'RAMP_TIME_VALUE', value: 0}, {param: 'RAMP_TIME_UNIT', value: 'S'}, {param: 'HUE', value: 120}, {param: 'SATURATION', value: 1}]);
        expect(combinedValues('L=100,TC=4500,E=0', rgbw)).toEqual([{param: 'LEVEL', value: 1}, {param: 'COLOR_TEMPERATURE', value: 4500}, {param: 'EFFECT', value: 'NO_EFFECT'}]);
    });
    it('stays as it is when it cannot be read whole', () => {
        expect(combinedValues('', bbl)).toBeUndefined();
        expect(combinedValues('L=50', undefined)).toBeUndefined();
        // a short name the page does not know (the on-time in seconds), a list of sound files, text
        expect(combinedValues('L=100,OT=0,RT=0,H=0,SAT=100', rgbw)).toBeUndefined();
        expect(combinedValues('L=30,R=0,SL=1;2;3', sound)).toBeUndefined();
        expect(combinedValues('hello', bbl)).toBeUndefined();
        expect(combinedValues('L=50,', bbl)).toBeUndefined();
        // a datapoint the channel does not take, one named twice
        expect(combinedValues('L=50,CB=1', bsl1)).toBeUndefined();
        expect(combinedValues('L=50,L=60', bbl)).toBeUndefined();
        // a value outside what the datapoint takes, an entry the list does not have, half an entry, half a whole number
        expect(combinedValues('L=150,L2=0', bbl)).toBeUndefined();
        expect(combinedValues('L=-1,L2=0', bbl)).toBeUndefined();
        expect(combinedValues('L=50,C=40', bsl2)).toBeUndefined();
        expect(combinedValues('L=50,C=1.5', bsl2)).toBeUndefined();
        expect(combinedValues('L=50,DV=1.5', bsl2)).toBeUndefined();
        expect(combinedValues('L=50,DV=99999', bsl2)).toBeUndefined();
        // a datapoint that takes neither a number nor an entry
        expect(combinedValues('L=1', [{id: 'LEVEL', type: 'STRING', read: true, write: true}])).toBeUndefined();
        expect(combinedValues('C=1', [{id: 'COLOR', type: 'ENUM', read: true, write: true}])).toBeUndefined();
        // limits the description does not give are not checked
        expect(combinedValues('TC=1', [{id: 'COLOR_TEMPERATURE', type: 'INTEGER', read: true, write: true}])).toEqual([{param: 'COLOR_TEMPERATURE', value: 1}]);
    });
});
