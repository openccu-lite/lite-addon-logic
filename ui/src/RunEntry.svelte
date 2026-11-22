<script lang="ts">
    // One entry of the run trace: a row of one height - when, which program, what caused it,
    // what became of it - that opens to the condition lines as the run saw them and to what
    // each activity did. The History list shows these; the dry run shows the one it was answered.
    import {t} from 'occulite-ui/i18n';

    import type {RunView} from './trace.js';

    let {v, own = false, open, ontoggle, onopen}: {v: RunView; /** the program is not named: the list is one program's */ own?: boolean; open: boolean; ontoggle?: () => void; onopen?: (key: string) => void} = $props();
</script>

<div class="lgc-run" class:lgc-run-open={open} class:lgc-run-own={own} data-testid="run">
    <button class="lgc-run-head" aria-expanded={open} onclick={() => ontoggle?.()} disabled={!ontoggle} data-testid="run-head">
        <span class="lgc-run-caret" aria-hidden="true">{ontoggle ? (open ? '▾' : '▸') : ''}</span>
        <span class="lgc-run-time lgc-cut" title={v.time} data-testid="run-time">{v.time}</span>
        {#if !own}<span class="lgc-run-program lgc-cut" title={v.program} data-testid="run-program">{v.program}</span>{/if}
        <span class="lgc-run-trigger lgc-cut" title={v.trigger} data-testid="run-trigger">⚡ {v.trigger}</span>
        <span class="lgc-run-outcome"><span class={'lgc-pill ' + (v.kind === 'ok' ? 'ok' : v.kind)} title={v.outcome} data-testid="run-outcome">{v.outcome}</span></span>
    </button>
    {#if open}
        <div class="lgc-run-body" data-testid="run-body">
            {#if v.detail}<p class="lgc-muted lgc-small lgc-run-detail" data-testid="run-detail">{v.detail}</p>{/if}
            {#each v.lines as l (l.n)}
                <div class="lgc-run-line" class:lgc-run-skip={l.kind === 'skip'} data-testid="run-line">
                    <span class="lgc-line-mark" title={l.mark ? t('This line triggers the program.') : ''}>{l.mark ? '⚡' : ''}</span>
                    <span class="lgc-run-text lgc-cut" title={l.text}>{l.text}</span>
                    <span class="lgc-run-value lgc-cut" title={l.value + (l.hint ? ' · ' + l.hint : '')}>{l.value}{#if l.hint} <span class="lgc-muted lgc-small">{l.hint}</span>{/if}</span>
                    <span class="lgc-run-verdict"><span class={'lgc-pill ' + (l.kind === 'ok' ? 'ok' : '')} data-testid="run-verdict">{l.verdict}</span></span>
                </div>
            {/each}
            {#each v.steps as s, si (si)}
                <div class="lgc-run-step" class:lgc-run-bad={s.kind === 'bad'} data-testid="run-step">
                    <span class="lgc-line-mark">{s.n}</span>
                    <span class="lgc-run-text lgc-cut" title={s.text}>{s.text}</span>
                    <span class="lgc-run-status lgc-cut" title={s.status} data-testid="run-status">{s.status}</span>
                </div>
            {/each}
            {#if onopen}
                <button class="lgc-linklike lgc-small" onclick={() => onopen(v.programKey)} data-testid="run-open-program">{t('Open the program')}</button>
            {/if}
        </div>
    {/if}
</div>
