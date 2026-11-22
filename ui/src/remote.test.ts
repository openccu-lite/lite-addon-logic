import {addMessages, i18n} from 'occulite-ui/i18n';
import {beforeAll, beforeEach, describe, expect, it} from 'vitest';

import {MESSAGES} from './messages.js';
import {addressKind, changeKind, changeText, entryText, NO_REMOTE, relationText, remoteErrorText, remoteProblem, remoteState, type RemoteInfo, type RemotePreview} from './remote.js';

const info = (more: Partial<RemoteInfo>): RemoteInfo => ({...NO_REMOTE, url: 'https://git.example.org/me/home.git', branch: 'main', kind: 'https', ...more});
const preview = (more: Partial<RemotePreview>): RemotePreview => ({relation: 'same', local: 'a', remote: 'b', entries: [], changes: [], problems: [], can_apply: false, ...more});
const entry = (n: number) => ({hash: String(n), time: '2026-11-22T12:00:00+01:00', author: 'Admin', message: 'Variable Regen changed'});

describe('the history\'s remote', () => {
    beforeAll(() => addMessages(MESSAGES));
    beforeEach(() => {
        i18n.language = 'en';
    });

    it('reads an address as the addon does', () => {
        expect(addressKind('https://git.example.org/me/home.git')).toBe('https');
        expect(addressKind(' https://git.example.org/me/home.git ')).toBe('https');
        expect(addressKind('http://127.0.0.1:18081/dev/git/haus')).toBe('https');
        expect(addressKind('http://localhost/x')).toBe('https');
        expect(addressKind('http://[::1]:3000/x')).toBe('https');
        expect(addressKind('http://git.example.org/me/home.git')).toBe('');
        expect(addressKind('ssh://git@git.example.org:2222/me/home.git')).toBe('ssh');
        expect(addressKind('ssh://git.example.org/me/home.git')).toBe('ssh');
        expect(addressKind('git@github.com:me/home.git')).toBe('ssh');
        expect(addressKind('/srv/git/home.git')).toBe('');
        expect(addressKind('https://git.example.org')).toBe('');
        expect(addressKind('https://me:pw@git.example.org/x.git')).toBe('');
        expect(addressKind('')).toBe('');
    });

    it('says what is wrong with the form before it is sent', () => {
        expect(remoteProblem('', '')).toBe('Enter the address of the repository.');
        expect(remoteProblem('http://git.example.org/x.git', '')).toBe('http:// would send the token unprotected: use https:// or ssh.');
        expect(remoteProblem('ftp://git.example.org/x.git', '')).toBe('The address must be https://…, ssh://… or user@host:path.');
        expect(remoteProblem('https://git.example.org/x.git', 'a b')).toBe('That cannot be a branch\'s name.');
        expect(remoteProblem('https://git.example.org/x.git', '-x')).toBe('That cannot be a branch\'s name.');
        expect(remoteProblem('https://git.example.org/x.git', '')).toBe('');
        expect(remoteProblem('git@github.com:me/home.git', 'haus/keller')).toBe('');
        expect(remoteProblem('http://127.0.0.1:18081/dev/git/haus', ' main ')).toBe('');
    });

    it('says where the push stands in one sentence', () => {
        expect(remoteState(NO_REMOTE, 'en')).toEqual({text: 'No remote is set: the history stays on this system.', bad: false});
        expect(remoteState(info({pending: true}), 'en')).toEqual({text: 'A push is under way.', bad: false});
        expect(remoteState(info({}), 'en')).toEqual({text: 'In step with the remote.', bad: false});
        expect(remoteState(info({last_push: '2026-11-22T12:00:00+01:00'}), 'en').text).toMatch(/^In step with the remote; last pushed 22\/11\/2026, 12:00/);
        expect(remoteState(info({pending: true, error: 'x', error_code: 'auth'}), 'en')).toEqual({text: 'The remote does not accept the token.', bad: true});
        i18n.language = 'de';
        expect(remoteState(info({}), 'de').text).toBe('Auf dem Stand des Remotes.');
    });

    it('has words for every way a push or a fetch fails', () => {
        expect(remoteErrorText(info({kind: 'ssh', error: 'x', error_code: 'auth'}))).toContain('enter the public key below as a deploy key');
        expect(remoteErrorText(info({error: 'x', error_code: 'not-found'}))).toContain('no such repository');
        expect(remoteErrorText(info({error: 'x', error_code: 'host-key'}))).toContain('another key than the one remembered');
        expect(remoteErrorText(info({error: 'x', error_code: 'rejected'}))).toContain('did not take the push');
        expect(remoteErrorText(info({error: 'x', error_code: 'unreachable'}))).toBe('The remote cannot be reached.');
        expect(remoteErrorText(info({error: 'the disk is full', error_code: 'failed'}))).toBe('the disk is full');
        expect(remoteErrorText(info({error_code: 'failed'}))).toBe('');
    });

    it('says how the two histories relate', () => {
        expect(relationText(preview({relation: 'empty'}))).toContain('the next push creates it');
        expect(relationText(preview({relation: 'same'}))).toBe('The remote and this system have the same history.');
        expect(relationText(preview({relation: 'ahead'}))).toContain('a push sends them');
        expect(relationText(preview({relation: 'behind', entries: [entry(1)]}))).toBe('The remote has 1 entry this system lacks.');
        expect(relationText(preview({relation: 'behind', entries: [entry(1), entry(2)]}))).toBe('The remote has 2 entries this system lacks.');
        expect(relationText(preview({relation: 'behind', entries: [entry(1), entry(2)], more: true}))).toBe('The remote has 2+ entries this system lacks.');
        expect(relationText(preview({relation: 'new'}))).toContain('can be taken over as it is');
        expect(relationText(preview({relation: 'diverged'}))).toContain('each gone their own way');
        i18n.language = 'de';
        expect(relationText(preview({relation: 'behind', entries: [entry(1)]}))).toBe('Das Remote hat 1 Eintrag, der diesem System fehlt.');
    });

    it('names a change and its kind', () => {
        expect(changeKind({path: 'p', kind: 'added'})).toEqual({label: 'new', kind: 'ok'});
        expect(changeKind({path: 'p', kind: 'changed'})).toEqual({label: 'changed', kind: ''});
        expect(changeKind({path: 'p', kind: 'removed'})).toEqual({label: 'removed', kind: 'warn'});
        expect(changeKind({path: 'p', kind: 'added', problem: 'no'})).toEqual({label: 'not possible', kind: 'bad'});
        expect(changeText({path: 'programs/a.yaml', kind: 'added', what: 'program', name: 'Markise'})).toBe('Program Markise');
        expect(changeText({path: 'programs/a.yaml', kind: 'added', what: 'program'})).toBe('Program programs/a.yaml');
        expect(changeText({path: 'variables/v.yaml', kind: 'removed', what: 'variable', name: 'Regen'})).toBe('System variable Regen');
        expect(changeText({path: 'variables/v.yaml', kind: 'removed', what: 'variable'})).toBe('System variable variables/v.yaml');
        expect(changeText({path: 'settings.yaml', kind: 'changed', what: 'settings'})).toBe('The settings');
        expect(changeText({path: 'notes/x.txt', kind: 'added'})).toBe('notes/x.txt');
        expect(entryText(entry(1), 'en')).toMatch(/^22\/11\/2026, 12:00.* · Admin · Variable Regen changed$/);
    });
});
