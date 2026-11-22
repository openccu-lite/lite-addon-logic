<script lang="ts">
    // The schedule: what is armed. Every time module with the program it belongs to, when it
    // fires next and when it last fired; every delayed activity that is not due yet, which can
    // be carried out now or cancelled. Times are the system's wall clock; a time on the other
    // side of a clock change says its offset. Every row has the same height.
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {get} from './api.js';
    import {context, wallTime, type Overview} from './model.js';
    import {offsetNote, relative, scheduleRows, type Schedule} from './schedule.js';

    let {overview, onopen, onrun, oncancel}: {overview: Overview; onopen: (key: string) => void; onrun: (id: number) => void; oncancel: (id: number) => void} = $props();

    let schedule = $state<Schedule | null>(null);
    let error = $state('');
    let filter = $state('all');
    let query = $state('');

    // asked again whenever the page's data is (every five seconds, and after every action)
    $effect(() => {
        void overview;
        get<Schedule>('api/v1/schedule').then(
            (s) => {
                schedule = s;
                error = '';
            },
            (e: Error) => (error = e.message),
        );
    });

    const ctx = $derived(context(overview));
    const rows = $derived(schedule ? scheduleRows(schedule, ctx, filter, query) : []);
    const when = (iso: string | undefined, note: string, now: string) => {
        if (!iso) return '';
        const offset = offsetNote(iso, now);
        return wallTime(iso, i18n.language) + (offset ? ' ' + offset : '') + (note ? ' (' + note + ')' : '');
    };
</script>

<section class="lgc-section lgc-section-wide">
    <div class="lgc-section-head">
        <h2>{t('Schedule')}</h2>
        <HelpLink topic="schedule" />
        <span class="lgc-spacer"></span>
        <select class="hmm-select" aria-label={t('Show')} bind:value={filter} data-testid="schedule-filter">
            <option value="all">{t('everything')}</option>
            <option value="time">{t('time modules only')}</option>
            <option value="delay">{t('pending delays only')}</option>
            <option value="hold">{t('counting lines only')}</option>
        </select>
        <input class="hmm-input lgc-search" type="search" placeholder={t('Search…')} aria-label={t('Search…')} bind:value={query} data-testid="schedule-search" />
    </div>
    <div class="lgc-error-inline lgc-small lgc-reserve" role="alert" data-testid="schedule-error">{error}</div>
    {#if schedule}
        <div class="lgc-tablewrap">
            <table class="lgc-grid lgc-cardgrid lgc-fixed lgc-schedule" data-testid="schedule-list">
                <thead>
                    <tr>
                        <th class="lgc-w-type">{t('Kind')}</th>
                        <th>{t('What')}</th>
                        <th class="lgc-w-sprog">{t('Program')}</th>
                        <th class="lgc-w-run">{t('Next time')} <HelpLink topic="time-next" /></th>
                        <th class="lgc-w-rel">{t('That is')}</th>
                        <th class="lgc-w-slast">{t('Last time')}</th>
                        <th class="lgc-col-actions lgc-w-sdelay">{t('Delay')} <HelpLink topic="schedule-delays" /></th>
                    </tr>
                </thead>
                <tbody>
                    {#each rows as row (row.id)}
                        {@const next = when(row.next, row.nextNote, schedule.now)}
                        {@const last = when(row.last, row.lastNote, schedule.now)}
                        <tr class:lgc-off={!!row.off} data-testid="schedule-{row.id}">
                            <td data-label={t('Kind')}>
                                <span class={'lgc-pill ' + (row.kind === 'delay' ? 'warn' : row.active ? 'ok' : '')}
                                    title={row.kind === 'hold' ? (row.active ? t('The comparison has held long enough: the line is true now.') : t('The comparison holds: the line becomes true when it has held long enough.')) : row.active ? t('A range that is under way: the line is true now.') : ''}
                                    data-testid="schedule-kind">
                                    {row.kind === 'delay' ? t('delay') : row.active ? t('true now') : row.kind === 'hold' ? t('counting') : t('time module')}
                                </span>
                            </td>
                            <td data-label={t('What')}><span class="lgc-cut" title={row.what}>{row.what}</span></td>
                            <td data-label={t('Program')}>
                                <button class="lgc-linklike lgc-cut" title={row.program + (row.target ? ' → ' + row.target : '')} onclick={() => onopen(row.programKey)} data-testid="schedule-program">{row.program}{row.target ? ' → ' + row.target : ''}</button>
                            </td>
                            <td data-label={t('Next time')}><span class="lgc-cut" class:lgc-muted={!next} title={next || row.off} data-testid="schedule-next">{next || (row.off ? t('not armed') : '–')}</span></td>
                            <td data-label={t('That is')}>
                                <span class="lgc-rel" title={row.off}>
                                    <span class="lgc-cut" class:lgc-muted={!row.next} data-testid="schedule-relative">{row.next ? relative(schedule.now, row.next) : row.off || '–'}</span>
                                    <span class="lgc-bar lgc-rel-bar" class:lgc-rel-none={row.progress === null}><span class="lgc-bar-fill ok" style={`width: ${Math.round((row.progress ?? 0) * 100)}%`}></span></span>
                                </span>
                            </td>
                            <td data-label={t('Last time')}><span class="lgc-cut" class:lgc-muted={!last} title={last} data-testid="schedule-last">{last || '–'}</span></td>
                            <td class="lgc-cell-actions lgc-col-actions">
                                {#if row.pending !== undefined}
                                    {@const id = row.pending}
                                    <button class="hmm-button lgc-ed-icon" onclick={() => onrun(id)} title={t('Carry it out now')} aria-label={t('Carry it out now')} data-testid="pending-run">▶</button>
                                    <button class="hmm-button lgc-ed-icon" onclick={() => oncancel(id)} title={t('Cancel: it will not happen')} aria-label={t('Cancel: it will not happen')} data-testid="pending-cancel">✕</button>
                                {/if}
                            </td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
        {#if rows.length === 0}
            <p class="lgc-muted" data-testid="schedule-empty">
                {schedule.timers.length + schedule.pending.length + (schedule.holds ?? []).length === 0 ? t('Nothing is armed: no program has a time module, and no delayed activity is pending.') : t('Nothing matches.')}
            </p>
        {/if}
    {:else if !error}
        <p class="lgc-muted">{t('Loading…')}</p>
    {/if}
</section>
