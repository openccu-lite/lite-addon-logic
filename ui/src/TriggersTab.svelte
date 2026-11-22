<script lang="ts">
    // The triggers page: what triggers what, from the source's side - the CCU's "Programme"
    // button of a variable or a channel, for all of them at once, with a search.
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import TriggerList from './TriggerList.svelte';
    import type {Overview} from './model.js';

    let {overview, query = $bindable(''), onopen}: {overview: Overview; query?: string; onopen: (key: string) => void} = $props();

    let kind = $state('');
</script>

<section class="lgc-section lgc-section-wide">
    <div class="lgc-section-head">
        <h2>{t('Triggers')}</h2>
        <HelpLink topic="triggers-page" />
        <span class="lgc-spacer"></span>
        <select class="hmm-select" aria-label={t('Show')} bind:value={kind} data-testid="trigger-filter">
            <option value="">{t('everything')}</option>
            <option value="variable">{t('variables only')}</option>
            <option value="device">{t('devices only')}</option>
            <option value="time">{t('time modules only')}</option>
        </select>
        <input class="hmm-input lgc-search" type="search" placeholder={t('Search device, variable, program…')} aria-label={t('Search device, variable, program…')} bind:value={query} data-testid="trigger-search" />
    </div>
    <TriggerList {overview} {query} {kind} {onopen} />
</section>
