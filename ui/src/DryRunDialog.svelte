<script lang="ts">
    // The dry run of a program: what it would do if it were evaluated now, or if one of its
    // condition lines' source reported a value - with the same entry the History shows, and
    // nothing sent, written or scheduled.
    import {onMount} from 'svelte';
    import {Dialog} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import RunEntry from './RunEntry.svelte';
    import ValueField from './ValueField.svelte';
    import {ApiError, post} from './api.js';
    import type {Context, Program, Value} from './model.js';
    import {runView, simLines, simRequest, type TraceRun} from './trace.js';

    let {program, ctx, onclose}: {program: Program; ctx: Context; onclose: () => void} = $props();

    const lines = $derived(simLines(program, ctx));
    // 0: the program as it stands now; otherwise the number of the line whose source reports
    let chosen = $state(0);
    let value = $state<Value>(undefined);
    let result = $state<TraceRun | null>(null);
    let error = $state('');
    let busy = $state(false);
    const line = $derived(lines.find((l) => l.n === chosen));

    function choose(n: number) {
        chosen = n;
        value = lines.find((l) => l.n === n)?.value;
        result = null;
        error = '';
        if (n === 0) void go();
    }

    async function go() {
        busy = true;
        error = '';
        try {
            result = await post<TraceRun>(`api/v1/programs/${program.key}/dry-run`, simRequest(line, value));
        } catch (e) {
            result = null;
            error = e instanceof ApiError || e instanceof Error ? e.message : String(e);
        }
        busy = false;
    }

    // opened: what would run now
    onMount(() => void go());
</script>

<Dialog open title={t('Dry run: {name}', {name: program.name})} width="52rem" height="34rem" {onclose} testId="dry-run-dialog">
    <p class="lgc-muted lgc-small">{t('What the program would do. Nothing is sent, written or scheduled, and the program remembers nothing of it.')} <HelpLink topic="dry-run" /></p>
    <div class="lgc-sim-form">
        <select class="hmm-select lgc-sim-line" aria-label={t('What to try')} value={String(chosen)} onchange={(e) => choose(Number(e.currentTarget.value))} data-testid="sim-line">
            <option value="0">{t('now, as everything stands')}</option>
            {#each lines as l (l.n)}<option value={String(l.n)}>{l.kind ? t('if line {label} reported…', {label: l.label}) : t('if line {label} fired now', {label: l.label})}</option>{/each}
        </select>
        <span class="lgc-sim-value">
            {#if line?.kind}
                {#key chosen}<ValueField bind:value kind={line.kind} label={t('Value')} testId="sim-value" />{/key}
            {/if}
        </span>
        <button class="hmm-button lgc-primary" onclick={go} disabled={busy || chosen === 0} data-testid="sim-go">{t('Simulate')}</button>
    </div>
    <div class="lgc-sim-result" data-testid="sim-result">
        {#if error}
            <div class="lgc-error" role="alert" data-testid="sim-error">{error}</div>
        {:else if result}
            <RunEntry v={runView(result, ctx, i18n.language)} own open />
        {/if}
    </div>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose} data-testid="sim-close">{t('Close')}</button>
    {/snippet}
</Dialog>
