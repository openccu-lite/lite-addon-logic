<script lang="ts">
    // The run trace as a list: every entry a row - when, which program, what caused it, what
    // became of it - that opens to the condition lines as the run saw them and to what each
    // activity did. With a program, that program's entries (and the notes that it was not
    // triggered); otherwise the global log.
    import {i18n, t} from 'occulite-ui/i18n';

    import RunEntry from './RunEntry.svelte';
    import {get} from './api.js';
    import {context, type Overview} from './model.js';
    import {filterRuns, mergeRuns, runView, type TracePage, type TraceRun} from './trace.js';

    let {overview, program = '', query = '', show = '', onopen}: {overview: Overview; program?: string; query?: string; show?: string; onopen?: (key: string) => void} = $props();

    const PAGE = 100;
    let runs = $state<TraceRun[]>([]);
    let more = $state(false);
    let loaded = $state(false);
    let busy = $state(false);
    // the entries that are open, by their number
    let open = $state<Record<number, boolean>>({});
    // which program the entries belong to: another one starts anew
    let of = '';

    const path = (before?: number) => `api/v1/runs?limit=${PAGE}` + (program ? '&program=' + encodeURIComponent(program) : '') + (before ? '&before=' + before : '');

    // asked again whenever the page's data is; pages loaded before stay
    $effect(() => {
        void overview;
        const want = program;
        get<TracePage>(path()).then(
            (p) => {
                if (want !== program) return;
                if (of !== want) {
                    of = want;
                    runs = p.runs;
                    more = p.more;
                } else {
                    if (runs.length <= PAGE) more = p.more;
                    runs = mergeRuns(runs, p.runs);
                }
                loaded = true;
            },
            () => {},
        );
    });

    async function older() {
        const last = runs[runs.length - 1];
        if (!last || busy) return;
        busy = true;
        try {
            const p = await get<TracePage>(path(last.seq));
            runs = mergeRuns(runs, p.runs);
            more = p.more;
        } catch {
            /* the banner says the addon is not reachable */
        }
        busy = false;
    }

    const ctx = $derived(context(overview));
    const views = $derived(filterRuns(runs.map((r) => runView(r, ctx, i18n.language)), {query, show}));
</script>

<div class="lgc-run-list" class:lgc-run-own={!!program} data-testid="run-list">
    {#each views as v (v.seq)}
        <RunEntry {v} own={!!program} open={!!open[v.seq]} ontoggle={() => (open[v.seq] = !open[v.seq])} onopen={onopen && !program ? onopen : undefined} />
    {/each}
    {#if loaded && views.length === 0}
        <p class="lgc-muted" data-testid="run-empty">{runs.length === 0 ? (program ? t('This program has not run yet, and nothing it looks at has reported.') : t('No program has run yet.')) : t('Nothing matches.')}</p>
    {/if}
    {#if more}
        <button class="hmm-button" onclick={older} disabled={busy} data-testid="run-older">{t('Older entries')}</button>
    {/if}
</div>
