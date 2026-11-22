<script lang="ts">
    // One program as the CCU shows it: "Bedingung: Wenn…", "Sonst, wenn…", "Sonst…", each with
    // its condition lines and its "Aktivität: Dann…". From here it is edited, copied or removed.
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import DryRunDialog from './DryRunDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import RunHistory from './RunHistory.svelte';
    import TriggerList from './TriggerList.svelte';
    import VersionsDialog from './VersionsDialog.svelte';
    import {get} from './api.js';
    import {actionText, blockText, dateTime, edgeText, lineNumbers, lineText, programState, ruleHeading, runTrigger, wallTime, type Context, type Overview, type ProgramView, type TimeLine} from './model.js';

    let {
        view,
        ctx,
        overview,
        onback,
        onactive,
        onatstart,
        onrun,
        onsettings,
        onedit,
        oncopy,
        ondelete,
        onrestored,
    }: {
        view: ProgramView;
        ctx: Context;
        overview: Overview;
        onback: () => void;
        onactive: (key: string, active: boolean) => void;
        onatstart: (key: string, atStart: boolean) => void;
        onrun: (key: string, mode: string) => void;
        onsettings: () => void;
        onedit: () => void;
        oncopy: () => void;
        ondelete: () => void;
        /** A version was brought back: the page's data is asked again. */
        onrestored: () => void;
    } = $props();

    let asking = $state(false);
    let trying = $state(false);
    let versions = $state(false);

    const p = $derived(view.program);
    const st = $derived(programState(view));
    const numbers = $derived(lineNumbers(p));

    // the time module lines as the engine sees them: asked again whenever the page's data is
    let times = $state<Record<number, TimeLine>>({});
    $effect(() => {
        const key = view.program.key;
        void view.state;
        if (!view.time) {
            times = {};
            return;
        }
        get<{lines: TimeLine[]}>(`api/v1/programs/${key}/times`).then(
            (r) => (times = Object.fromEntries(r.lines.map((l) => [l.line, l]))),
            () => {},
        );
    });
</script>

