<script lang="ts">
    // The values a "channel" activity writes in one command: the channel's writable datapoints
    // as the system describes them, each with a tick and a field of its kind. A datapoint the
    // channel does not (or no longer) have stays listed until it is unticked, and is said. For
    // a channel the system does not know the datapoints are typed by name. Nothing changes
    // before "Apply".
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import ValueField from './ValueField.svelte';
    import {firstValue, MAX_VALUES, namedValue, valuesKind, valuesProblem, type Env, type Param} from './editor.js';
    import type {Value} from './model.js';

    let {channel, name, values, env, loading = false, onsave, onform, onclose}: {
        channel: string;
        /** The channel as the row shows it. */
        name: string;
        values: {param: string; value: Value}[];
        env: Env;
        /** The channel's datapoints are being asked for. */
        loading?: boolean;
        onsave: (values: {param: string; value: Value}[]) => void;
        /** The channel has a form: asked for with the values as they are here. */
        onform?: (values: {param: string; value: Value}[]) => void;
        onclose: () => void;
    } = $props();

    interface Row {
        param: string;
        on: boolean;
        value: Value;
        /** The channel's description of it; none for one it does not have. */
        def?: Param;
    }

    const known = $derived(env.params[channel]);
    // svelte-ignore state_referenced_locally
    let stored = $state(values.map((v) => ({...v})));
    let rows = $state<Row[]>([]);
    let free = $state<{param: string; text: string}[]>([]);
    let tried = $state(false);
    let built = '';

    // the rows are made once the channel's datapoints are known: the writable ones in their
    // order, then what is stored and not among them
    $effect(() => {
        const sig = known ? 'known' : 'free';
        if (built === sig || (!known && loading)) return;
        built = sig;
        if (!known) {
            free = stored.map((v) => ({param: v.param, text: v.value === undefined || v.value === null ? '' : String(v.value)}));
            return;
        }
        const writable = known.filter((p) => p.write);
        rows = [
            ...writable.map((p) => {
                const have = stored.find((v) => v.param === p.id);
                return {param: p.id, on: !!have, value: have ? namedValue(p, have.value) : firstValue(valuesKind(p)), def: p};
            }),
            ...stored.filter((v) => !writable.some((p) => p.id === v.param)).map((v) => ({param: v.param, on: true, value: v.value})),
        ];
    });

    const freeValue = (text: string): Value => {
        const s = text.trim();
        if (s === 'true' || s === 'false') return s === 'true';
        return s !== '' && !Number.isNaN(Number(s.replace(',', '.'))) ? Number(s.replace(',', '.')) : text;
    };
    const result = $derived(known ? rows.filter((r) => r.on).map((r) => ({param: r.param, value: r.value})) : free.filter((f) => f.param.trim()).map((f) => ({param: f.param.trim(), value: freeValue(f.text)})));
    const problem = $derived(valuesProblem(channel, result, env));

    function save() {
        tried = true;
        if (problem) return;
        onsave(result);
    }
</script>

<Dialog open title={t('Values of {channel}', {channel: name})} width="42rem" {onclose} testId="values-dialog">
    <form class="lgc-vals" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <p class="lgc-small lgc-muted lgc-vals-intro">
            {known ? t('Tick the datapoints to write: they go to the device together, in one command.') : t('The system does not know this channel: name the datapoints and their values. They are checked when the command is sent.')}
            <HelpLink topic="values" />
        </p>
        {#if !known && loading}
            <div class="lgc-muted" data-testid="values-loading">{t('Loading…')}</div>
        {:else if known}
            <div class="lgc-vals-list" role="list">
                {#each rows as row (row.param)}
                    <div class="lgc-vals-row" class:lgc-vals-gone={!row.def} role="listitem" data-testid="value-{row.param}">
                        <label class="lgc-check lgc-vals-name lgc-cut" title={row.param}><input type="checkbox" bind:checked={row.on} data-testid="value-on" /> <span class="lgc-mono">{row.param}</span></label>
                        {#if row.def}
                            <span class="lgc-vals-field" class:lgc-vals-off={!row.on}><ValueField bind:value={row.value} kind={valuesKind(row.def)} label={row.param} testId="value-field" invalid={tried && row.on && problem.startsWith(row.param + ':')} /></span>
                            <span class="lgc-small lgc-muted lgc-cut lgc-vals-range" title={row.def.min !== undefined && row.def.max !== undefined ? `${row.def.min} … ${row.def.max}` : ''}>{row.def.min !== undefined && row.def.max !== undefined && (row.def.type === 'FLOAT' || row.def.type === 'INTEGER') ? `${row.def.min} … ${row.def.max}` : ''}</span>
                        {:else}
                            <span class="lgc-vals-field lgc-small lgc-error-inline lgc-cut" title={t('The channel has no such datapoint to write: untick it.')} data-testid="value-gone">{t('The channel has no such datapoint to write: untick it.')}</span>
                            <span></span>
                        {/if}
                    </div>
                {:else}
                    <div class="lgc-vals-row lgc-muted" role="listitem" data-testid="values-none">{t('This channel has no datapoint that can be written.')}</div>
                {/each}
            </div>
        {:else}
            {#each free as f, i (i)}
                <div class="lgc-hook-row" data-testid="value-free">
                    <input class="hmm-input lgc-hook-name lgc-mono" type="text" spellcheck="false" autocomplete="off" placeholder={t('Datapoint')} aria-label={t('Datapoint')} bind:value={f.param} data-testid="value-free-name" />
                    <input class="hmm-input lgc-hook-url" type="text" spellcheck="false" autocomplete="off" placeholder={t('Value')} aria-label={t('Value')} bind:value={f.text} data-testid="value-free-value" />
                    <button class="hmm-button lgc-ed-icon" type="button" title={t('Remove')} aria-label={t('Remove')} onclick={() => free.splice(i, 1)} data-testid="value-free-remove">✕</button>
                </div>
            {/each}
            <div><button class="hmm-button" type="button" disabled={free.length >= MAX_VALUES} onclick={() => free.push({param: '', text: ''})} data-testid="value-free-add">{t('Add a datapoint')}</button></div>
        {/if}
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="values-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        {#if onform}<button class="hmm-button lgc-cf-all" onclick={() => onform(result)} data-testid="values-form">{t('Form…')}</button>{/if}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="values-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
