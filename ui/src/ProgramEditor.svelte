<script lang="ts">
    // The program editor, in the CCU's rows and words: "Bedingung: Wenn…" with its lines,
    // "Aktivität: Dann…" with its activities, "Sonst, wenn…", "Sonst…". Devices are chosen by
    // name in a picker, values are entered as their datapoint or variable asks, what is wrong is
    // said below its row while it is typed, lines and activities are dragged (or stepped with
    // their arrows) to another place, a rule can be copied. Nothing is stored before "Save".
    import {onMount, untrack} from 'svelte';
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import DevicePicker from './DevicePicker.svelte';
    import {combinedDisplay, displayForm, displayText, displayOf, type DisplayForm} from './display.js';
    import DisplayDialog from './DisplayDialog.svelte';
    import FormDialog from './FormDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import OptionsDialog from './OptionsDialog.svelte';
    import TimeDialog from './TimeDialog.svelte';
    import ValueField from './ValueField.svelte';
    import {get} from './api.js';
    import {
        actionExtras,
        addLine,
        addRule,
        channelNames,
        channelTypes,
        copyOf,
        copyRule,
        draftChannels,
        draftHints,
        draftOf,
        draftProblems,
        firstValue,
        lineExtras,
        lineKind,
        moveAction,
        moveLine,
        newAction,
        newDraft,
        newLine,
        opsFor,
        programOf,
        removeAction,
        removeLine,
        removeRule,
        sameProgram,
        stepAction,
        stepLine,
        stepRule,
        valueProblem,
        valuesText,
        type Device,
        type DraftAction,
        type DraftLine,
        type DraftRule,
        type Env,
        type Param,
    } from './editor.js';
    import {combinedValues, formKind, formWords, type Stored} from './forms.js';
    import ValuesDialog from './ValuesDialog.svelte';
    import WebhookDialog from './WebhookDialog.svelte';
    import CommandDialog from './CommandDialog.svelte';
    import {commandEmpty, commandOf, commandText} from './command.js';
    import {timeText, triggerLabel, variableName, type Context, type Program} from './model.js';

    let {
        program,
        copy = false,
        ctx,
        onsave,
        onclose,
        ondirty,
    }: {
        /** The program to edit, null for a new one; with copy, a new one that starts as its copy. */
        program: Program | null;
        copy?: boolean;
        ctx: Context;
        onsave: (program: Program) => Promise<string>;
        onclose: () => void;
        ondirty: (dirty: boolean) => void;
    } = $props();

    // svelte-ignore state_referenced_locally
    const isNew = program === null || copy;
    // svelte-ignore state_referenced_locally
    let draft = $state(program ? (copy ? copyOf(program) : draftOf(program)) : newDraft());
    // svelte-ignore state_referenced_locally
    const original = isNew ? newDraft() : draftOf(program!);

    let devices = $state<Device[]>([]);
    let devicesLoading = $state(true);
    let devicesError = $state('');
    let params = $state<Record<string, Param[]>>({});
    const asked = new Set<string>();
    // the channels whose datapoints are being asked for right now
    let asking = $state<string[]>([]);

    let tried = $state(false);
    let busy = $state(false);
    let error = $state('');
    let leaving = $state(false);
    let picking = $state<{of: DraftLine | DraftAction; from: boolean} | null>(null);
    let timing = $state<DraftLine | null>(null);
    let options = $state<DraftLine | DraftAction | null>(null);
    let hooking = $state<DraftAction | null>(null);
    let commanding = $state<DraftAction | null>(null);
    // the activity whose values are being set, in its channel's form or (plain) datapoint by datapoint
    let valuing = $state<{of: DraftAction; plain: boolean; values: Stored[]} | null>(null);
    let showing = $state<DraftAction | null>(null);
    // the names of the webhooks' secrets, asked when the draft has a webhook
    let secrets = $state<string[] | undefined>(undefined);
    let secretsAsked = false;
    let dragging = $state<{what: 'line' | 'action'; id: number} | null>(null);
    let over = $state(0);

    const env = $derived<Env>({ctx: {...ctx, names: {...ctx.names, ...channelNames(devices)}}, params, secrets});
    const problems = $derived(draftProblems(draft, env));
    const hints = $derived(draftHints(draft));
    const count = $derived(Object.keys(problems).length);
    const dirty = $derived(copy || !sameProgram(draft, original));
    const variables = $derived(Object.values(ctx.variables).sort((a, b) => variableName(a).localeCompare(variableName(b))));
    const programs = $derived(Object.entries(ctx.programs).filter(([key]) => key !== draft.key).sort((a, b) => a[1].localeCompare(b[1])));

    $effect(() => ondirty(dirty));

    onMount(() => {
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
        return () => ondirty(false);
    });

    $effect(() => {
        if (secretsAsked || !draft.rules.some((r) => r.then.some((a) => a.kind === 'webhook' || a.kind === 'command'))) return;
        secretsAsked = true;
        get<{secrets: {name: string}[]}>('api/v1/secrets').then(
            (r) => (secrets = r.secrets.map((x) => x.name)),
            () => undefined,
        );
    });

    // the datapoints of every channel the draft names, asked once per channel
    $effect(() => {
        const missing = draftChannels(draft).filter((ch) => !asked.has(ch));
        if (missing.length === 0) return;
        for (const ch of missing) asked.add(ch);
        asking = [...untrack(() => asking), ...missing];
        const answered = () => (asking = asking.filter((ch) => !missing.includes(ch)));
        get<{channels: Record<string, Param[]>}>('api/v1/datapoints?' + missing.map((ch) => 'channel=' + encodeURIComponent(ch)).join('&')).then(
            (r) => {
                answered();
                params = {...params, ...r.channels};
                // a channel chosen a moment ago gets its datapoint now
                for (const rule of draft.rules) {
                    for (const g of rule.groups) for (const l of g.lines) if (l.kind === 'device' && missing.includes(l.channel) && !l.param) settleChannel(l, false);
                    for (const a of rule.then) if (a.kind === 'set' && missing.includes(a.channel) && !a.param) settleChannel(a, true);
                }
            },
            answered,
        );
    });

    const PREFERRED = ['STATE', 'LEVEL', 'PRESS_SHORT', 'ACTUAL_TEMPERATURE', 'SET_POINT_TEMPERATURE'];

    function offered(channel: string, write: boolean): Param[] {
        return (params[channel] ?? []).filter((p) => (write ? p.write : p.read));
    }

    // a line's or an activity's comparison and value, made to fit what it is on now
    function settle(of: DraftLine | DraftAction) {
        const kind = lineKind(of, env);
        if ('op' in of) {
            if (!opsFor(kind).includes(of.op)) of.op = 'eq';
            if (of.op !== 'range' && of.op !== 'outside' && valueProblem(kind, of.value, of.op !== 'eq' && of.op !== 'ne')) of.value = firstValue(kind);
        } else if (valueProblem(kind, of.to)) {
            of.to = firstValue(kind);
        }
    }

    function settleChannel(of: DraftLine | DraftAction, write: boolean) {
        const list = offered(of.channel, write);
        if (list.length > 0 && !list.some((p) => p.id === of.param)) of.param = (PREFERRED.map((id) => list.find((p) => p.id === id)).find(Boolean) ?? list[0]!).id;
        settle(of);
    }

    function pick(channel: string) {
        if (!picking) return;
        const {of, from} = picking;
        picking = null;
        if (from) {
            const a = of as DraftAction;
            a.fromChannel = channel;
            a.fromParam = '';
            return;
        }
        of.channel = channel;
        if (!('op' in of) && of.kind === 'display') {
            showing = of;
            return;
        }
        if (!('op' in of) && of.kind === 'channel') {
            // another channel: its datapoints are chosen anew
            of.values = [];
            valuing = {of, plain: false, values: []};
            return;
        }
        if (channel in params) settleChannel(of, !('op' in of));
        else of.param = '';
    }

    function setLineKind(line: DraftLine, kind: string) {
        Object.assign(line, {...newLine(kind as DraftLine['kind']), id: line.id, note: line.note});
        if (kind === 'time') timing = line;
    }

    function setActionKind(action: DraftAction, kind: string) {
        const {id, note, delayed, delayN, delayUnit, random, upToN, upToUnit} = action;
        Object.assign(action, {...newAction(kind as DraftAction['kind']), id, note, delayed, delayN, delayUnit, random, upToN, upToUnit});
        if (kind === 'webhook') hooking = action;
        if (kind === 'command') commanding = action;
    }

    function setOp(line: DraftLine, op: string) {
        line.op = op;
        settle(line);
    }

    // "delayed at random" asks for its longest delay in the activity's options
    function setTiming(action: DraftAction, timing: string) {
        action.delayed = timing !== 'now';
        action.random = timing === 'random';
        if (action.random && !action.upToN) options = action;
    }

    const types = $derived(channelTypes(devices));
    const formOf = (channel: string) => formKind(types[channel], params[channel]);
    /** A "channel" activity's values as its row says them: in its form's words where it has one. */
    const valuesWords = (a: DraftAction) => formWords(types[a.channel], params[a.channel], a.values) || valuesText(a.values);

    /** The values an imported COMBINED_PARAMETER string stands for, on a channel that has a form; undefined when it stays as it is. */
    function combined(a: DraftAction): Stored[] | undefined {
        if (a.kind !== 'set' || a.param !== 'COMBINED_PARAMETER' || a.source !== 'value' || typeof a.to !== 'string' || !formOf(a.channel)) return undefined;
        return combinedValues(a.to, params[a.channel]);
    }

    function convert(action: DraftAction) {
        const values = combined(action);
        const lines = shownBy(action);
        if (!values && !lines) return;
        const channel = action.channel;
        setActionKind(action, lines ? 'display' : 'channel');
        action.channel = channel;
        if (lines) action.display = lines;
        else action.values = values!;
    }

    /** What an imported COMBINED_PARAMETER text shows on a display channel; undefined when it stays as it is. */
    function shownBy(a: DraftAction): DisplayForm | undefined {
        if (a.kind !== 'set' || a.param !== 'COMBINED_PARAMETER' || a.source !== 'value' || typeof a.to !== 'string' || types[a.channel] !== 'ACOUSTIC_DISPLAY_RECEIVER') return undefined;
        const d = combinedDisplay(a.to);
        return d && displayForm(d);
    }

    const channelLabel = (channel: string) => (channel ? (env.ctx.names[channel] ?? channel.split('/')[1] + ' (' + channel.split('/')[0] + ')') : t('Choose a device…'));

    async function save() {
        tried = true;
        error = '';
        if (count > 0) return;
        busy = true;
        error = await onsave(programOf(draft));
        busy = false;
    }

    function cancel() {
        if (dirty) leaving = true;
        else onclose();
    }

    function drop(what: 'line' | 'action', onto: number) {
        if (dragging?.what === what) {
            if (what === 'line') moveLine(draft, dragging.id, onto);
            else moveAction(draft, dragging.id, onto);
        }
        dragging = null;
        over = 0;
    }

    function dragStart(e: DragEvent, what: 'line' | 'action', id: number) {
        dragging = {what, id};
        e.dataTransfer?.setData('text/plain', String(id));
        if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
        const row = (e.currentTarget as HTMLElement).closest('.lgc-ed-row');
        if (row) e.dataTransfer?.setDragImage(row, 12, 12);
    }

    function dragOver(e: DragEvent, what: 'line' | 'action', id: number) {
        if (dragging?.what !== what) return;
        e.preventDefault();
        over = id;
    }
