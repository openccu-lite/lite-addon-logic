<script lang="ts">
    // The Verlauf page: the run trace of all programs in the order things happened - why a
    // program ran, and why it did not.
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import RunHistory from './RunHistory.svelte';
    import type {Overview} from './model.js';

    let {overview, program = $bindable(''), query = $bindable(''), onopen}: {overview: Overview; program?: string; query?: string; onopen: (key: string) => void} = $props();

    let show = $state('');
    const programs = $derived(overview.programs.map((p) => ({key: p.program.key, name: p.program.name})).sort((a, b) => a.name.localeCompare(b.name)));
</script>

<section class="lgc-section lgc-section-wide">
    <div class="lgc-section-head">
        <h2>{t('History')}</h2>
        <HelpLink topic="trace" />
        <span class="lgc-spacer"></span>
        <select class="hmm-select lgc-run-select" aria-label={t('Program')} bind:value={program} data-testid="run-program-filter">
            <option value="">{t('all programs')}</option>
            {#each programs as p (p.key)}<option value={p.key}>{p.name}</option>{/each}
        </select>
        <select class="hmm-select" aria-label={t('Show')} bind:value={show} data-testid="run-filter">
            <option value="">{t('everything')}</option>
            <option value="acted">{t('only where something ran')}</option>
            <option value="problems">{t('only problems')}</option>
        </select>
        <input class="hmm-input lgc-search" type="search" placeholder={t('Search program, trigger, activity…')} aria-label={t('Search program, trigger, activity…')} bind:value={query} data-testid="run-search" />
    </div>
    {#if program}<p class="lgc-muted lgc-small" data-testid="run-own-note">{t('One program: with the notes when something it looks at reported and did not trigger it.')} <HelpLink topic="trace-not-triggered" /></p>{/if}
    <RunHistory {overview} {program} {query} {show} {onopen} />
</section>
