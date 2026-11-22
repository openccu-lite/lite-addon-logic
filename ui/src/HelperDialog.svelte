<script lang="ts">
    // Add a device helper: a channel of this system that has a counter - a measuring actuator,
    // a weather sensor, a water meter - and the name its variables are called by. The helper
    // counts from now; the total takes the meter's reading afterwards, like any value.
    import {Dialog} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {ApiError, get, post} from './api.js';
    import {baseName, candidateId, candidateLabel, hasDays, helperNames, helperProblem, ROLES, type HelperList} from './helpers.js';

    let {onclose, oncreated}: {onclose: () => void; oncreated: () => void} = $props();

    let list = $state<HelperList | null>(null);
    let loadError = $state('');
    let chosen = $state('');
    let name = $state('');
    let days = $state(true);
    let tried = $state(false);
    let error = $state('');
    let busy = $state(false);

    const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : String(e));

    $effect(() => {
        get<HelperList>(`api/v1/helpers?lang=${i18n.language}`).then(
            (l) => {
                list = l;
                if (l.candidates[0]) choose(candidateId(l.candidates[0]));
            },
            (e) => (loadError = message(e)),
        );
    });

    const candidate = $derived(list?.candidates.find((c) => candidateId(c) === chosen));
    const names = $derived(candidate ? helperNames(candidate, name, days) : {});
    const problem = $derived(helperProblem(candidate, name));
    // what the place below the fields says: why nothing can be chosen, or nothing
    const hint = $derived(
        loadError ? loadError : !list ? t('Asking the system for its devices…') : !list.system ? t('The devices of the system cannot be asked right now.') : list.candidates.length === 0 ? t('No device of this system has a counter without a helper.') : '',
    );

    function choose(id: string) {
        chosen = id;
        const c = list?.candidates.find((x) => candidateId(x) === id);
        if (c) name = baseName(c);
    }

    async function save() {
        tried = true;
        if (problem || !candidate) return;
        busy = true;
        try {
            await post('api/v1/helpers', {kind: candidate.kind, channel: candidate.channel, names});
            oncreated();
        } catch (e) {
            error = message(e);
        }
        busy = false;
    }
</script>

<Dialog open title={t('Add a device helper')} width="38rem" height="30rem" {onclose} testId="helper-dialog">
    <form class="lgc-form" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <p class="lgc-span lgc-help lgc-small">{t('A device helper keeps a counter of a device as system variables: the total, and what was added today and yesterday. It counts from now on.')} <HelpLink topic="helpers" /></p>
        <label class="lgc-label" for="helper-device">{t('Device')}</label>
        <div>
            <select id="helper-device" class="hmm-select lgc-hlp-device" value={chosen} onchange={(e) => choose(e.currentTarget.value)} disabled={!list || list.candidates.length === 0} data-testid="helper-device">
                {#if !candidate}<option value="">–</option>{/if}
                {#each list?.candidates ?? [] as c (candidateId(c))}<option value={candidateId(c)}>{candidateLabel(c)}</option>{/each}
            </select>
            <div class="lgc-help lgc-small lgc-reserve" data-testid="helper-state">{hint}</div>
        </div>
        <label class="lgc-label" for="helper-name">{t('Name')}</label>
        <div>
            <input id="helper-name" class="hmm-input" type="text" bind:value={name} disabled={!candidate} data-testid="helper-name" />
            <div class="lgc-error-inline lgc-small lgc-reserve" data-testid="helper-name-error">{tried && candidate ? problem : ''}</div>
        </div>
        <span class="lgc-label">{t('Day values')}</span>
        <label class="lgc-check"><input type="checkbox" bind:checked={days} disabled={!candidate || !hasDays(candidate)} data-testid="helper-days" /> {t('also keep what was added today and yesterday')}</label>
        <span class="lgc-label">{t('Variables')}</span>
        <!-- always three lines high: the list does not move the buttons when the day values go -->
        <ul class="lgc-hlp-names" data-testid="helper-names">
            {#each ROLES as role (role)}
                <li class="lgc-cut" title={names[role] ?? ''}>{#if names[role]}<span class="lgc-muted">{t('role.' + role)}:</span> {names[role]}{candidate?.unit ? ' (' + candidate.unit + ')' : ''}{/if}</li>
            {/each}
        </ul>
        <div class="lgc-span lgc-error-inline lgc-reserve" role="alert" data-testid="helper-error">{error}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose} data-testid="helper-cancel">{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} disabled={busy || !candidate} data-testid="helper-save">{t('Add')}</button>
    {/snippet}
</Dialog>
