<script lang="ts">
    // What triggers what: every source with its last event and the programs that listen to it,
    // each with its mode. With a program, only that program's sources and lines (its own view).
    import {i18n, t} from 'occulite-ui/i18n';

    import {get} from './api.js';
    import {context, wallTime, type Overview} from './model.js';
    import {kindText, triggerGroups, type Sources} from './triggers.js';

    let {overview, program = '', query = '', kind = '', onopen}: {overview: Overview; program?: string; query?: string; kind?: string; onopen?: (key: string) => void} = $props();

    let sources = $state<Sources | null>(null);
    // asked again whenever the page's data is
    $effect(() => {
        void overview;
        get<Sources>('api/v1/sources').then(
            (s) => (sources = s),
            () => {},
        );
    });

    const groups = $derived(triggerGroups(overview, sources, context(overview), {query, kind, program}));
</script>

<div class="lgc-trg-list" data-testid="trigger-list">
    {#each groups as g (g.id)}
        {@const last = g.last ? wallTime(g.last, i18n.language) + (g.value ? ' → ' + g.value : '') : g.value}
        <div class="lgc-trg" data-testid="trigger-{g.id}">
            <div class="lgc-trg-head">
                <span class="lgc-pill" data-testid="trigger-kind">{kindText(g.kind)}</span>
                <span class="lgc-trg-title lgc-cut" title={g.title + (g.detail ? ' · ' + g.detail : '')} data-testid="trigger-title"><strong>{g.title}</strong>{#if g.detail} <span class="lgc-muted lgc-small lgc-mono">{g.detail}</span>{/if}</span>
                <span class="lgc-trg-last lgc-cut lgc-small" class:lgc-muted={!last} title={(g.kind === 'time' ? '' : t('last') + ': ' + (last || '–')) + (g.every ? ' · ' + g.every : '')} data-testid="trigger-last">
                    {#if g.kind !== 'time'}{t('last')}: {last || '–'}{/if}{#if g.every} · {g.every}{/if}
                </span>
            </div>
            {#each g.listeners as l (l.programKey + l.label)}
                <div class="lgc-trg-row" class:lgc-trg-off={!l.active} data-testid="trigger-listener">
                    <span class="lgc-line-mark" title={l.triggers ? t('This line triggers the program.') : ''}>{l.triggers ? '⚡' : ''}</span>
                    <span class="lgc-trg-mode lgc-cut" title={l.label}>{l.label}</span>
                    {#if onopen}
                        <button class="lgc-linklike lgc-cut" title={l.program} onclick={() => onopen(l.programKey)} data-testid="trigger-program">{l.program}</button>
                    {:else}
                        <span class="lgc-cut" title={l.program} data-testid="trigger-program">{l.program}</span>
                    {/if}
                    <span class="lgc-trg-note lgc-cut lgc-small" title={l.warn || (l.active ? '' : t('the program is inactive'))}>{#if l.warn}⚠ {l.warn}{:else if !l.active}{t('inactive')}{/if}</span>
                </div>
            {/each}
        </div>
    {/each}
    {#if groups.length === 0}
        <p class="lgc-muted" data-testid="trigger-empty">{overview.programs.length === 0 || program ? t('No program looks at anything yet.') : t('Nothing matches.')}</p>
    {/if}
</div>
