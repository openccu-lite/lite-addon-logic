<script lang="ts">
    // The versions of a program or a variable: every history entry that changed its file, what
    // an entry changed or how it differs from now, line by line - and the way back to it.
    import {Dialog} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import {ApiError, get, post} from './api.js';
    import {dateTime} from './model.js';
    import {collapse, diffStats, lineDiff, messageText, shortHash, type VersionList} from './versions.js';

    let {kind, key, name, onclose, onrestored}: {kind: 'programs' | 'variables'; key: string; name: string; onclose: () => void; onrestored: () => void} = $props();

    let list = $state<VersionList | null>(null);
    let chosen = $state('');
    // 'change': what the chosen version changed (against the one before it); 'now': how it differs from the file now
    let mode = $state<'change' | 'now'>('change');
    // the versions' texts, by hash ('' for a version that does not hold the file)
    let texts = $state<Record<string, string>>({});
    let error = $state('');
    let asking = $state(false);
    let busy = $state(false);

    const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : String(e));

    async function text(hash: string): Promise<void> {
        if (hash in texts) return;
        try {
            texts[hash] = (await get<{text: string}>(`api/v1/${kind}/${key}/versions/${hash}`)).text;
        } catch {
            texts[hash] = '';
        }
    }

    async function load() {
        try {
            list = await get<VersionList>(`api/v1/${kind}/${key}/versions`);
            error = '';
            if (!list.versions.some((v) => v.hash === chosen)) chosen = list.versions[0]?.hash ?? '';
        } catch (e) {
            error = message(e);
        }
    }
    $effect(() => {
        void kind;
        void key;
        void load();
    });

    const index = $derived(list?.versions.findIndex((v) => v.hash === chosen) ?? -1);
    const before = $derived(index >= 0 ? (list?.versions[index + 1]?.hash ?? '') : '');
    // the texts the comparison needs
    $effect(() => {
        if (chosen) void text(chosen);
        if (before) void text(before);
    });
    const ready = $derived(chosen !== '' && chosen in texts && (mode === 'now' || before === '' || before in texts));
    const diff = $derived(!ready ? [] : mode === 'now' ? lineDiff(texts[chosen]!, list?.current ?? '') : lineDiff(before ? texts[before]! : '', texts[chosen]!));
    const stats = $derived(diffStats(diff));
    const shown = $derived(collapse(diff));
    // the version is what the file is now: nothing to go back to
    const same = $derived(ready && texts[chosen] === (list?.current ?? ''));

    async function restore() {
        asking = false;
        busy = true;
        try {
            await post(`api/v1/${kind}/${key}/versions/${chosen}/restore`);
            chosen = '';
            await load();
            onrestored();
        } catch (e) {
            error = message(e);
        }
        busy = false;
    }
</script>

<Dialog open title={t('Versions: {name}', {name})} width="64rem" height="38rem" {onclose} testId="versions-dialog">
    <p class="lgc-muted lgc-small">{t('Every save is a version. A version can be compared and brought back; bringing one back is a new version, nothing is lost.')} <HelpLink topic="versions" /></p>
    <div class="lgc-ver">
        <div class="lgc-ver-list" data-testid="version-list">
            {#each list?.versions ?? [] as v, i (v.hash)}
                <button class="lgc-ver-row" class:lgc-ver-chosen={v.hash === chosen} onclick={() => (chosen = v.hash)} aria-pressed={v.hash === chosen} data-testid="version">
                    <span class="lgc-cut" title={dateTime(v.time, i18n.language)} data-testid="version-time">{dateTime(v.time, i18n.language)}{i === 0 ? ' · ' + t('now') : ''}</span>
                    <span class="lgc-cut lgc-small" title={messageText(v.message) + ' · ' + v.author} data-testid="version-message">{messageText(v.message)}</span>
                    <span class="lgc-cut lgc-small lgc-muted" title={v.author + ' · ' + shortHash(v.hash)}>{v.author} · <span class="lgc-mono">{shortHash(v.hash)}</span></span>
                </button>
            {/each}
            {#if list && list.versions.length === 0}<p class="lgc-muted" data-testid="version-none">{t('The history has no entry for this yet.')}</p>{/if}
        </div>
        <div class="lgc-ver-side">
            <div class="lgc-ver-bar">
                <select class="hmm-select" aria-label={t('Compare')} bind:value={mode} data-testid="version-mode">
                    <option value="change">{t('what this version changed')}</option>
                    <option value="now">{t('from this version to now')}</option>
                </select>
                <span class="lgc-ver-stats lgc-small lgc-muted lgc-cut" data-testid="version-stats">{ready ? t('+{added} −{removed} lines', stats) : ''}</span>
                <button class="hmm-button" onclick={() => (asking = true)} disabled={busy || !ready || same || texts[chosen] === ''} title={same ? t('This version is what is there now.') : ''} data-testid="version-restore">{t('Restore this version')}</button>
            </div>
            <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="version-error">{error}</div>
            <div class="lgc-ver-diff lgc-mono" data-testid="version-diff">
                {#each shown as l, i (i)}
                    {#if l.kind === '…'}
                        <div class="lgc-diff-gap lgc-muted">{t('{n} unchanged lines', {n: l.n ?? 0})}</div>
                    {:else}
                        <div class="lgc-diff-line" class:lgc-diff-add={l.kind === '+'} class:lgc-diff-del={l.kind === '-'} data-testid="diff-line"><span class="lgc-diff-mark" aria-hidden="true">{l.kind}</span><span class="lgc-diff-text">{l.text}</span></div>
                    {/if}
                {/each}
                {#if ready && diff.length > 0 && stats.added + stats.removed === 0}<p class="lgc-muted" data-testid="version-same">{t('No difference.')}</p>{/if}
            </div>
        </div>
    </div>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose} data-testid="versions-close">{t('Close')}</button>
    {/snippet}
</Dialog>
<ConfirmDialog open={asking} title={t('Restore this version?')} message={t('{name} becomes what it was in this version. What is there now stays in the history.', {name})} confirmLabel={t('Restore')} testId="confirm-restore"
    onconfirm={restore} onclose={() => (asking = false)} />