</script>

{#snippet datapoint(of: DraftLine | DraftAction, write: boolean, prefix: string)}
    <span class="lgc-ed-subject">
        <button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={of.channel} onclick={() => (picking = {of, from: false})} data-testid="{prefix}-device">{channelLabel(of.channel)}</button>
        {#if of.channel && !(of.channel in params)}
            <input class="hmm-input lgc-ed-param" type="text" aria-label={t('Datapoint')} placeholder={t('Datapoint')} bind:value={of.param} data-testid="{prefix}-param" />
        {:else}
            <select class="hmm-select lgc-ed-param" aria-label={t('Datapoint')} value={of.param} disabled={!of.channel} onchange={(e) => { of.param = e.currentTarget.value; settle(of); }} data-testid="{prefix}-param">
                {#if !offered(of.channel, write).some((p) => p.id === of.param)}<option value={of.param}>{of.param || '–'}</option>{/if}
                {#each offered(of.channel, write) as p (p.id)}<option value={p.id}>{p.id}</option>{/each}
            </select>
        {/if}
    </span>
{/snippet}

{#snippet variablePick(value: string, set: (key: string) => void, testId: string)}
    <select class="hmm-select lgc-ed-subject" aria-label={t('System variable')} {value} onchange={(e) => set(e.currentTarget.value)} data-testid={testId}>
        {#if !(value in ctx.variables)}<option {value}>{value || t('Choose a variable…')}</option>{/if}
        {#each variables as v (v.key)}<option value={v.key}>{variableName(v)}</option>{/each}
    </select>
{/snippet}

{#snippet steps(up: () => void, down: () => void, remove: () => void, prefix: string, more: (() => void) | null, extras: string)}
    <span class="lgc-ed-buttons">
        <button class="hmm-button lgc-ed-icon" class:lgc-ed-set={!!extras} type="button" onclick={() => more?.()} disabled={!more} title={extras || t('More options')} aria-label={t('More options')} data-testid="{prefix}-more">⋯</button>
        <button class="hmm-button lgc-ed-icon" type="button" onclick={up} title={t('Move up')} aria-label={t('Move up')} data-testid="{prefix}-up">▲</button>
        <button class="hmm-button lgc-ed-icon" type="button" onclick={down} title={t('Move down')} aria-label={t('Move down')} data-testid="{prefix}-down">▼</button>
        <button class="hmm-button lgc-ed-icon" type="button" onclick={remove} title={t('Remove')} aria-label={t('Remove')} data-testid="{prefix}-remove">✕</button>
    </span>
{/snippet}

{#snippet lineRow(line: DraftLine)}
    {@const kind = lineKind(line, env)}
    {@const problem = problems['l' + line.id] ?? ''}
    <div class="lgc-ed-row lgc-ed-line" class:lgc-ed-over={over === line.id && dragging?.what === 'line'} class:lgc-ed-dragged={dragging?.id === line.id} role="listitem"
        ondragover={(e) => dragOver(e, 'line', line.id)} ondrop={(e) => { e.preventDefault(); drop('line', line.id); }} data-testid="ed-line">
        <span class="lgc-ed-handle" draggable="true" role="button" tabindex="-1" title={t('Drag to another place')} aria-label={t('Drag to another place')}
            ondragstart={(e) => dragStart(e, 'line', line.id)} ondragend={() => { dragging = null; over = 0; }} data-testid="line-handle">⠿</span>
        <span class="lgc-ed-mark" title={line.trigger !== 'check' || line.kind === 'time' ? t('This line triggers the program.') : ''} data-testid="ed-line-mark">{line.trigger !== 'check' || line.kind === 'time' ? '⚡' : ''}</span>
        <select class="hmm-select lgc-ed-kind" aria-label={t('Kind of line')} value={line.kind} onchange={(e) => setLineKind(line, e.currentTarget.value)} data-testid="line-kind">
            <option value="device">{t('Device selection')}</option>
            <option value="variable">{t('System variable')}</option>
            <option value="time">{t('Time control')}</option>
        </select>
        {#if line.kind === 'time'}
            <button class="hmm-button lgc-ed-timebutton lgc-cut" type="button" title={line.time ? timeText(line.time) : ''} onclick={() => (timing = line)} data-testid="line-time">{line.time ? timeText(line.time) : t('Set the time…')}</button>
        {:else}
            {#if line.kind === 'device'}
                {@render datapoint(line, false, 'line')}
            {:else}
                {@render variablePick(line.variable, (key) => { line.variable = key; settle(line); }, 'line-variable')}
            {/if}
            <span class="lgc-ed-compare">
                <label class="lgc-check lgc-ed-not" title={t('The line is true when the comparison is not.')}><input type="checkbox" bind:checked={line.not} data-testid="line-not" /> {t('not')}</label>
                <select class="hmm-select lgc-ed-op" aria-label={t('Comparison')} value={line.op} onchange={(e) => setOp(line, e.currentTarget.value)} data-testid="line-op">
                    {#if !opsFor(kind).includes(line.op)}<option value={line.op}>{t('op.' + line.op)}</option>{/if}
                    {#each opsFor(kind) as op (op)}<option value={op}>{t('op.' + op)}</option>{/each}
                </select>
                {#if line.op === 'range' || line.op === 'outside'}
                    <ValueField bind:value={line.from} kind={{kind: 'number', unit: kind.unit}} label={t('from')} testId="line-from" invalid={tried && !!problem} />
                    <span class="lgc-small lgc-muted">{line.op === 'range' ? t('to less than') : t('to')}</span>
                    {#if line.op === 'range'}
                        <ValueField bind:value={line.below} kind={{kind: 'number', unit: kind.unit}} label={t('to less than')} testId="line-below" invalid={tried && !!problem} />
                    {:else}
                        <ValueField bind:value={line.to} kind={{kind: 'number', unit: kind.unit}} label={t('to')} testId="line-to" invalid={tried && !!problem} />
                    {/if}
                {:else}
                    <ValueField bind:value={line.value} kind={line.op === 'eq' || line.op === 'ne' ? kind : {kind: 'number', unit: kind.unit}} label={t('Value')} testId="line-value" invalid={tried && !!problem} />
                {/if}
            </span>
        {/if}
        <select class="hmm-select lgc-ed-trigger" aria-label={t('Trigger mode')} bind:value={line.trigger} data-testid="line-trigger">
            {#each line.kind === 'time' ? ['update', 'check'] : ['change', 'update', 'value', 'check'] as mode (mode)}<option value={mode}>{triggerLabel(mode, line.kind === 'time')}</option>{/each}
        </select>
        {@render steps(() => stepLine(draft, line.id, -1), () => stepLine(draft, line.id, 1), () => removeLine(draft, line.id), 'line', line.kind === 'time' ? null : () => (options = line), lineExtras(line))}
        <div class="lgc-ed-problem lgc-small" title={problem || lineExtras(line)}><span class:lgc-error-inline={tried} class:lgc-muted={!tried} data-testid="line-problem">{problem}</span><span class="lgc-muted" data-testid="line-extras">{problem ? '' : lineExtras(line)}</span></div>
    </div>
{/snippet}

{#snippet actionRow(action: DraftAction, index: number)}
    {@const kind = lineKind(action, env)}
    {@const problem = problems['a' + action.id] ?? ''}
    <div class="lgc-ed-row lgc-ed-action" class:lgc-ed-over={over === action.id && dragging?.what === 'action'} class:lgc-ed-dragged={dragging?.id === action.id} role="listitem"
        ondragover={(e) => dragOver(e, 'action', action.id)} ondrop={(e) => { e.preventDefault(); drop('action', action.id); }} data-testid="ed-action">
        <span class="lgc-ed-handle" draggable="true" role="button" tabindex="-1" title={t('Drag to another place')} aria-label={t('Drag to another place')}
            ondragstart={(e) => dragStart(e, 'action', action.id)} ondragend={() => { dragging = null; over = 0; }} data-testid="action-handle">⠿</span>
        <span class="lgc-ed-mark">{index + 1}</span>
        {#if action.kind === 'script'}
            <span class="lgc-ed-kind">{t('Script')}</span>
            <span class="lgc-ed-script lgc-muted lgc-cut" title={action.script}>{t('is kept as text and never run')}</span>
        {:else}
            <select class="hmm-select lgc-ed-kind" aria-label={t('Kind of activity')} value={action.kind} onchange={(e) => setActionKind(action, e.currentTarget.value)} data-testid="action-kind">
                <option value="set">{t('Device selection')}</option>
                <option value="variable">{t('System variable')}</option>
                <option value="program">{t('Program')}</option>
                <option value="channel">{t('Several values')}</option>
                <option value="display">{t('Display')}</option>
                <option value="webhook">{t('Webhook')}</option>
                <option value="wake">{t('Wake-on-LAN')}</option>
                <option value="command">{t('Command')}</option>
            </select>
            {#if action.kind === 'channel'}
                <span class="lgc-ed-subject"><button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={action.channel} onclick={() => (picking = {of: action, from: false})} data-testid="action-device">{channelLabel(action.channel)}</button></span>
                <button class="hmm-button lgc-ed-compare lgc-ed-values lgc-cut" type="button" title={valuesText(action.values)} disabled={!action.channel} onclick={() => (valuing = {of: action, plain: false, values: action.values})} data-testid="action-values">{action.values.length > 0 ? valuesWords(action) : t('Choose the values…')}</button>
            {:else if action.kind === 'display'}
                {@const words = displayText(displayOf(action.display))}
                <span class="lgc-ed-subject"><button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={action.channel} onclick={() => (picking = {of: action, from: false})} data-testid="action-device">{channelLabel(action.channel)}</button></span>
                <button class="hmm-button lgc-ed-compare lgc-ed-values lgc-cut" type="button" title={words} disabled={!action.channel} onclick={() => (showing = action)} data-testid="action-display">{words || t('Compose the display…')}</button>
            {:else if action.kind === 'webhook'}
                <button class="hmm-button lgc-ed-timebutton lgc-cut" type="button" title={action.hook.url} onclick={() => (hooking = action)} data-testid="action-webhook">{action.hook.url ? action.hook.method + ' ' + action.hook.url : t('Set the address…')}</button>
            {:else if action.kind === 'command'}
                {@const words = commandEmpty(action.command) ? '' : commandText(commandOf(action.command), env.ctx)}
                <button class="hmm-button lgc-ed-timebutton lgc-cut lgc-mono" type="button" title={words} onclick={() => (commanding = action)} data-testid="action-command">{words || t('Set the command…')}</button>
            {:else if action.kind === 'wake'}
                <input class="hmm-input lgc-ed-subject lgc-mono" type="text" spellcheck="false" autocomplete="off" placeholder="AA:BB:CC:DD:EE:FF" aria-label={t('Hardware address')} bind:value={action.mac} data-testid="action-mac" />
                <input class="hmm-input lgc-ed-compare" type="text" inputmode="decimal" spellcheck="false" autocomplete="off" placeholder={t('through (broadcast address, optional)')} title={t('The IPv4 broadcast address of another network; empty: the local network.')} aria-label={t('Broadcast address')} bind:value={action.broadcast} data-testid="action-broadcast" />
            {:else if action.kind === 'program'}
                <select class="hmm-select lgc-ed-subject" aria-label={t('Program')} bind:value={action.program} data-testid="action-program">
                    {#if !programs.some(([key]) => key === action.program)}<option value={action.program}>{action.program || t('Choose a program…')}</option>{/if}
                    {#each programs as [key, name] (key)}<option value={key}>{name}</option>{/each}
                </select>
                <select class="hmm-select lgc-ed-compare" aria-label={t('What is done')} bind:value={action.what} data-testid="action-what">
                    <option value="run-check">{t('evaluate')}</option>
                    <option value="run-then">{t('run the first "Then"')}</option>
                    <option value="activate">{t('switch on')}</option>
                    <option value="deactivate">{t('switch off')}</option>
                </select>
            {:else}
                {#if action.kind === 'set'}
                    {@render datapoint(action, true, 'action')}
                {:else}
                    {@render variablePick(action.variable, (key) => { action.variable = key; settle(action); }, 'action-variable')}
                {/if}
                <span class="lgc-ed-compare">
                    <select class="hmm-select lgc-ed-op" aria-label={t('Where the value comes from')} bind:value={action.source} data-testid="action-source">
                        <option value="value">{t('to the value')}</option>
                        <option value="variable">{t('to the value of the variable')}</option>
                        <option value="device">{t('to the value of the datapoint')}</option>
                        {#if action.source === 'special'}<option value="special">{t('special value {v}: skipped until edited', {v: action.special})}</option>{/if}
                    </select>
                    {#if action.source === 'value'}
                        <ValueField bind:value={action.to} {kind} label={t('Value')} testId="action-value" invalid={tried && !!problem} />
                    {:else if action.source === 'variable'}
                        {@render variablePick(action.fromVariable, (key) => (action.fromVariable = key), 'action-from-variable')}
                    {:else if action.source === 'device'}
                        <button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={action.fromChannel} onclick={() => (picking = {of: action, from: true})} data-testid="action-from-device">{channelLabel(action.fromChannel)}</button>
                        <select class="hmm-select lgc-ed-param" aria-label={t('Datapoint')} bind:value={action.fromParam} disabled={!action.fromChannel} data-testid="action-from-param">
                            {#if !offered(action.fromChannel, false).some((p) => p.id === action.fromParam)}<option value={action.fromParam}>{action.fromParam || '–'}</option>{/if}
                            {#each offered(action.fromChannel, false) as p (p.id)}<option value={p.id}>{p.id}</option>{/each}
                        </select>
                    {/if}
                </span>
            {/if}
        {/if}
        <span class="lgc-ed-trigger lgc-ed-delay">
            <select class="hmm-select" aria-label={t('When')} value={!action.delayed ? 'now' : action.random && action.kind !== 'script' ? 'random' : 'delayed'} onchange={(e) => setTiming(action, e.currentTarget.value)} data-testid="action-timing">
                <option value="now">{t('immediately')}</option>
                <option value="delayed">{t('delayed by')}</option>
                {#if action.kind !== 'script'}<option value="random">{t('at random')}</option>{/if}
            </select>
            <input class="hmm-input lgc-ed-delay-n" class:lgc-ed-hidden={!action.delayed} type="text" inputmode="numeric" aria-label={t('Delay')} bind:value={action.delayN} disabled={!action.delayed} data-testid="action-delay" />
            <select class="hmm-select lgc-ed-delay-unit" class:lgc-ed-hidden={!action.delayed} aria-label={t('Unit')} bind:value={action.delayUnit} disabled={!action.delayed} data-testid="action-delay-unit">
                {#each ['s', 'min', 'h'] as unit (unit)}<option value={unit} title={t('unit.' + unit)}>{unit}</option>{/each}
            </select>
        </span>
        {@render steps(() => stepAction(draft, action.id, -1), () => stepAction(draft, action.id, 1), () => removeAction(draft, action.id), 'action', action.kind === 'script' ? null : () => (options = action), actionExtras(action))}
        <div class="lgc-ed-problem lgc-small" title={problem || actionExtras(action)}><span class:lgc-error-inline={tried} class:lgc-muted={!tried} data-testid="action-problem">{problem}</span>{#if !problem && shownBy(action)}<button class="lgc-ed-convert" type="button" onclick={() => convert(action)} data-testid="action-convert">{t('Convert into a form')}</button><span class="lgc-muted" data-testid="action-decoded">{displayText(displayOf(shownBy(action)!))}</span>{/if}{#if !problem && combined(action)}{@const values = combined(action)!}<button class="lgc-ed-convert" type="button" title={valuesText(values)} onclick={() => convert(action)} data-testid="action-convert">{t('Convert into a form')}</button><span class="lgc-muted" data-testid="action-decoded">{formWords(types[action.channel], params[action.channel], values) || valuesText(values)}</span>{/if}<span class="lgc-muted" data-testid="action-extras">{problem ? '' : actionExtras(action)}</span></div>
    </div>
{/snippet}

{#snippet ruleBlock(rule: DraftRule, ri: number)}
    <div class="lgc-rule lgc-ed-rule" data-testid="ed-rule">
        <div class="lgc-ed-rulehead">
            <h3>{rule.else ? t('Else…') : ri === 0 ? t('Condition: If…') : t('Else if…')} {#if ri === 0}<HelpLink topic="editor-lines" />{/if}{#if rule.else}<HelpLink topic="else" />{/if}</h3>
            <span class="lgc-spacer"></span>
            {#if !rule.else}
                <button class="hmm-button lgc-ed-icon" type="button" onclick={() => stepRule(draft, rule.id, -1)} title={t('Move up')} aria-label={t('Move up')} data-testid="rule-up">▲</button>
                <button class="hmm-button lgc-ed-icon" type="button" onclick={() => stepRule(draft, rule.id, 1)} title={t('Move down')} aria-label={t('Move down')} data-testid="rule-down">▼</button>
                <button class="hmm-button" type="button" onclick={() => copyRule(draft, rule.id)} data-testid="rule-copy">{t('Copy the rule')}</button>
            {/if}
            <button class="hmm-button" type="button" onclick={() => removeRule(draft, rule.id)} disabled={draft.rules.length < 2} data-testid="rule-remove">{t('Remove the rule')}</button>
        </div>
        {#if !rule.else}
            {#if rule.groups.length === 0}
                <p class="lgc-muted lgc-indent">{t('No condition: always true.')}</p>
            {/if}
            {#each rule.groups as group, gi (group.id)}
                {#if gi > 0}
                    <div class="lgc-junction lgc-junction-outer">
                        <select class="hmm-select lgc-ed-junction" aria-label={t('How the groups are joined')} bind:value={rule.op} disabled={gi > 1} data-testid="rule-op">
                            <option value="any">{t('OR')}</option>
                            <option value="all">{t('AND')}</option>
                        </select>
                    </div>
                {/if}
                <div class="lgc-group" role="list" data-testid="ed-group">
                    {#each group.lines as line, li (line.id)}
                        {#if li > 0}
                            <div class="lgc-junction">
                                <select class="hmm-select lgc-ed-junction" aria-label={t('How the lines are joined')} bind:value={group.op} disabled={li > 1} data-testid="group-op">
                                    <option value="all">{t('AND')}</option>
                                    <option value="any">{t('OR')}</option>
                                </select>
                            </div>
                        {/if}
                        {@render lineRow(line)}
                    {/each}
                    <button class="hmm-button lgc-ed-add" type="button" onclick={() => addLine(rule, group)} data-testid="line-add">{t('Add a line')}</button>
                </div>
            {/each}
            <button class="hmm-button lgc-ed-add" type="button" onclick={() => addLine(rule)} data-testid="group-add">{rule.groups.length === 0 ? t('Add a line') : t('Add a group of lines')}</button>
        {/if}
        <h4>
            {rule.else ? t('Activity:') : t('Activity: Then…')}
            {#if ri === 0}<HelpLink topic="editor-activities" />{/if}
            <label class="lgc-check lgc-ed-cancel"><input type="checkbox" bind:checked={rule.cancelPending} data-testid="rule-cancel-pending" /> {t('end running delays for these targets first')}</label>
            {#if ri === 0}<HelpLink topic="delays" />{/if}
        </h4>
        {#if rule.then.length === 0}
            <p class="lgc-muted lgc-indent">{t('(no activity)')}</p>
        {/if}
        <div role="list">
            {#each rule.then as action, ai (action.id)}
                {@render actionRow(action, ai)}
            {/each}
        </div>
        <button class="hmm-button lgc-ed-add" type="button" onclick={() => rule.then.push(newAction())} data-testid="action-add">{t('Add an activity')}</button>
    </div>
{/snippet}

<section class="lgc-section lgc-section-wide lgc-ed" data-testid="program-editor">
    <div class="lgc-section-head">
        <h2>{isNew ? t('New program') : t('Edit the program')}</h2>
        <HelpLink topic="editor" />
        <span class="lgc-spacer"></span>
        <button class="hmm-button" type="button" onclick={cancel} data-testid="editor-cancel">{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" type="button" onclick={save} disabled={busy} data-testid="editor-save">{t('Save')}</button>
    </div>

    <div class="lgc-form lgc-ed-head">
        <label class="lgc-label" for="ed-name">{t('Name')}</label>
        <div>
            <input id="ed-name" class="hmm-input" class:lgc-invalid={tried && !!problems.name} type="text" bind:value={draft.name} data-testid="ed-name" />
            <div class="lgc-small lgc-reserve" class:lgc-error-inline={tried} class:lgc-muted={!tried} data-testid="ed-name-problem">{problems.name ?? ''}</div>
        </div>
        <label class="lgc-label" for="ed-note">{t('Description')}</label>
        <input id="ed-note" class="hmm-input" type="text" bind:value={draft.note} data-testid="ed-note" />
        <span class="lgc-label">{t('Properties')}</span>
        <div class="lgc-checks">
            <label class="lgc-check"><input type="checkbox" bind:checked={draft.active} data-testid="ed-active" /> {t('active')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={draft.visible} data-testid="ed-visible" /> {t('visible')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={draft.operable} data-testid="ed-operable" /> {t('operable')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={draft.internal} data-testid="ed-internal" /> {t('internal')}</label>
            <label class="lgc-check"><input type="checkbox" bind:checked={draft.atStart} data-testid="ed-at-start" /> {t('evaluate at system start')}</label>
            <HelpLink topic="start" />
        </div>
    </div>

    {#each draft.rules as rule, ri (rule.id)}
        {@render ruleBlock(rule, ri)}
    {/each}

    <div class="lgc-actions">
        <button class="hmm-button" type="button" onclick={() => addRule(draft, false)} data-testid="rule-add">{t('Add "Else if…"')}</button>
        <button class="hmm-button" type="button" onclick={() => addRule(draft, true)} disabled={draft.rules.some((r) => r.else)} data-testid="else-add">{t('Add "Else…"')}</button>
        <HelpLink topic="else" />
    </div>

    <ul class="lgc-ed-hints lgc-small" data-testid="ed-hints">
        {#each hints as hint (hint)}<li>{hint}</li>{/each}
    </ul>
    <div class="lgc-actions lgc-ed-foot">
        <div class="lgc-error-inline lgc-reserve lgc-ed-error" role="alert" data-testid="editor-error">
            {error || (tried && count > 0 ? t('{n} places need attention before the program can be saved.', {n: count}, count) : '')}
        </div>
        <span class="lgc-spacer"></span>
        <button class="hmm-button" type="button" onclick={cancel} data-testid="editor-cancel-foot">{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" type="button" onclick={save} disabled={busy} data-testid="editor-save-foot">{t('Save')}</button>
    </div>
</section>

{#if picking}
    <DevicePicker {devices} loading={devicesLoading} error={devicesError} onpick={pick} onclose={() => (picking = null)} />
{/if}
{#if options}
    {@const of = options}
    <OptionsDialog {of} {env} onclose={() => (options = null)} />
{/if}
{#if valuing}
    {@const of = valuing.of}
    {@const form = formOf(of.channel)}
    {#if form && !valuing.plain}
        <FormDialog channel={of.channel} name={channelLabel(of.channel)} kind={form} values={valuing.values} {env} onsave={(values) => { of.values = values; valuing = null; }} onall={(values) => (valuing = {of, plain: true, values})} onclose={() => (valuing = null)} />
    {:else}
        {#key valuing.values}
            <ValuesDialog channel={of.channel} name={channelLabel(of.channel)} values={valuing.values} {env} loading={asking.includes(of.channel)} onsave={(values) => { of.values = values; valuing = null; }}
                onform={form ? (values) => (valuing = {of, plain: false, values}) : undefined} onclose={() => (valuing = null)} />
        {/key}
    {/if}
{/if}
{#if showing}
    {@const of = showing}
    <DisplayDialog channel={of.channel} name={channelLabel(of.channel)} display={of.display} {env} onsave={(display) => { of.display = display; showing = null; }} onclose={() => (showing = null)} />
{/if}
{#if hooking}
    {@const of = hooking}
    <WebhookDialog hook={of.hook} ctx={env.ctx} {secrets} onsave={(hook) => { of.hook = hook; hooking = null; }} onclose={() => (hooking = null)} />
{/if}
{#if commanding}
    {@const of = commanding}
    <CommandDialog command={of.command} {env} {devices} {devicesLoading} {devicesError} onsave={(command) => { of.command = command; commanding = null; }} onclose={() => (commanding = null)} />
{/if}
{#if timing}
    {@const line = timing}
    <TimeDialog time={line.time} onsave={(m) => { line.time = m; timing = null; }} onclose={() => (timing = null)} />
{/if}
<ConfirmDialog open={leaving} title={t('Discard the changes?')} message={t('The program was changed and is not saved yet.')} confirmLabel={t('Discard')} danger testId="confirm-discard"
    onconfirm={() => { leaving = false; onclose(); }} onclose={() => (leaving = false)} />
