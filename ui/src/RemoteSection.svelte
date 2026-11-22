<script lang="ts">
    // The history's remote: another git repository the history is pushed to after every entry,
    // and from which it can be taken over - only when it continues what is here. The token and
    // the private key never come back from the addon; the public key is shown to be entered at
    // the host.
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import {ApiError, del, get, post, put} from './api.js';
    import {addressKind, changeKind, changeText, entryText, NO_REMOTE, relationText, remoteProblem, remoteState, type RemoteInfo, type RemotePreview} from './remote.js';

    // (tick changes whenever the page has asked the addon anew: the state here follows)
    let {tick, onchanged}: {tick: string; onchanged: () => void} = $props();

    let info = $state<RemoteInfo>(NO_REMOTE);
    let url = $state('');
    let branch = $state('');
    let username = $state('');
    let token = $state('');
    let touched = $state(false);
    let error = $state('');
    let saved = $state('');
    let busy = $state('');
    let preview = $state<RemotePreview | null>(null);
    let ask = $state<'' | 'remove' | 'key' | 'host' | 'pull'>('');

    const kind = $derived(addressKind(url) || info.kind);
    const status = $derived(remoteState(info, i18n.language));

    function take(r: RemoteInfo) {
        info = r;
        if (!touched) {
            url = r.url;
            branch = r.kind === '' || r.branch === 'main' ? '' : r.branch;
            username = r.username;
            token = '';
        }
    }

    async function load() {
        try {
            take(await get<RemoteInfo>('api/v1/remote'));
        } catch {
            // the page's own banner says when the addon does not answer
        }
    }

    $effect(() => {
        void tick;
        if (busy === '') load();
    });
    $effect(() => {
        // a push runs in the background: look again while one is due
        const timer = setInterval(() => {
            if (info.pending && busy === '') load();
        }, 3000);
        return () => clearInterval(timer);
    });

    async function act(what: string, run: () => Promise<RemoteInfo>, done: string): Promise<boolean> {
        saved = '';
        error = '';
        busy = what;
        try {
            touched = false;
            take(await run());
            saved = done;
            return true;
        } catch (e) {
            touched = true;
            error = (e as Error).message;
            return false;
        } finally {
            busy = '';
        }
    }

    async function save(e: Event) {
        e.preventDefault();
        const problem = remoteProblem(url, branch);
        if (problem) {
            saved = '';
            error = problem;
            return;
        }
        const body: {url: string; branch: string; username: string; token?: string} = {url: url.trim(), branch: branch.trim(), username: kind === 'https' ? username.trim() : ''};
        if (kind === 'https' && token !== '') body.token = token;
        if (await act('save', () => put<RemoteInfo>('api/v1/remote', body), t('The remote was saved; the history is being pushed.'))) preview = null;
    }

    async function confirmed() {
        const what = ask;
        ask = '';
        if (what === 'remove') {
            if (await act('remove', () => del<RemoteInfo>('api/v1/remote'), t('The remote was removed. The history stays on this system.'))) preview = null;
        } else if (what === 'key') {
            await act('key', () => post<RemoteInfo>('api/v1/remote/key'), t('A new key was made: enter it at the host instead of the old one.'));
        } else if (what === 'host') {
            await act('host', () => del<RemoteInfo>('api/v1/remote/host-key'), t('The host\'s key was forgotten: the next contact remembers the one the host presents then.'));
        } else if (what === 'pull' && preview) {
            saved = '';
            error = '';
            busy = 'pull';
            try {
                const done = await post<RemotePreview>('api/v1/remote/pull', {remote: preview.remote});
                saved = t('{n} files were taken over.', {n: done.changes.length}, done.changes.length);
                preview = null;
                onchanged();
                await load();
            } catch (e) {
                error = e instanceof ApiError && e.code === 'moved' ? t('Something changed since the fetch: fetch again.') : (e as Error).message;
            } finally {
                busy = '';
            }
        }
    }

    async function push() {
        await act('push', () => post<RemoteInfo>('api/v1/remote/push'), '');
    }

    async function fetchRemote() {
        saved = '';
        error = '';
        busy = 'fetch';
        try {
            preview = await post<RemotePreview>('api/v1/remote/fetch');
        } catch (e) {
            preview = null;
            if (!(e instanceof ApiError) || !e.code.startsWith('remote-')) error = (e as Error).message;
        } finally {
            busy = '';
        }
        await load();
    }
