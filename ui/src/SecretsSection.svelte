<script lang="ts">
    // The webhooks' secrets: entered here once, named in a webhook as {secret:<name>}. The
    // addon keeps them in a file of its own - not with the programs' files, not in their
    // history - and never shows a value again: a secret is replaced by entering its name anew.
    import {t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import {ApiError, del, get, put} from './api.js';

    interface Secrets {
        secrets: {name: string; users: string[]}[];
        missing: string[];
    }

    let {programs}: {programs: Record<string, string>} = $props();

    let data = $state<Secrets>({secrets: [], missing: []});
    let name = $state('');
    let value = $state('');
    let error = $state('');
    let saved = $state('');
    let removing = $state('');

    const users = (keys: string[]) => keys.map((k) => programs[k] ?? k).join(', ');

    async function act(run: () => Promise<Secrets>, done: string) {
        saved = '';
        try {
            data = await run();
            error = '';
            saved = done;
            return true;
        } catch (e) {
            error = e instanceof ApiError && e.code === 'in-use' ? t('The secret is used by programs and cannot be removed.') : (e as Error).message;
            return false;
        }
    }

    // read again whenever the page has asked the addon anew: who uses a secret follows the programs
    $effect(() => {
        void programs;
        get<Secrets>('api/v1/secrets').then(
            (r) => (data = r),
            () => undefined,
        );
    });

    async function save(e: Event) {
        e.preventDefault();
        const n = name.trim();
        if (!/^[a-z0-9]([a-z0-9-]{0,46}[a-z0-9])?$/.test(n) || n.includes('--')) {
            saved = '';
            error = t('A secret\'s name is made of lower-case letters, digits and hyphens.');
            return;
        }
        if (value === '') {
            saved = '';
            error = t('Enter the secret\'s value.');
            return;
        }
        const replaced = data.secrets.some((s) => s.name === n);
        if (await act(() => put<Secrets>('api/v1/secrets/' + n, {value}), replaced ? t('The secret {name} was replaced.', {name: n}) : t('The secret {name} was saved.', {name: n}))) {
            name = '';
            value = '';
        }
    }

    async function remove() {
        const n = removing;
        removing = '';
        await act(() => del<Secrets>('api/v1/secrets/' + n), t('The secret {name} was removed.', {name: n}));
    }
</script>

<section class="lgc-section" data-testid="settings-secrets">
    <div class="lgc-section-head lgc-head-tight">
        <h2>{t('Secrets')}</h2>
        <HelpLink topic="secrets" />
    </div>
    <p class="lgc-muted">{t('Tokens and passwords for webhooks: a webhook names one as {secret:name}. A value is never shown again; entering a name anew replaces it.')}</p>
    <div class="lgc-secrets" role="list">
        {#each data.secrets as s (s.name)}
            <div class="lgc-secret-row" role="listitem" data-testid="secret-{s.name}">
                <span class="lgc-mono lgc-cut" title={s.name}>{s.name}</span>
                <span class="lgc-cut lgc-muted" title={users(s.users)} data-testid="secret-users">{s.users.length > 0 ? t('used by {programs}', {programs: users(s.users)}) : t('not used')}</span>
                <button class="hmm-button lgc-ed-icon" type="button" disabled={s.users.length > 0} title={s.users.length > 0 ? t('The secret is used by programs and cannot be removed.') : t('Remove')} aria-label={t('Remove')} onclick={() => (removing = s.name)} data-testid="secret-remove">✕</button>
            </div>
        {:else}
            <div class="lgc-secret-row lgc-muted" role="listitem" data-testid="secrets-none"><span class="lgc-secret-wide">{t('No secret is entered.')}</span></div>
        {/each}
    </div>
    <form class="lgc-settings-form" onsubmit={save}>
        <label class="lgc-field">
            <span>{t('Name')}</span>
            <input class="hmm-input" type="text" spellcheck="false" autocomplete="off" bind:value={name} placeholder="telegram-token" data-testid="secret-name" />
            <span class="lgc-muted lgc-small">{t('lower-case letters, digits, hyphens')}</span>
        </label>
        <label class="lgc-field">
            <span>{t('Value')}</span>
            <input class="hmm-input" type="password" autocomplete="new-password" bind:value data-testid="secret-value" />
            <span class="lgc-muted lgc-small">{t('is stored on the system and not shown again')}</span>
        </label>
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" type="submit" data-testid="secret-save">{t('Save the secret')}</button>
        </div>
        <p class="lgc-settings-message lgc-cut" role="status" data-testid="secrets-message">
            {#if error}<span class="lgc-bad">{error}</span>{:else if saved}{saved}{:else if data.missing.length > 0}<span class="lgc-bad" title={data.missing.join(', ')}>{t('Programs name secrets that are not entered: {names}', {names: data.missing.join(', ')})}</span>{:else}&nbsp;{/if}
        </p>
    </form>
</section>
<ConfirmDialog open={removing !== ''} title={t('Remove the secret?')} message={t('The secret {name} is removed for good.', {name: removing})} confirmLabel={t('Remove')} danger testId="confirm-secret"
    onconfirm={remove} onclose={() => (removing = '')} />
