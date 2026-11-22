<script lang="ts">
    // A time module in a form: how it repeats, when on its day (a clock time, the sun, all
    // day), a point or a range, and how long it is valid. Below it the next times it would
    // fire at, asked from the addon while the form is filled in.
    import {Dialog} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {ApiError, post} from './api.js';
    import {ASTRO_KINDS, astroWithOffset, TIME_KINDS, TWILIGHTS, timeFormOf, timeOf, timeProblem, WEEKDAYS} from './editor.js';
    import {edgeText, timeText, wallTime, type TimeEdge, type TimeModule} from './model.js';

    let {time, onsave, onclose}: {time: TimeModule | null; onsave: (m: TimeModule) => void; onclose: () => void} = $props();

    // svelte-ignore state_referenced_locally
    let form = $state(timeFormOf(time));
    let tried = $state(false);
    let next = $state<TimeEdge[]>([]);
    let note = $state('');

    const module = $derived(timeOf(form));
    const problem = $derived(timeProblem(module));
    const astro = $derived(form.when !== 'at' && form.when !== 'all_day');

    // the next times: asked a moment after the last change, and only for a module that can run
    $effect(() => {
        const m = module;
        if (problem) {
            next = [];
            note = '';
            return;
        }
        const timer = setTimeout(() => {
            post<{next: TimeEdge[]; no_position: boolean}>('api/v1/time/preview', {time: m}).then(
                (r) => {
                    next = r.next;
                    note = r.no_position ? t('Sunrise and sunset need the location of the system: without it this time module never triggers.') : '';
                },
                (e: unknown) => {
                    next = [];
                    note = e instanceof ApiError ? e.message : String(e);
                },
            );
        }, 250);
        return () => clearTimeout(timer);
    });

    function save() {
        tried = true;
        if (!problem) onsave(module);
    }

    function toggleDay(day: string, on: boolean) {
        form.weekdays = on ? [...form.weekdays, day] : form.weekdays.filter((d) => d !== day);
    }
</script>

