<script lang="ts">
    // Create or change a system variable: name, type and what the type needs. "Computed" is a
    // number with a function and its inputs - datapoints chosen in the device picker, or other
    // number variables.
    import {Dialog} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';

    import DevicePicker from './DevicePicker.svelte';
    import HelpLink from './HelpLink.svelte';
    import {get} from './api.js';
    import {channelNames, type Device, type Param} from './editor.js';
    import {COMPUTED_FUNCTIONS, defaultValue, defOf, emptyForm, emptyInput, fitInputs, formOf, formProblems, inputCount, inputLabel, variableName, type FormInput, type Value, type VariableDef, type VariableView} from './model.js';

    let {
        view,
        variables = [],
        onsave,
        onclose,
        onversions,
    }: {
        view: VariableView | null;
        /** The variables there are: a computed value's inputs are chosen among the numbers. */
        variables?: VariableView[];
        /** Opens the versions of the variable being changed. */
        onversions?: () => void;
        onsave: (body: {variable: VariableDef; renames: Record<string, string>; value?: Value}) => Promise<string>;
        onclose: () => void;
    } = $props();

    // svelte-ignore state_referenced_locally
    let form = $state(view ? formOf(view.def) : emptyForm());
    let tried = $state(false);
    let error = $state('');
    let busy = $state(false);

    const isNew = $derived(view === null);
    const problems = $derived(formProblems(form));
    // a type that programs use cannot be changed: the CCU lets it and breaks the programs
    const typeLocked = $derived(!!view && (view.users.length > 0 || !!view.def.helper));

    // --- a computed value's inputs ---
    let devices = $state<Device[]>([]);
    let devicesLoading = $state(false);
    let devicesError = $state('');
    let devicesAsked = false;
    let params = $state<Record<string, Param[]>>({});
    const asked = new Set<string>();
    let picking = $state<FormInput | null>(null);

    const names = $derived(channelNames(devices));
    const numbers = $derived(variables.filter((v) => v.def.type === 'number' && v.def.key !== form.key).sort((a, b) => variableName(a.def).localeCompare(variableName(b.def))));
    const channelLabel = (channel: string) => (channel ? (names[channel] ?? channel.split('/')[1] + ' (' + channel.split('/')[0] + ')') : t('Choose a device…'));
    const numeric = (channel: string) => (params[channel] ?? []).filter((p) => p.read && (p.type === 'FLOAT' || p.type === 'INTEGER'));

    function loadDevices() {
        if (devicesAsked) return;
        devicesAsked = true;
        devicesLoading = true;
        get<{devices: Device[]}>(`api/v1/devices?lang=${i18n.language}`).then(
            (r) => {
                devices = r.devices;
                devicesLoading = false;
            },
            (e: Error) => {
                devicesError = e.message;
                devicesLoading = false;
            },
        );
    }

    // the names of the devices and the datapoints of the channels the inputs name, asked once each
    $effect(() => {
        if (form.type !== 'computed') return;
        loadDevices();
        const missing = form.inputs.filter((in_) => in_.kind === 'device' && in_.channel && !asked.has(in_.channel)).map((in_) => in_.channel);
        if (missing.length === 0) return;
        for (const ch of missing) asked.add(ch);
        get<{channels: Record<string, Param[]>}>('api/v1/datapoints?' + missing.map((ch) => 'channel=' + encodeURIComponent(ch)).join('&')).then(
            (r) => {
                params = {...params, ...r.channels};
                // a channel chosen a moment ago gets a datapoint: the one the function's input asks for, or the first
                form.inputs.forEach((in_, i) => {
                    if (in_.kind === 'device' && !in_.param && missing.includes(in_.channel)) settleInput(in_, i);
                });
            },
            () => {},
        );
    });

    // a channel's datapoint for an input: the one the function's input asks for, or the first number
    function settleInput(input: FormInput, index: number) {
        const list = numeric(input.channel);
        const wanted = ['dew-point', 'absolute-humidity'].includes(form.fn) ? (index === 0 ? ['ACTUAL_TEMPERATURE', 'TEMPERATURE'] : ['HUMIDITY']) : [];
        input.param = (wanted.map((id) => list.find((p) => p.id === id)).find(Boolean) ?? list[0])?.id ?? '';
    }

    function pick(channel: string) {
        const input = picking;
        picking = null;
        if (!input) return;
        Object.assign(input, {channel, param: ''});
        if (channel in params) settleInput(input, form.inputs.indexOf(input));
    }

    function setFunction(fn: string) {
        form.fn = fn;
        form.inputs = fitInputs(fn, form.inputs);
    }

    async function save() {
        tried = true;
        if (Object.keys(problems).length > 0) return;
        busy = true;
        const body = defOf(form);
        error = await onsave(isNew ? {...body, value: defaultValue(form.type, body.variable.entries ?? [])} : body);
        busy = false;
    }
