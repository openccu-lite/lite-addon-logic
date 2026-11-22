<script lang="ts">
    // A webhook's request in a form of its own, so the editor's row keeps its shape: method,
    // address, headers, a body. The placeholders are a fixed set, listed here; a secret's name
    // must be one of those entered in the settings. Nothing changes before "Apply".
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {hookProblem, MAX_HEADERS, type HookForm} from './editor.js';
    import type {Context} from './model.js';

    let {hook, ctx, secrets, onsave, onclose}: {hook: HookForm; ctx: Context; secrets?: string[]; onsave: (hook: HookForm) => void; onclose: () => void} = $props();

    // svelte-ignore state_referenced_locally
    let form = $state<HookForm>({...hook, headers: hook.headers.map((h) => ({...h}))});
    let tried = $state(false);
    const problem = $derived(hookProblem(form, ctx, secrets));
    const PLACEHOLDERS = ['{program}', '{trigger}', '{value}', '{time}', '{var:…}', '{secret:…}'];

    function save() {
        tried = true;
        if (problem) return;
        onsave({...form, headers: form.headers.filter((h) => h.name.trim() || h.value)});
    }
</script>

<Dialog open title={t('Webhook')} width="46rem" {onclose} testId="webhook-dialog">
    <form class="lgc-hook" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <div class="lgc-hook-row">
            <select class="hmm-select lgc-hook-method" aria-label={t('Method')} bind:value={form.method} data-testid="hook-method">
                {#each ['GET', 'POST', 'PUT'] as m (m)}<option value={m}>{m}</option>{/each}
            </select>
            <input class="hmm-input lgc-hook-url" type="text" inputmode="url" spellcheck="false" autocomplete="off" placeholder="https://…" aria-label={t('Address')} bind:value={form.url} data-testid="hook-url" />
            <HelpLink topic="webhook" />
        </div>
        <label class="lgc-check"><input type="checkbox" bind:checked={form.insecure} data-testid="hook-insecure" /> {t('do not verify the certificate (a device that signs its own)')}</label>

        <div class="lgc-hook-head"><span>{t('Headers')}</span></div>
        {#each form.headers as header, i (i)}
            <div class="lgc-hook-row" data-testid="hook-header">
                <input class="hmm-input lgc-hook-name" type="text" spellcheck="false" autocomplete="off" placeholder={t('Name')} aria-label={t('Header name')} bind:value={header.name} data-testid="hook-header-name" />
                <input class="hmm-input lgc-hook-url" type="text" spellcheck="false" autocomplete="off" placeholder={t('Value')} aria-label={t('Header value')} bind:value={header.value} data-testid="hook-header-value" />
                <button class="hmm-button lgc-ed-icon" type="button" title={t('Remove')} aria-label={t('Remove')} onclick={() => form.headers.splice(i, 1)} data-testid="hook-header-remove">✕</button>
            </div>
        {/each}
        <div><button class="hmm-button" type="button" disabled={form.headers.length >= MAX_HEADERS} onclick={() => form.headers.push({name: '', value: ''})} data-testid="hook-header-add">{t('Add a header')}</button></div>

        <div class="lgc-hook-head">
            <span>{t('Body')}</span>
            <select class="hmm-select" aria-label={t('Kind of body')} bind:value={form.bodyType} disabled={form.method === 'GET'} data-testid="hook-body-type">
                {#each ['json', 'form', 'text'] as kind (kind)}<option value={kind}>{t('body.' + kind)}</option>{/each}
            </select>
        </div>
        <textarea class="hmm-input lgc-hook-body lgc-mono" rows="5" spellcheck="false" aria-label={t('Body')} placeholder={form.method === 'GET' ? t('A GET has no body.') : ''} bind:value={form.body} disabled={form.method === 'GET'} data-testid="hook-body"></textarea>

        <p class="lgc-small lgc-muted" data-testid="hook-placeholders">
            {t('Placeholders, in the address, the headers and the body:')}
            {#each PLACEHOLDERS as p (p)}<code class="lgc-hook-ph">{p}</code>{' '}{/each}
            <HelpLink topic="webhook-placeholders" />
            <br />{t('The answer is never read: a status 2xx counts as done, and the history says the status and how long the call took.')}
        </p>
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="hook-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="hook-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