<Dialog open title={t('Time control')} width="40rem" height="46rem" {onclose} testId="time-dialog">
    <form class="lgc-form lgc-tm-form" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <label class="lgc-label" for="tm-kind">{t('Repeats')} <HelpLink topic="editor-time" /></label>
        <select id="tm-kind" class="hmm-select" bind:value={form.kind} data-testid="tm-kind">
            {#each TIME_KINDS as kind (kind)}<option value={kind}>{t('timekind.' + kind)}</option>{/each}
        </select>

        {#if form.kind === 'once'}
            <label class="lgc-label" for="tm-date">{t('Date')}</label>
            <input id="tm-date" class="hmm-input lgc-time" type="text" placeholder="2026-12-24" bind:value={form.date} data-testid="tm-date" />
        {:else if form.kind === 'interval'}
            <label class="lgc-label" for="tm-interval">{t('Every')}</label>
            <div class="lgc-row">
                <input id="tm-interval" class="hmm-input lgc-narrow" type="text" inputmode="numeric" bind:value={form.intervalN} data-testid="tm-interval" />
                <select class="hmm-select" aria-label={t('Unit')} bind:value={form.intervalUnit} data-testid="tm-interval-unit">
                    {#each ['s', 'min', 'h'] as unit (unit)}<option value={unit}>{t('unit.' + unit)}</option>{/each}
                </select>
            </div>
        {:else if form.kind === 'daily'}
            <label class="lgc-label" for="tm-days">{t('On')}</label>
            <select id="tm-days" class="hmm-select" bind:value={form.days} data-testid="tm-days">
                <option value="all">{t('every day')}</option>
                <option value="workdays">{t('on workdays')}</option>
                <option value="weekend">{t('at the weekend')}</option>
            </select>
        {:else if form.kind === 'weekly'}
            <span class="lgc-label">{t('On')}</span>
            <div class="lgc-checks">
                {#each WEEKDAYS as day (day)}
                    <label class="lgc-check"><input type="checkbox" checked={form.weekdays.includes(day)} onchange={(e) => toggleDay(day, e.currentTarget.checked)} data-testid="tm-day-{day}" /> {t('day.' + day)}</label>
                {/each}
            </div>
        {:else}
            <label class="lgc-label" for="tm-by">{t('On')}</label>
            <div class="lgc-row">
                <select id="tm-by" class="hmm-select" bind:value={form.by} data-testid="tm-by">
                    <option value="day">{t('the day of the month')}</option>
                    <option value="weekday">{t('a weekday of the month')}</option>
                </select>
                {#if form.by === 'day'}
                    <input class="hmm-input lgc-narrow" type="text" inputmode="numeric" aria-label={t('the day of the month')} bind:value={form.day} data-testid="tm-day" />
                {:else}
                    <select class="hmm-select" aria-label={t('Which one')} bind:value={form.nth} data-testid="tm-nth">
                        {#each ['1', '2', '3', '4', '5'] as n (n)}<option value={n}>{n}.</option>{/each}
                    </select>
                    <select class="hmm-select" aria-label={t('Weekday')} bind:value={form.weekday} data-testid="tm-weekday">
                        {#each WEEKDAYS as day (day)}<option value={day}>{t('day.' + day)}</option>{/each}
                    </select>
                {/if}
            </div>
            {#if form.kind === 'yearly'}
                <label class="lgc-label" for="tm-month">{t('Month')}</label>
                <select id="tm-month" class="hmm-select" bind:value={form.month} data-testid="tm-month">
                    {#each Array.from({length: 12}, (_, i) => String(i + 1)) as n (n)}<option value={n}>{n}</option>{/each}
                </select>
            {/if}
        {/if}
        {#if form.kind === 'daily' || form.kind === 'weekly' || form.kind === 'monthly'}
            <label class="lgc-label" for="tm-every">{form.kind === 'daily' ? t('Every … days') : form.kind === 'weekly' ? t('Every … weeks') : t('Every … months')}</label>
            <input id="tm-every" class="hmm-input lgc-narrow" type="text" inputmode="numeric" placeholder="1" bind:value={form.every} data-testid="tm-every" />
        {/if}

        <label class="lgc-label" for="tm-when">{form.kind === 'interval' ? t('Beginning at') : t('At')}</label>
        <div class="lgc-row">
            <select id="tm-when" class="hmm-select" bind:value={form.when} data-testid="tm-when">
                <option value="at">{t('a time of day')}</option>
                <option value="all_day">{t('all day')}</option>
                {#each ASTRO_KINDS as kind (kind)}<option value={kind}>{t('astro.' + kind)}</option>{/each}
            </select>
            {#if form.when === 'at'}
                <input class="hmm-input lgc-time" type="text" placeholder="07:00" aria-label={t('a time of day')} bind:value={form.at} data-testid="tm-at" />
            {:else if astroWithOffset(form.when)}
                <input class="hmm-input lgc-narrow" type="text" inputmode="numeric" placeholder="0" aria-label={t('Offset in minutes')} bind:value={form.offsetMin} data-testid="tm-offset" />
                <span class="lgc-small lgc-muted">{t('min offset')}</span>
            {/if}
        </div>
        {#if form.when !== 'all_day' && form.kind !== 'interval'}
            <span class="lgc-label">{astro ? t('Until') : t('Point or range')}</span>
            <div class="lgc-row">
                <label class="lgc-check"><input type="checkbox" bind:checked={form.ranged} disabled={astro && form.point} data-testid="tm-ranged" /> {astro ? t('for a set time instead of to the sun\'s other event:') : t('a range, lasting')}</label>
                <input class="hmm-input lgc-narrow" type="text" inputmode="numeric" aria-label={t('Duration')} bind:value={form.durationN} disabled={!form.ranged} data-testid="tm-duration" />
                <select class="hmm-select" aria-label={t('Unit')} bind:value={form.durationUnit} disabled={!form.ranged} data-testid="tm-duration-unit">
                    {#each ['s', 'min', 'h'] as unit (unit)}<option value={unit}>{t('unit.' + unit)}</option>{/each}
                </select>
            </div>
        {/if}
        {#if astro && form.kind !== 'interval'}
            <label class="lgc-label" for="tm-shape">{t('Point or range')} <HelpLink topic="additions" /></label>
            <select id="tm-shape" class="hmm-select" value={form.point && !form.ranged ? 'point' : 'range'} onchange={(e) => { form.point = e.currentTarget.value === 'point'; if (form.point) form.ranged = false; }} data-testid="tm-shape">
                <option value="range">{t('a range, as on the CCU')}</option>
                <option value="point">{t('a point in time')}</option>
            </select>
        {/if}
        {#if astro}
            <label class="lgc-label" for="tm-twilight">{t('Sunrise and sunset are')}</label>
            <select id="tm-twilight" class="hmm-select" bind:value={form.twilight} data-testid="tm-twilight">
                <option value="">{t('the sun at the horizon')}</option>
                {#each TWILIGHTS as twilight (twilight)}<option value={twilight}>{t('twilight.' + twilight)}</option>{/each}
            </select>
        {/if}

        <label class="lgc-label" for="tm-from">{t('Valid from')}</label>
        <input id="tm-from" class="hmm-input lgc-time" type="text" placeholder="2026-01-01" bind:value={form.from} data-testid="tm-from" />
        <label class="lgc-label" for="tm-until">{t('Valid until')}</label>
        <input id="tm-until" class="hmm-input lgc-time" type="text" placeholder="2026-12-31" bind:value={form.until} data-testid="tm-until" />
        <label class="lgc-label" for="tm-count">{t('Number of times')}</label>
        <input id="tm-count" class="hmm-input lgc-narrow" type="text" inputmode="numeric" bind:value={form.count} data-testid="tm-count" />

        <span class="lgc-label">{t('In words')}</span>
        <div class="lgc-tm-words" data-testid="tm-words">{problem ? '–' : timeText(module)}</div>
        <span class="lgc-label">{t('Next times')} <HelpLink topic="time-next" /></span>
        <div class="lgc-tm-next lgc-small" data-testid="tm-next">
            {#each next as edge, i (i)}<div>{wallTime(edge.at, i18n.language)}{edge.kind === 'point' ? '' : ' (' + edgeText(edge.kind) + ')'}</div>{/each}
            {#if note}<div class="lgc-muted">{note}</div>{/if}
        </div>
        <div class="lgc-span lgc-error-inline lgc-reserve" role="alert" data-testid="tm-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="tm-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