</script>

<Dialog open title={isNew ? t('New variable') : t('Edit the variable')} width="38rem" height="40rem" {onclose} testId="variable-dialog">
    <form class="lgc-form" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <label class="lgc-label" for="var-name">{t('Name')}</label>
        <div>
            <input id="var-name" class="hmm-input" type="text" bind:value={form.name} data-testid="var-name" />
            <div class="lgc-error-inline lgc-small lgc-reserve" data-testid="var-name-error">{tried ? (problems.name ?? '') : ''}</div>
        </div>
        <label class="lgc-label" for="var-note">{t('Description')}</label>
        <input id="var-note" class="hmm-input" type="text" bind:value={form.note} data-testid="var-note" />
        <label class="lgc-label" for="var-type">{t('Type')} <HelpLink topic="variable-types" /></label>
        <div>
            <select id="var-type" class="hmm-select" bind:value={form.type} disabled={typeLocked} data-testid="var-type">
                {#each ['bool', 'list', 'number', 'string', 'alarm', 'computed'] as type (type)}<option value={type}>{t('type.' + type)}</option>{/each}
            </select>
            <div class="lgc-help lgc-small lgc-reserve">{form.helper ? t('A device helper keeps this variable: its type cannot be changed.') : typeLocked ? t('Programs use this variable: its type cannot be changed.') : ''}</div>
        </div>

        {#if form.type === 'bool' || form.type === 'alarm'}
            <label class="lgc-label" for="var-true">{t('Text for true')}</label>
            <input id="var-true" class="hmm-input" type="text" bind:value={form.labelTrue} placeholder={t('true')} data-testid="var-label-true" />
            <label class="lgc-label" for="var-false">{t('Text for false')}</label>
            <input id="var-false" class="hmm-input" type="text" bind:value={form.labelFalse} placeholder={t('false')} data-testid="var-label-false" />
        {:else if form.type === 'list'}
            <span class="lgc-label">{t('Entries')}</span>
            <div class="lgc-entries">
                {#each form.entries as entry, i (i)}
                    <div class="lgc-row">
                        <input class="hmm-input" type="text" bind:value={entry.label} aria-label={t('Entry {n}', {n: i + 1})} data-testid="var-entry" />
                        <button class="hmm-button" type="button" onclick={() => form.entries.splice(i, 1)} disabled={form.entries.length < 2} aria-label={t('Remove the entry')} data-testid="var-entry-remove">✕</button>
                    </div>
                {/each}
                <button class="hmm-button" type="button" onclick={() => form.entries.push({label: '', original: ''})} data-testid="var-entry-add">{t('Add an entry')}</button>
                <div class="lgc-help lgc-small">{t('An entry renamed here is renamed in the programs that use it; one that is in use cannot be removed.')}</div>
                <div class="lgc-error-inline lgc-small lgc-reserve" data-testid="var-entries-error">{tried ? (problems.entries ?? '') : ''}</div>
            </div>
        {:else if form.type === 'number'}
            <label class="lgc-label" for="var-unit">{t('Unit')}</label>
            <input id="var-unit" class="hmm-input lgc-narrow" type="text" bind:value={form.unit} data-testid="var-unit" />
            <label class="lgc-label" for="var-min">{t('Minimum')} <HelpLink topic="variable-limits" /></label>
            <div>
                <input id="var-min" class="hmm-input lgc-narrow" type="text" inputmode="decimal" bind:value={form.min} data-testid="var-min" />
                <div class="lgc-error-inline lgc-small lgc-reserve">{tried ? (problems.min ?? '') : ''}</div>
            </div>
            <label class="lgc-label" for="var-max">{t('Maximum')}</label>
            <div>
                <input id="var-max" class="hmm-input lgc-narrow" type="text" inputmode="decimal" bind:value={form.max} data-testid="var-max" />
                <div class="lgc-error-inline lgc-small lgc-reserve" data-testid="var-max-error">{tried ? (problems.max ?? '') : ''}</div>
            </div>
        {:else if form.type === 'computed'}
            <label class="lgc-label" for="var-function">{t('Function')} <HelpLink topic="computed" /></label>
            <select id="var-function" class="hmm-select" value={form.fn} onchange={(e) => setFunction(e.currentTarget.value)} data-testid="var-function">
                {#each COMPUTED_FUNCTIONS as fn (fn)}<option value={fn}>{t('function.' + fn)}</option>{/each}
            </select>
            <div class="lgc-entries lgc-inputs" data-testid="var-inputs">
                <span class="lgc-label">{t('Inputs')}</span>
                {#each form.inputs as input, i (i)}
                    <div class="lgc-small lgc-muted">{inputLabel(form.fn, i)}</div>
                    <div class="lgc-row lgc-input-row" data-testid="var-input">
                        <select class="hmm-select lgc-input-kind" aria-label={t('Kind of input')} value={input.kind} onchange={(e) => Object.assign(input, {...emptyInput(), kind: e.currentTarget.value})} data-testid="input-kind">
                            <option value="device">{t('Datapoint of a device')}</option>
                            <option value="variable">{t('System variable')}</option>
                        </select>
                        {#if input.kind === 'device'}
                            <button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={input.channel} onclick={() => (picking = input)} data-testid="input-device">{channelLabel(input.channel)}</button>
                            {#if input.channel && !(input.channel in params)}
                                <input class="hmm-input lgc-ed-param" type="text" aria-label={t('Datapoint')} placeholder={t('Datapoint')} bind:value={input.param} data-testid="input-param" />
                            {:else}
                                <select class="hmm-select lgc-ed-param" aria-label={t('Datapoint')} bind:value={input.param} disabled={!input.channel} data-testid="input-param">
                                    {#if !numeric(input.channel).some((p) => p.id === input.param)}<option value={input.param}>{input.param || '–'}</option>{/if}
                                    {#each numeric(input.channel) as p (p.id)}<option value={p.id}>{p.id}</option>{/each}
                                </select>
                            {/if}
                        {:else}
                            <select class="hmm-select lgc-input-variable" aria-label={t('System variable')} bind:value={input.variable} data-testid="input-variable">
                                {#if !numbers.some((v) => v.def.key === input.variable)}<option value={input.variable}>{input.variable || t('Choose a variable…')}</option>{/if}
                                {#each numbers as v (v.def.key)}<option value={v.def.key}>{variableName(v.def)}</option>{/each}
                            </select>
                        {/if}
                        <button class="hmm-button" type="button" onclick={() => form.inputs.splice(i, 1)} disabled={form.inputs.length <= inputCount(form.fn)[0]} aria-label={t('Remove the input')} title={t('Remove the input')} data-testid="input-remove">✕</button>
                    </div>
                {/each}
                <button class="hmm-button" type="button" onclick={() => form.inputs.push(emptyInput())} disabled={form.inputs.length >= inputCount(form.fn)[1]} data-testid="input-add">{t('Add an input')}</button>
                <div class="lgc-help lgc-small">{t('The value is calculated again whenever an input reports, and is an update for the programs that look at it. Until every input has a value it stays what it was.')}</div>
                <div class="lgc-error-inline lgc-small lgc-reserve" data-testid="var-inputs-error">{tried ? (problems.inputs ?? '') : ''}</div>
            </div>
            <label class="lgc-label" for="var-cunit">{t('Unit')}</label>
            <input id="var-cunit" class="hmm-input lgc-narrow" type="text" bind:value={form.unit} data-testid="var-unit" />
        {/if}

        <span class="lgc-label">{t('Properties')}</span>
        <div class="lgc-checks">
            <label class="lgc-check"><input type="checkbox" bind:checked={form.visible} data-testid="var-visible" /> {t('visible')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={form.logged} data-testid="var-logged" /> {t('logged')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={form.internal} data-testid="var-internal" /> {t('internal')}</label>
            {#if form.helper && view?.def.helper && (view.def.helper.role === 'total' || view.def.helper.kind === 'rain-hm')}
                <label class="lgc-check"><input type="checkbox" bind:checked={form.helper.off} data-testid="var-helper-off" /> {t('device helper switched off')} <HelpLink topic="helpers" /></label>
            {/if}
        </div>
        <div class="lgc-span lgc-error-inline lgc-reserve" role="alert" data-testid="var-error">{error}</div>
    </form>
    {#snippet buttons()}
        {#if view && onversions}<button class="hmm-button lgc-button-aside" onclick={onversions} data-testid="var-versions">{t('Versions…')}</button>{/if}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} disabled={busy} data-testid="var-save">{t('Save')}</button>
    {/snippet}
</Dialog>
{#if picking}
    <DevicePicker {devices} loading={devicesLoading} error={devicesError} onpick={pick} onclose={() => (picking = null)} />
{/if}
