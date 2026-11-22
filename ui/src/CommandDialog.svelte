<script lang="ts">
    // A command in a form of its own, so the editor's row keeps its shape: the program and its
    // arguments - each a text or the value of a variable or a datapoint, always one argument -
    // or, marked as such, a line for the shell; then what becomes of it: how long it may run,
    // whether the block waits, where its output and its exit code go. Nothing changes before
    // "Apply".
    import {untrack} from 'svelte';
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import {get} from './api.js';
    import {commandProblem, DEFAULT_TIMEOUT, formatLabel, formatsFor, MAX_ARGS, MAX_TIMEOUT, newArg, paramKind, type ArgForm, type CommandForm} from './command.js';
    import DevicePicker from './DevicePicker.svelte';
    import type {Device, Env, Param} from './editor.js';
    import HelpLink from './HelpLink.svelte';
    import {variableName} from './model.js';

    let {command, env, devices, devicesLoading, devicesError, onsave, onclose}: {
        command: CommandForm; env: Env; devices: Device[]; devicesLoading: boolean; devicesError: string; onsave: (command: CommandForm) => void; onclose: () => void;
    } = $props();

    // svelte-ignore state_referenced_locally
    let form = $state<CommandForm>({...command, args: command.args.map((a) => ({...a}))});
    let tried = $state(false);
    let picking = $state<ArgForm | null>(null);
    // the datapoints of the channels chosen here, asked once each
    let params = $state<Record<string, Param[]>>({});
    const asked = new Set<string>();

    const problem = $derived(commandProblem(form, env.ctx, env.secrets));
    const all = $derived(Object.values(env.ctx.variables).sort((a, b) => variableName(a).localeCompare(variableName(b))));
    const outputs = $derived(all.filter((v) => !v.computed && ['string', 'number', 'bool'].includes(v.type)));
    const codes = $derived(all.filter((v) => !v.computed && v.type === 'number'));
    const PLACEHOLDERS = ['{program}', '{trigger}', '{value}', '{time}', '{var:…}', '{secret:…}'];

    const known = (channel: string): Param[] => (params[channel] ?? env.params[channel] ?? []).filter((p) => p.read);

    $effect(() => {
        const missing = form.args.filter((a) => a.kind === 'device' && a.channel && !(a.channel in env.params) && !asked.has(a.channel)).map((a) => a.channel);
        if (missing.length === 0) return;
        for (const ch of missing) asked.add(ch);
        get<{channels: Record<string, Param[]>}>('api/v1/datapoints?' + missing.map((ch) => 'channel=' + encodeURIComponent(ch)).join('&')).then(
            (r) => {
                params = {...untrack(() => params), ...r.channels};
                for (const a of form.args) if (a.kind === 'device' && missing.includes(a.channel) && !a.param) a.param = known(a.channel)[0]?.id ?? '';
            },
            () => undefined,
        );
    });

    function setKind(arg: ArgForm, kind: string) {
        Object.assign(arg, {...newArg(kind as ArgForm['kind']), id: arg.id});
    }

    function setShell(shell: boolean) {
        form.shell = shell;
        // the program's row is the line's first word and back
        if (shell && form.args[0]?.kind === 'text') {
            if (!form.line.trim()) form.line = form.args[0].text;
            form.args.splice(0, 1);
        } else if (!shell) {
            form.args.unshift(newArg('text', ''));
        }
    }

    function pick(channel: string) {
        if (!picking) return;
        picking.channel = channel;
        picking.param = known(channel)[0]?.id ?? '';
        picking.format = '';
        picking = null;
    }

    const channelLabel = (channel: string) => (channel ? (env.ctx.names[channel] ?? channel) : t('Choose a device…'));

    function formats(arg: ArgForm): string[] {
        if (arg.kind === 'variable') return formatsFor(env.ctx.variables[arg.variable]?.type ?? '');
        return formatsFor(paramKind(known(arg.channel).find((p) => p.id === arg.param)?.type));
    }

    function save() {
        tried = true;
        if (problem) return;
        onsave(form);
    }
</script>

