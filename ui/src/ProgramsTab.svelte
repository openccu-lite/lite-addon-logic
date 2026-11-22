<script lang="ts">
    // The program list: search, state, what triggers each, the last run; a click opens the
    // program in the CCU's row layout, from where it is edited, copied or removed; "New
    // program" opens the editor on an empty one. Every row has the same
    // height: a text that does not fit its column is cut, and is whole in its tooltip.
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import Marks from './Marks.svelte';
    import ProgramDetail from './ProgramDetail.svelte';
    import ProgramEditor from './ProgramEditor.svelte';
    import type {Program} from './model.js';
    import {blockText, context, dateTime, matches, nameTitle, programMarks, programState, triggerSummary, wallTime, type Overview} from './model.js';

    let {
        overview,
        open = $bindable(''),
        onactive,
        onatstart,
        onrun,
        onimport,
        onsettings,
        onsave,
        ondelete,
        ondirty,
        onrefresh,
    }: {
        overview: Overview;
        /** The key of the program that is shown; '' shows the list. */
        open?: string;
        onactive: (key: string, active: boolean) => void;
        onatstart: (key: string, atStart: boolean) => void;
        onrun: (key: string, mode: string) => void;
        onimport: () => void;
        onsettings: () => void;
        /** Stores a program (key '' for a new one): answers the key it has now, or what was wrong. */
        onsave: (key: string, program: Program) => Promise<{key?: string; error?: string}>;
        ondelete: (key: string) => Promise<boolean>;
        ondirty: (dirty: boolean) => void;
        /** Something was changed past the page's own actions (a version brought back). */
        onrefresh: () => void;
    } = $props();

    let query = $state('');
    let internal = $state(false);
    // the editor: on a program, on a copy of one, or on a new one
    let editing = $state<{program: Program | null; copy: boolean} | null>(null);

    async function save(program: Program): Promise<string> {
        const res = await onsave(editing?.copy ? '' : program.key, program);
        if (res.error) return res.error;
        open = res.key ?? '';
        editing = null;
        return '';
    }

    async function remove(key: string) {
        if (await ondelete(key)) open = '';
    }

    const ctx = $derived(context(overview));
    const shown = $derived(
        overview.programs.filter((p) => (internal || !p.program.internal) && matches(query, p.program.name, p.program.note, ...triggerSummary(p, ctx))),
    );
    const hidden = $derived(overview.programs.filter((p) => p.program.internal).length);
    const current = $derived(overview.programs.find((p) => p.program.key === open));
</script>

{#if editing}
    <ProgramEditor program={editing.program} copy={editing.copy} {ctx} onsave={save} onclose={() => (editing = null)} {ondirty} />
{:else if current}
    <ProgramDetail view={current} {ctx} {overview} onback={() => (open = '')} {onactive} {onatstart} {onrun} {onsettings} onrestored={onrefresh}
        onedit={() => (editing = {program: current.program, copy: false})} oncopy={() => (editing = {program: current.program, copy: true})} ondelete={() => remove(current.program.key)} />
{:else}
    <section class="lgc-section lgc-section-wide">
        <div class="lgc-section-head">
            <h2>{t('Programs')}</h2>
            <HelpLink topic="program-list" />
            <span class="lgc-spacer"></span>
            <input class="hmm-input lgc-search" type="search" placeholder={t('Search…')} aria-label={t('Search…')} bind:value={query} data-testid="program-search" />
            <button class="hmm-button lgc-primary" onclick={() => (editing = {program: null, copy: false})} data-testid="program-new">{t('New program')}</button>
            <HelpLink topic="editor" />
        </div>
        {#if overview.programs.length === 0}
            <div class="lgc-notice" data-testid="no-programs">
                <p>{t('There are no programs yet. Programs of a CCU come in through the import.')}</p>
                <button class="hmm-button" onclick={onimport} data-testid="go-import">{t('Import')}</button>
            </div>
        {:else}
            <div class="lgc-tablewrap">
                <table class="lgc-grid lgc-cardgrid lgc-fixed lgc-programs" data-testid="program-list">
                    <thead>
                        <tr>
                            <th class="lgc-w-pill">{t('State')}</th>
                            <th class="lgc-w-name">{t('Name')}</th>
                            <th class="lgc-c-note">{t('Description')}</th>
                            <th class="lgc-w-marks">{t('Marks')} <HelpLink topic="program-marks" /></th>
                            <th>{t('Triggered by')} <HelpLink topic="triggers" /></th>
                            <th class="lgc-w-run">{t('Last run')}</th>
                            <th class="lgc-w-time">{t('Next time')} <HelpLink topic="time-next" /></th>
                            <th class="lgc-col-actions lgc-w-switch"></th>
                        </tr>
                    </thead>
                    <tbody>
                        {#each shown as p (p.program.key)}
                            {@const st = programState(p)}
                            {@const triggers = triggerSummary(p, ctx)}
                            {@const lastRun = p.state.last_run ? dateTime(p.state.last_run.time, i18n.language) + ' · ' + blockText(p.state.last_run.block) : ''}
                            <tr class:lgc-off={!p.program.active} data-testid="program-{p.program.key}">
                                <td data-label={t('State')}><span class={'lgc-pill ' + st.kind} data-testid="program-state">{st.label}</span></td>
                                <td data-label={t('Name')}><button class="lgc-linklike lgc-cut" title={nameTitle(p.program.name, p.program.note)} onclick={() => (open = p.program.key)} data-testid="program-open">{p.program.name}</button></td>
                                <td class="lgc-c-note" data-label={t('Description')}><span class="lgc-cut lgc-muted" title={p.program.note ?? ''}>{p.program.note || '–'}</span></td>
                                <td data-label={t('Marks')}><Marks marks={programMarks(p)} /></td>
                                <td data-label={t('Triggered by')}>
                                    {#if triggers.length === 0}
                                        <span class="lgc-cut lgc-muted" title={t('Only "check only" lines: it never runs by itself.')}>{t('no trigger')}</span>
                                    {:else}
                                        <span class="lgc-cut" title={triggers.join(', ')}>{triggers.join(', ')}</span>
                                    {/if}
                                </td>
                                <td data-label={t('Last run')}><span class="lgc-cut" class:lgc-muted={!lastRun} title={lastRun}>{lastRun || '–'}</span></td>
                                <td data-label={t('Next time')}>
                                    <span class="lgc-cut" class:lgc-muted={!p.state.next_time} data-testid="program-next">{p.state.next_time ? wallTime(p.state.next_time, i18n.language) : '–'}</span>
                                </td>
                                <td class="lgc-cell-actions lgc-col-actions">
                                    <label class="lgc-switch" title={t('active')}>
                                        <input type="checkbox" checked={p.program.active} disabled={p.script || p.problems.length > 0} onchange={(e) => onactive(p.program.key, e.currentTarget.checked)} data-testid="program-active" />
                                        <span>{t('active')}</span>
                                    </label>
                                </td>
                            </tr>
                        {/each}
                    </tbody>
                </table>
            </div>
            {#if shown.length === 0}
                <p class="lgc-muted" data-testid="no-match">{t('No program matches the search.')}</p>
            {/if}
            {#if hidden > 0}
                <label class="lgc-check lgc-small"><input type="checkbox" bind:checked={internal} data-testid="show-internal" /> {t('show the {n} internal programs', {n: hidden}, hidden)}</label>
            {/if}
        {/if}
    </section>
{/if}
