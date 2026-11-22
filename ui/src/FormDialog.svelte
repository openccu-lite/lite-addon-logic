<script lang="ts">
    // The form of a "channel" activity on a channel that has one - a signal light, a sound, a
    // siren, a coloured light, a blind with slats: colours to click, shares as sliders, the
    // device's own lists in words. It edits the same values as the plain form ("All
    // datapoints…" opens that one with what is set here); nothing changes before "Apply".
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import ValueField from './ValueField.svelte';
    import {valuesProblem, valuesText, type Env, type ValueKind} from './editor.js';
    import {entryLabel, formFields, formProblem, formState, formValues, hueColor, SWATCHES, type FormField, type FormKind, type Stored} from './forms.js';

    let {channel, name, kind, values, env, onsave, onall, onclose}: {
        channel: string;
        /** The channel as the row shows it. */
        name: string;
        kind: FormKind;
        values: Stored[];
        env: Env;
        onsave: (values: Stored[]) => void;
        /** The plain form is asked for, with the values as they are here. */
        onall: (values: Stored[]) => void;
        onclose: () => void;
    } = $props();

    // svelte-ignore state_referenced_locally
    const fields = formFields(kind, env.params[channel] ?? []);
    // svelte-ignore state_referenced_locally
    const initial = formState(fields, values);
    let form = $state(initial.state);
    const rest = initial.rest;
    let tried = $state(false);

    const result = $derived(formValues(fields, form, rest));
    const wrong = $derived(formProblem(fields, form));
    const problem = $derived(wrong?.text ?? valuesProblem(channel, result, env));

    const TITLES: Record<FormKind, () => string> = {
        signal: () => t('Signal light {channel}', {channel: name}),
        sound: () => t('Sound of {channel}', {channel: name}),
        siren: () => t('Siren {channel}', {channel: name}),
        light: () => t('Coloured light {channel}', {channel: name}),
        blind: () => t('Blind {channel}', {channel: name}),
    };

    const choices = (f: FormField, i: number): ValueKind => ({kind: 'choice', options: (f.defs[i]!.values ?? []).filter((x) => x !== '').map((x) => ({value: x, label: entryLabel(x)}))});
    const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);

    function toggle(f: FormField, on: boolean) {
        form[f.id]!.on = on;
        if (on) for (const id of f.excludes) if (form[id]) form[id].on = false;
    }

    function save() {
        tried = true;
        if (problem) return;
        onsave(result);
    }
</script>

<Dialog open title={TITLES[kind]()} width="34rem" {onclose} testId="form-dialog">
    <form class="lgc-cf" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <p class="lgc-small lgc-muted lgc-vals-intro">
            {t('What is set here goes to the device together, in one command.')}
            <HelpLink topic="forms" />
        </p>
        {#each fields as f (f.id)}
            {@const s = form[f.id]!}
            <div class="lgc-cf-row" class:lgc-cf-tall={f.type === 'hue' || f.type === 'swatch'} data-testid="field-{f.id}">
                <label class="lgc-cf-label lgc-cut" title={`${f.label} (${f.defs.map((d) => d.id).join(", ")})`}>
                    {#if f.optional}<input type="checkbox" checked={s.on} onchange={(e) => toggle(f, e.currentTarget.checked)} data-testid="field-on" />{/if}
                    <span>{f.label}</span>
                </label>
                <div class="lgc-cf-field" class:lgc-vals-off={!s.on}>
                    {#if f.type === 'swatch'}
                        <div class="lgc-swatches" role="radiogroup" aria-label={f.label}>
                            {#each (f.defs[0]!.values ?? []).filter((x) => x !== '') as x (x)}
                                {#if SWATCHES[x]}
                                    <button type="button" class="lgc-swatch" class:lgc-swatch-on={s.a === x} style:background={SWATCHES[x]} role="radio" aria-checked={s.a === x} title={entryLabel(x)} aria-label={entryLabel(x)} onclick={() => (s.a = x)} data-testid="swatch-{x}"></button>
                                {:else}
                                    <button type="button" class="lgc-swatch lgc-swatch-word lgc-small" class:lgc-swatch-on={s.a === x} role="radio" aria-checked={s.a === x} title={x} onclick={() => (s.a = x)} data-testid="swatch-{x}">{entryLabel(x)}</button>
                                {/if}
                            {/each}
                        </div>
                    {:else if f.type === 'choice'}
                        <ValueField bind:value={s.a} kind={choices(f, 0)} label={f.label} testId="field-value" />
                    {:else if f.type === 'percent'}
                        <input class="lgc-cf-range" type="range" min="0" max="100" step="1" aria-label={f.label} value={num(s.a)} oninput={(e) => (s.a = Number(e.currentTarget.value))} data-testid="field-range" />
                        <ValueField bind:value={s.a} kind={{kind: 'number', unit: '%'}} label={f.label} testId="field-value" invalid={tried && wrong?.field === f.id} />
                    {:else if f.type === 'number'}
                        <ValueField bind:value={s.a} kind={{kind: 'number', unit: f.defs[0]!.unit}} label={f.label} testId="field-value" invalid={tried && wrong?.field === f.id} />
                    {:else if f.type === 'span'}
                        <ValueField bind:value={s.a} kind={{kind: 'number'}} label={f.label} testId="field-value" invalid={tried && wrong?.field === f.id} />
                        <ValueField bind:value={s.b} kind={choices(f, 1)} label={t('Unit')} testId="field-unit" />
                    {:else}
                        <div class="lgc-cf-hue">
                            <span class="lgc-swatch lgc-swatch-show" style:background={hueColor(num(s.a), num(s.b))} data-testid="field-colour"></span>
                            <input class="lgc-cf-range lgc-cf-rainbow" type="range" min="0" max="360" step="1" aria-label={t('Hue')} value={num(s.a)} oninput={(e) => (s.a = Number(e.currentTarget.value))} data-testid="field-range" />
                            <ValueField bind:value={s.a} kind={{kind: 'number', unit: '°'}} label={t('Hue')} testId="field-value" invalid={tried && wrong?.field === f.id} />
                            <span class="lgc-small lgc-muted lgc-cut">{t('Saturation')}</span>
                            <input class="lgc-cf-range" type="range" min="0" max="100" step="1" aria-label={t('Saturation')} value={num(s.b)} oninput={(e) => (s.b = Number(e.currentTarget.value))} data-testid="field-sat-range" />
                            <ValueField bind:value={s.b} kind={{kind: 'number', unit: '%'}} label={t('Saturation')} testId="field-sat" invalid={tried && wrong?.field === f.id} />
                        </div>
                    {/if}
                </div>
            </div>
        {/each}
        <div class="lgc-small lgc-muted lgc-cut lgc-reserve" title={valuesText(rest)} data-testid="form-rest">{rest.length > 0 ? t('Also sent: {values}', {values: valuesText(rest)}) : ''}</div>
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="form-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button lgc-cf-all" onclick={() => onall(result)} data-testid="form-all">{t('All datapoints…')}</button>
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="form-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