<Dialog open title={t('Command')} width="50rem" {onclose} testId="command-dialog">
    <form class="lgc-cmd" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <div class="lgc-cmd-row">
            <label class="lgc-check"><input type="checkbox" checked={form.shell} onchange={(e) => setShell(e.currentTarget.checked)} data-testid="command-shell" /> {t('run a line through the shell')}</label>
            <span class="lgc-cmd-mark" class:lgc-cmd-on={form.shell} data-testid="command-shell-mark">{form.shell ? t('Shell: the line is interpreted') : t('no shell: every argument is handed over as it is')}</span>
            <HelpLink topic="command" />
        </div>
        {#if form.shell}
            <textarea class="hmm-input lgc-cmd-line lgc-mono" rows="3" spellcheck="false" aria-label={t('The line the shell runs')} placeholder={'wget -q -O - \'http://…\' | grep …'} bind:value={form.line} data-testid="command-line"></textarea>
        {/if}

        <div class="lgc-hook-head"><span>{form.shell ? t('Values for the line: "$1", "$2", …') : t('Program and arguments')}</span></div>
        {#each form.args as arg, i (arg.id)}
            {@const program = !form.shell && i === 0}
            <div class="lgc-cmd-row" data-testid="command-arg">
                <span class="lgc-cmd-n lgc-mono lgc-muted">{form.shell ? '$' + (i + 1) : program ? '' : i}</span>
                {#if program}
                    <input class="hmm-input lgc-cmd-text lgc-mono" type="text" spellcheck="false" autocomplete="off" placeholder="/usr/bin/curl" aria-label={t('Program')} bind:value={arg.text} data-testid="command-program" />
                {:else}
                    <select class="hmm-select lgc-cmd-kind" aria-label={t('Kind of argument')} value={arg.kind} onchange={(e) => setKind(arg, e.currentTarget.value)} data-testid="command-arg-kind">
                        <option value="text">{t('Text')}</option>
                        <option value="variable">{t('Value of a variable')}</option>
                        <option value="device">{t('Value of a datapoint')}</option>
                    </select>
                    {#if arg.kind === 'text'}
                        <input class="hmm-input lgc-cmd-text lgc-mono" type="text" spellcheck="false" autocomplete="off" aria-label={t('Argument')} bind:value={arg.text} data-testid="command-arg-text" />
                    {:else}
                        {#if arg.kind === 'variable'}
                            <select class="hmm-select lgc-cmd-text" aria-label={t('System variable')} bind:value={arg.variable} onchange={() => (arg.format = '')} data-testid="command-arg-variable">
                                {#if !env.ctx.variables[arg.variable]}<option value={arg.variable}>{arg.variable || t('Choose a variable…')}</option>{/if}
                                {#each all as v (v.key)}<option value={v.key}>{variableName(v)}</option>{/each}
                            </select>
                        {:else}
                            <span class="lgc-cmd-text lgc-cmd-device">
                                <button class="hmm-button lgc-ed-pick lgc-cut" type="button" title={arg.channel} onclick={() => (picking = arg)} data-testid="command-arg-device">{channelLabel(arg.channel)}</button>
                                <select class="hmm-select lgc-ed-param" aria-label={t('Datapoint')} bind:value={arg.param} disabled={!arg.channel} onchange={() => (arg.format = '')} data-testid="command-arg-param">
                                    {#if !known(arg.channel).some((p) => p.id === arg.param)}<option value={arg.param}>{arg.param || '–'}</option>{/if}
                                    {#each known(arg.channel) as p (p.id)}<option value={p.id}>{p.id}</option>{/each}
                                </select>
                            </span>
                        {/if}
                        <select class="hmm-select lgc-cmd-format" aria-label={t('Format')} bind:value={arg.format} data-testid="command-arg-format">
                            {#if !formats(arg).includes(arg.format)}<option value={arg.format}>{formatLabel(arg.format)}</option>{/if}
                            {#each formats(arg) as f (f)}<option value={f}>{formatLabel(f)}</option>{/each}
                        </select>
                    {/if}
                    <button class="hmm-button lgc-ed-icon" type="button" title={t('Remove')} aria-label={t('Remove')} onclick={() => form.args.splice(i, 1)} data-testid="command-arg-remove">✕</button>
                {/if}
            </div>
        {/each}
        <div><button class="hmm-button" type="button" disabled={form.args.length >= MAX_ARGS} onclick={() => form.args.push(newArg())} data-testid="command-arg-add">{form.shell ? t('Add a value') : t('Add an argument')}</button></div>

        <div class="lgc-hook-head"><span>{t('What becomes of it')}</span></div>
        <label class="lgc-check"><input type="checkbox" bind:checked={form.wait} data-testid="command-wait" /> {t('the following activities of this block wait until the command has ended (other programs never wait)')}</label>
        <div class="lgc-cmd-row">
            <label class="lgc-cmd-field">{t('Ended after (seconds)')}
                <input class="hmm-input lgc-cmd-num" type="text" inputmode="numeric" autocomplete="off" placeholder={String(DEFAULT_TIMEOUT)} title={t('1 to {max} seconds; empty: {n}', {max: MAX_TIMEOUT, n: DEFAULT_TIMEOUT})} bind:value={form.timeout} data-testid="command-timeout" />
            </label>
            <label class="lgc-cmd-field">{t('Output into')}
                <select class="hmm-select lgc-cmd-var" bind:value={form.output} data-testid="command-output">
                    <option value="">{t('– nothing –')}</option>
                    {#if form.output && !outputs.some((v) => v.key === form.output)}<option value={form.output}>{form.output}</option>{/if}
                    {#each outputs as v (v.key)}<option value={v.key}>{variableName(v)}</option>{/each}
                </select>
            </label>
            <label class="lgc-cmd-field">{t('Exit code into')}
                <select class="hmm-select lgc-cmd-var" bind:value={form.exitCode} data-testid="command-exit-code">
                    <option value="">{t('– nothing –')}</option>
                    {#if form.exitCode && !codes.some((v) => v.key === form.exitCode)}<option value={form.exitCode}>{form.exitCode}</option>{/if}
                    {#each codes as v (v.key)}<option value={v.key}>{variableName(v)}</option>{/each}
                </select>
            </label>
        </div>

        <p class="lgc-small lgc-muted" data-testid="command-notes">
            {t('The command runs with the addon\'s own rights, never as root, and only programs the system has.')}
            <br />{form.shell ? t('Placeholders in the line are put in as one quoted word each - do not put quotes around them:') : t('Placeholders in a text:')}
            {#each PLACEHOLDERS as p (p)}<code class="lgc-hook-ph">{p}</code>{' '}{/each}
            <HelpLink topic="command-values" />
        </p>
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="command-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="command-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
{#if picking}
    <DevicePicker {devices} loading={devicesLoading} error={devicesError} onpick={pick} onclose={() => (picking = null)} />
{/if}