<section class="lgc-section lgc-section-wide" data-testid="program-detail">
    <div class="lgc-section-head">
        <button class="hmm-button" onclick={onback} data-testid="program-back">‹ {t('Programs')}</button>
        <h2 data-testid="program-name">{p.name}</h2>
        <span class={'lgc-pill ' + st.kind}>{st.label}</span>
        <HelpLink topic="programs" />
        <span class="lgc-spacer"></span>
        <label class="lgc-switch">
            <input type="checkbox" checked={p.active} disabled={view.script || view.problems.length > 0} onchange={(e) => onactive(p.key, e.currentTarget.checked)} data-testid="detail-active" />
            <span>{t('active')}</span>
        </label>
        <label class="lgc-switch">
            <input type="checkbox" checked={!!p.at_start} onchange={(e) => onatstart(p.key, e.currentTarget.checked)} data-testid="detail-at-start" />
            <span>{t('evaluate at system start')}</span>
        </label>
        <HelpLink topic="start" />
    </div>
    {#if p.note}<p class="lgc-muted" data-testid="program-note">{p.note}</p>{/if}

    <div class="lgc-kv lgc-kv-compact">
        <span>{t('Last run')}</span>
        <span data-testid="detail-last-run">
            {#if view.state.last_run}{dateTime(view.state.last_run.time, i18n.language)} · {blockText(view.state.last_run.block)} · <span data-testid="detail-last-trigger">{runTrigger(view.state.last_run.trigger, ctx)}</span>{:else}–{/if}
        </span>
        {#if view.time}
            <span>{t('Next time')} <HelpLink topic="time-next" /></span>
            <span data-testid="detail-next-time">{wallTime(view.state.next_time, i18n.language)}</span>
        {/if}
        <span>{t('Properties')}</span>
        <span>
            {p.visible ? t('visible') : t('not visible')} · {p.operable ? t('operable') : t('not operable')}{p.internal ? ' · ' + t('internal') : ''} ·
            {p.at_start ? t('evaluated at system start') : t('not evaluated at system start')}
        </span>
    </div>

    {#each view.problems as problem (problem)}
        <div class="lgc-error" role="alert" data-testid="program-problem">{problem}</div>
    {/each}
    {#if view.state.failures}
        <div class="lgc-warning">{t('{n} writes refused', {n: view.state.failures}, view.state.failures)}: {view.state.last_failure}</div>
    {/if}
    {#if view.script}
        <div class="lgc-warning" data-testid="script-pointer">
            {t('This program holds a script. Scripts do not run here, so the program stays inactive; the script is kept as text. For such tasks Node-RED (RedMatic), Home Assistant or ioBroker are the right tools.')}
            <HelpLink topic="import-what" />
        </div>
    {/if}
    {#if view.state.no_position}
        <div class="lgc-warning" data-testid="no-location">
            {t('Sunrise and sunset need the location of the system: without it this time module never triggers.')}
            <button class="lgc-linklike" onclick={onsettings} data-testid="go-settings">{t('Set the location')}</button>
            <HelpLink topic="time-location" />
        </div>
    {/if}
    {#if view.state.no_clock}
        <div class="lgc-warning" data-testid="no-clock">
            {t('The clock of the system is not set yet: time modules wait until it is.')}
            <HelpLink topic="time-clock" />
        </div>
    {/if}

    <div class="lgc-actions">
        <button class="hmm-button" onclick={() => onrun(p.key, 'check')} disabled={view.problems.length > 0} data-testid="run-check">{t('Evaluate now')}</button>
        <button class="hmm-button" onclick={() => onrun(p.key, 'then')} disabled={view.problems.length > 0} data-testid="run-then">{t('Run the first "Then"')}</button>
        <HelpLink topic="program-run" />
        <button class="hmm-button" onclick={() => (trying = true)} disabled={view.problems.length > 0} title={t('What the program would do - nothing is sent or written.')} data-testid="dry-run">{t('Dry run…')}</button>
        <HelpLink topic="dry-run" />
        <span class="lgc-spacer"></span>
        <button class="hmm-button lgc-primary" onclick={onedit} data-testid="program-edit">{t('Edit')}</button>
        <button class="hmm-button" onclick={oncopy} data-testid="program-copy">{t('Copy')}</button>
        <button class="hmm-button" onclick={() => (versions = true)} data-testid="program-versions">{t('Versions…')}</button>
        <button class="hmm-button lgc-danger" onclick={() => (asking = true)} data-testid="program-delete">{t('Delete')}</button>
        <HelpLink topic="editor" />
    </div>

    {#each p.rules as rule, ri (ri)}
        <div class="lgc-rule" data-testid="rule-{ri}">
            <h3>{ruleHeading(rule, ri)} {#if ri === 0}<HelpLink topic="triggers" />{/if}{#if rule.else}<HelpLink topic="else" />{/if}</h3>
            {#if rule.when}
                {#if rule.when.groups.length === 0}
                    <p class="lgc-muted lgc-indent">{t('No condition: always true.')}</p>
                {/if}
                {#each rule.when.groups as group, gi (gi)}
                    {#if gi > 0}<div class="lgc-junction lgc-junction-outer">{rule.when.op === 'all' ? t('AND') : t('OR')}</div>{/if}
                    <div class="lgc-group">
                        {#each group.lines as line, li (li)}
                            {@const lt = lineText(line, ctx)}
                            {#if li > 0}<div class="lgc-junction">{group.op === 'all' ? t('AND') : t('OR')}</div>{/if}
                            <div class="lgc-line" class:lgc-line-unknown={lt.unknown} data-testid="line">
                                <span class="lgc-line-mark" title={lt.timeCheck ? t('A time module triggers the program at its times whatever its mode says - as on the CCU.') : lt.triggers ? t('This line triggers the program.') : ''} data-testid="line-mark">{lt.triggers ? '⚡' : ''}</span>
                                <span class="lgc-line-kind">{lt.kind}</span>
                                <span class="lgc-line-subject">{lt.subject}{#if lt.param} <span class="lgc-mono lgc-small">{lt.param}</span>{/if}</span>
                                <span class="lgc-line-comparison">{#if lt.not}<strong>{t('not')}</strong> {/if}{lt.comparison}</span>
                                <span class="lgc-line-mode">{lt.mode}</span>
                            </div>
                            {#if line.time}
                                {@const tl = times[numbers[ri]?.[gi]?.[li] ?? 0]}
                                <div class="lgc-timeline lgc-small" data-testid="time-line">
                                    <span class="lgc-timeline-state">{#if tl?.active}<span class="lgc-flag" data-testid="time-active">{t('true now')}</span>{/if}</span>
                                    <span class="lgc-timeline-next" data-testid="time-next">
                                        {t('next')}:
                                        {#each tl?.next ?? [] as edge, ei (ei)}{ei > 0 ? ' · ' : ''}<span class="lgc-nowrap">{wallTime(edge.at, i18n.language)}{edge.kind === 'point' ? '' : ' (' + edgeText(edge.kind) + ')'}</span>{:else} –{/each}
                                    </span>
                                    <span class="lgc-timeline-last" data-testid="time-last">{t('last')}: {#if tl?.last}{wallTime(tl.last.at, i18n.language)}{tl.last.kind === 'point' ? '' : ' (' + edgeText(tl.last.kind) + ')'}{:else}–{/if}</span>
                                    <HelpLink topic="time" />
                                </div>
                            {/if}
                        {/each}
                    </div>
                {/each}
            {/if}
            <h4>
                {rule.else ? t('Activity:') : t('Activity: Then…')}
                {#if rule.cancel_pending}<span class="lgc-flag" title={t('Before it runs, running delays for the targets of this block are ended - other programs\' too.')} data-testid="cancel-pending">{t('ends running delays')}</span>{/if}
                {#if ri === 0}<HelpLink topic="delays" />{/if}
            </h4>
            {#if rule.then.length === 0}
                <p class="lgc-muted lgc-indent">{t('(no activity)')}</p>
            {/if}
            {#each rule.then as action, ai (ai)}
                {@const at = actionText(action, ctx)}
                <div class="lgc-line" class:lgc-line-unknown={at.unknown} class:lgc-line-skipped={at.skipped} data-testid="action">
                    <span class="lgc-line-mark">{ai + 1}</span>
                    <span class="lgc-line-kind">{at.kind}</span>
                    <span class="lgc-line-subject">{at.subject}{#if at.param} <span class="lgc-mono lgc-small">{at.param}</span>{/if}</span>
                    <span class="lgc-line-comparison">{at.what}</span>
                    <span class="lgc-line-mode">{at.timing}</span>
                </div>
                {#if at.script}<pre class="lgc-code lgc-script" data-testid="script-text">{at.script}</pre>{/if}
            {/each}
        </div>
    {/each}

    <div class="lgc-rule" data-testid="program-triggers">
        <h3>{t('What this program looks at')} <HelpLink topic="triggers-page" /></h3>
        <TriggerList {overview} program={p.key} />
    </div>

    <div class="lgc-rule" data-testid="program-history">
        <h3>{t('History')} <HelpLink topic="trace" /></h3>
        <RunHistory {overview} program={p.key} />
    </div>
</section>
<ConfirmDialog open={asking} title={t('Delete the program?')} message={t('{name} is removed. Its earlier versions stay in the history.', {name: p.name})} confirmLabel={t('Delete')} danger testId="confirm-program-delete"
    onconfirm={() => { asking = false; ondelete(); }} onclose={() => (asking = false)} />
{#if trying}<DryRunDialog program={p} {ctx} onclose={() => (trying = false)} />{/if}
{#if versions}<VersionsDialog kind="programs" key={p.key} name={p.name} onclose={() => (versions = false)} onrestored={onrestored} />{/if}