</script>

<section class="lgc-section" data-testid="settings-remote">
    <div class="lgc-section-head lgc-head-tight">
        <h2>{t('Remote for the history')}</h2>
        <HelpLink topic="remote" />
    </div>
    <p class="lgc-muted">{t('The history of the programs\' and variables\' files can be pushed to a git repository elsewhere - a copy outside this system, with every earlier version. It goes out after every change by itself.')}</p>
    <form class="lgc-settings-form" onsubmit={save}>
        <label class="lgc-field">
            <span>{t('Address')}</span>
            <input class="hmm-input lgc-remote-wide" type="text" spellcheck="false" autocomplete="off" bind:value={url} oninput={() => (touched = true)} placeholder="https://git.example.org/me/home.git" data-testid="remote-url" />
        </label>
        <label class="lgc-field">
            <span>{t('Branch')}</span>
            <input class="hmm-input" type="text" spellcheck="false" autocomplete="off" bind:value={branch} oninput={() => (touched = true)} placeholder="main" data-testid="remote-branch" />
            <span class="lgc-muted lgc-small">{t('the branch at the remote')}</span>
        </label>
        <!-- the two transports' fields take the same two rows: nothing moves when the address changes its kind -->
        {#if kind === 'ssh'}
            <label class="lgc-field">
                <span>{t('Public key')}</span>
                <input class="hmm-input lgc-remote-wide lgc-mono" type="text" readonly value={info.kind === 'ssh' ? (info.public_key ?? '') : ''} placeholder={t('made when the remote is saved')} onfocus={(e) => e.currentTarget.select()} data-testid="remote-public-key" />
            </label>
            <div class="lgc-field">
                <span>{t('Host key')}</span>
                <span class="lgc-mono lgc-cut lgc-remote-host" title={info.host_key ?? ''} data-testid="remote-host-key">{info.kind === 'ssh' && info.host_key ? info.host_key : t('remembered at the first contact')}</span>
                <span class="lgc-remote-buttons">
                    <button class="hmm-button" type="button" disabled={info.kind !== 'ssh' || busy !== ''} onclick={() => (ask = 'key')} data-testid="remote-new-key">{t('New key')}</button>
                    <button class="hmm-button" type="button" disabled={info.kind !== 'ssh' || !info.host_key || busy !== ''} onclick={() => (ask = 'host')} data-testid="remote-forget-host">{t('Forget')}</button>
                </span>
            </div>
        {:else}
            <label class="lgc-field">
                <span>{t('User name')}</span>
                <input class="hmm-input" type="text" spellcheck="false" autocomplete="off" bind:value={username} oninput={() => (touched = true)} placeholder="git" data-testid="remote-username" />
                <span class="lgc-muted lgc-small">{t('most hosts ignore it with a token')}</span>
            </label>
            <label class="lgc-field">
                <span>{t('Token')}</span>
                <input class="hmm-input" type="password" autocomplete="new-password" bind:value={token} oninput={() => (touched = true)} placeholder={info.has_token ? t('unchanged') : ''} data-testid="remote-token" />
                <span class="lgc-muted lgc-small">{t('an access token with write access; is stored on the system and not shown again')}</span>
            </label>
        {/if}
        <div class="lgc-actions lgc-remote-actions">
            <button class="hmm-button lgc-primary" type="submit" disabled={busy !== ''} data-testid="remote-save">{t('Save')}</button>
            <button class="hmm-button" type="button" disabled={info.kind === '' || busy !== ''} onclick={push} data-testid="remote-push">{t('Push now')}</button>
            <button class="hmm-button" type="button" disabled={info.kind === '' || busy !== ''} onclick={fetchRemote} data-testid="remote-fetch">{t('Fetch…')}</button>
            <button class="hmm-button" type="button" disabled={info.kind === '' || busy !== ''} onclick={() => (ask = 'remove')} data-testid="remote-remove">{t('Remove the remote')}</button>
        </div>
        <!-- where the push stands, always; below it what the last click answered -->
        <p class="lgc-settings-message lgc-remote-state" role="status">
            {#if busy === 'fetch'}{t('Fetching…')}{:else if busy === 'push'}{t('Pushing…')}{:else}<span class={status.bad ? 'lgc-bad' : ''} data-testid="remote-state">{status.text}</span>{/if}
        </p>
        <p class="lgc-settings-message lgc-cut" role="status" data-testid="remote-message">
            {#if error}<span class="lgc-bad" title={error}>{error}</span>{:else if saved}{saved}{:else}&nbsp;{/if}
        </p>
    </form>

    {#if preview}
        <div class="lgc-remote-preview" data-testid="remote-preview">
            <p data-testid="remote-relation">{relationText(preview)}</p>
            {#if preview.entries.length > 0}
                <ul class="lgc-remote-entries" data-testid="remote-entries">
                    {#each preview.entries as v (v.hash)}<li class="lgc-cut" title={entryText(v, i18n.language)}>{entryText(v, i18n.language)}</li>{/each}
                    {#if preview.more}<li class="lgc-muted">{t('… and older entries')}</li>{/if}
                </ul>
            {/if}
            {#if preview.changes.length > 0}
                <div class="lgc-remote-changes" role="list" data-testid="remote-changes">
                    {#each preview.changes as c (c.path)}
                        {@const k = changeKind(c)}
                        <div class="lgc-remote-change" role="listitem" data-testid="remote-change">
                            <span><span class={'lgc-pill ' + k.kind}>{k.label}</span></span>
                            <span class="lgc-cut" title={c.path}>{changeText(c)}</span>
                            <span class="lgc-cut lgc-bad" title={c.problem ?? ''}>{c.problem ?? ''}</span>
                        </div>
                    {/each}
                </div>
            {/if}
            {#each preview.problems as p (p)}<div class="lgc-warning" data-testid="remote-problem">{p}</div>{/each}
            {#if preview.relation === 'behind' || preview.relation === 'new'}
                <div class="lgc-actions lgc-remote-actions">
                    <button class="hmm-button lgc-primary" type="button" disabled={!preview.can_apply || busy !== ''} onclick={() => (ask = 'pull')} data-testid="remote-pull">{t('Take over')}</button>
                    <button class="hmm-button" type="button" onclick={() => (preview = null)} data-testid="remote-preview-close">{t('Close')}</button>
                    {#if !preview.can_apply}<span class="lgc-bad lgc-small" data-testid="remote-cannot">{t('Put right at the remote what is marked, then fetch again.')}</span>{/if}
                </div>
            {:else}
                <div class="lgc-actions">
                    <button class="hmm-button" type="button" onclick={() => (preview = null)} data-testid="remote-preview-close">{t('Close')}</button>
                </div>
            {/if}
        </div>
    {/if}
</section>
<ConfirmDialog
    open={ask !== ''}
    title={ask === 'remove' ? t('Remove the remote?') : ask === 'key' ? t('Make a new key?') : ask === 'host' ? t('Forget the host\'s key?') : t('Take the remote\'s history over?')}
    message={ask === 'remove'
        ? t('The history is no longer pushed. The token or the key is removed from this system; the repository at the remote stays as it is.')
        : ask === 'key'
          ? t('The old key stops working as soon as the host has the new one instead. Until then pushes fail.')
          : ask === 'host'
            ? t('Only when you know why the host presents another key - it was set up anew, for example.')
            : t('The files are changed as listed; programs and variables are read anew. Nothing runs because of it, values stay.')}
    confirmLabel={ask === 'remove' ? t('Remove') : ask === 'key' ? t('New key') : ask === 'host' ? t('Forget') : t('Take over')}
    danger={ask === 'remove'}
    testId="confirm-remote"
    onconfirm={confirmed}
    onclose={() => (ask = '')}
/>
