<script lang="ts">
    // The system variables: the alarm messages on top, then every variable with its value, which
    // can be set here; create, change, remove.
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelperDialog from './HelperDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import Marks from './Marks.svelte';
    import VariableDialog from './VariableDialog.svelte';
    import VersionsDialog from './VersionsDialog.svelte';
    import {computedText, context, dateTime, matches, nameTitle, parseValue, valueText, variableMarks, variableName, type Overview, type Value, type VariableDef, type VariableView} from './model.js';

    let {
        overview,
        onvalue,
        onacknowledge,
        onacknowledgeall,
        onsave,
        ondelete,
        ondeletehelper,
        onrefresh,
    }: {
        overview: Overview;
        onvalue: (key: string, value: Value) => Promise<boolean>;
        onacknowledge: (key: string) => void;
        onacknowledgeall: () => void;
        onsave: (key: string, body: {variable: VariableDef; renames: Record<string, string>; value?: Value}) => Promise<string>;
        ondelete: (key: string) => void;
        /** Removes the device helper a variable belongs to, with all its variables. */
        ondeletehelper: (key: string) => void;
        /** A version was brought back: the page's data is asked again. */
        onrefresh: () => void;
    } = $props();

    let query = $state('');
    let internal = $state(false);
    let editing = $state<VariableView | null>(null);
    let creating = $state(false);
    let removing = $state<VariableView | null>(null);
    let versionsOf = $state<VariableView | null>(null);
    let addingHelper = $state(false);
    // the variables of the helper the variable to remove belongs to
    const helperOf = (v: VariableView | null) => (v?.def.helper ? overview.variables.filter((o) => o.def.helper?.kind === v.def.helper?.kind && o.def.helper?.channel === v.def.helper?.channel) : []);
    // the value being typed, per variable, and what is wrong with it
    let drafts = $state<Record<string, string>>({});
    let errors = $state<Record<string, string>>({});

    const shown = $derived(overview.variables.filter((v) => (internal || !v.def.internal) && matches(query, variableName(v.def), v.def.note, v.def.key)));
    const ctx = $derived(context(overview));
    const hidden = $derived(overview.variables.filter((v) => v.def.internal).length);
    // the variable whose typed value was refused last
    let failedKey = $state('');
    const failed = $derived(errors[failedKey] === undefined ? undefined : overview.variables.find((v) => v.def.key === failedKey));
    const programName = (key: string) => overview.programs.find((p) => p.program.key === key)?.program.name ?? key;

    async function set(v: VariableView, input: string | boolean) {
        const parsed = parseValue(v.def, input);
        if (parsed.error) {
            errors[v.def.key] = parsed.error;
            failedKey = v.def.key;
            return;
        }
        delete errors[v.def.key];
        if (await onvalue(v.def.key, parsed.value)) delete drafts[v.def.key];
    }
</script>

{#if overview.alarms.length > 0}
    <section class="lgc-section lgc-section-wide" data-testid="alarm-messages">
        <div class="lgc-section-head">
            <h2>{t('Alarm messages')} <span class="lgc-count">{overview.alarms.length}</span></h2>
            <HelpLink topic="alarms" />
            <span class="lgc-spacer"></span>
            <button class="hmm-button" onclick={onacknowledgeall} data-testid="acknowledge-all">{t('Acknowledge all')}</button>
        </div>
        <div class="lgc-tablewrap">
            <table class="lgc-grid lgc-cardgrid lgc-fixed lgc-alarms">
                <thead>
                    <tr><th class="lgc-w-name">{t('Name')}</th><th>{t('Description')}</th><th class="lgc-w-stamp">{t('First')}</th><th class="lgc-w-stamp">{t('Last')}</th><th class="lgc-num lgc-w-num">{t('Count')}</th><th class="lgc-col-actions lgc-w-button"></th></tr>
                </thead>
                <tbody>
                    {#each overview.alarms as a (a.def.key)}
                        <tr data-testid="alarm-{a.def.key}">
                            <td data-label={t('Name')}><span class="lgc-cut" title={a.def.name}>{a.def.name}</span></td>
                            <td data-label={t('Description')}><span class="lgc-cut lgc-muted" title={a.def.note ?? ''}>{a.def.note || '–'}</span></td>
                            <td data-label={t('First')}><span class="lgc-cut">{dateTime(a.alarm?.first, i18n.language)}</span></td>
                            <td data-label={t('Last')}><span class="lgc-cut">{dateTime(a.time, i18n.language)}</span></td>
                            <td data-label={t('Count')} class="lgc-num"><span class="lgc-cut">{a.alarm?.count ?? 0}</span></td>
                            <td class="lgc-cell-actions lgc-col-actions"><button class="hmm-button" onclick={() => onacknowledge(a.def.key)} data-testid="acknowledge">{t('Acknowledge')}</button></td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
    </section>
{/if}

<section class="lgc-section lgc-section-wide">
    <div class="lgc-section-head">
        <h2>{t('System variables')}</h2>
        <HelpLink topic="variable-page" />
        <span class="lgc-spacer"></span>
        <input class="hmm-input lgc-search" type="search" placeholder={t('Search…')} aria-label={t('Search…')} bind:value={query} data-testid="variable-search" />
        <button class="hmm-button" onclick={() => (addingHelper = true)} data-testid="helper-new">{t('Add a device helper')}</button>
        <HelpLink topic="helpers" />
        <button class="hmm-button lgc-primary" onclick={() => (creating = true)} data-testid="variable-new">{t('New variable')}</button>
    </div>
    <!-- what is wrong with a typed value: one line that is always there, so no row grows -->
    <p class="lgc-error-inline lgc-small lgc-table-message" role="status" data-testid="value-error">{failed ? variableName(failed.def) + ': ' + errors[failed.def.key] : ''}</p>
    <div class="lgc-tablewrap">
        <table class="lgc-grid lgc-cardgrid lgc-fixed lgc-variables" data-testid="variable-list">
            <thead>
                <tr>
                    <th class="lgc-w-name">{t('Name')}</th>
                    <th class="lgc-c-note">{t('Description')}</th>
                    <th class="lgc-w-type">{t('Type')} <HelpLink topic="variable-types" /></th>
                    <th class="lgc-w-marks">{t('Marks')}</th>
                    <th class="lgc-num lgc-w-num">{t('Programs')}</th>
                    <th class="lgc-num lgc-w-value">{t('Value')} <HelpLink topic="variable-limits" /></th>
                    <th class="lgc-w-stamp">{t('Last written')}</th>
                    <th class="lgc-col-actions lgc-w-buttons"></th>
                </tr>
            </thead>
            <tbody>
                {#each shown as v (v.def.key)}
                    {@const users = v.users.map(programName).join(', ')}
                    <tr data-testid="variable-{v.def.key}">
                        <td data-label={t('Name')}><span class="lgc-cut" title={nameTitle(variableName(v.def), v.def.note)}>{variableName(v.def)}</span></td>
                        <td class="lgc-c-note" data-label={t('Description')}><span class="lgc-cut lgc-muted" title={v.def.note ?? ''}>{v.def.note || '–'}</span></td>
                        <td data-label={t('Type')}><span class="lgc-pill lgc-pill-type" data-testid="variable-type">{t('type.' + (v.def.computed ? 'computed' : v.def.type))}</span></td>
                        <td data-label={t('Marks')}><Marks marks={variableMarks(v, ctx)} /></td>
                        <td data-label={t('Programs')} class="lgc-num"><span class="lgc-cut" class:lgc-muted={v.users.length === 0} title={users} data-testid="variable-users">{v.users.length}</span></td>
                        <td data-label={t('Value')} class="lgc-num lgc-value-cell">
                            {#if v.system || v.def.computed || (v.def.helper && v.def.helper.role !== 'total')}
                                <span class="lgc-cut" title={v.def.computed ? computedText(v.def.computed, ctx) + ': ' + valueText(v.def, v.value) : valueText(v.def, v.value)} data-testid="value">{valueText(v.def, v.value)}</span>
                            {:else if v.def.type === 'bool' || v.def.type === 'alarm'}
                                <select class="hmm-select lgc-value-select" value={String(v.value === true)} onchange={(e) => set(v, e.currentTarget.value)} data-testid="value">
                                    <option value="false">{valueText(v.def, false)}</option>
                                    <option value="true">{valueText(v.def, true)}</option>
                                </select>
                            {:else if v.def.type === 'list'}
                                <select class="hmm-select lgc-value-select" value={String(v.value ?? '')} onchange={(e) => set(v, e.currentTarget.value)} data-testid="value">
                                    {#each v.def.entries ?? [] as entry (entry)}<option value={entry}>{entry}</option>{/each}
                                </select>
                            {:else}
                                <form class="lgc-value-form" onsubmit={(e) => { e.preventDefault(); set(v, drafts[v.def.key] ?? String(v.value ?? '')); }}>
                                    <input class="hmm-input" class:lgc-num-input={v.def.type === 'number'} class:lgc-invalid={errors[v.def.key] !== undefined} type="text" inputmode={v.def.type === 'number' ? 'decimal' : 'text'} value={drafts[v.def.key] ?? String(v.value ?? '')}
                                        oninput={(e) => (drafts[v.def.key] = e.currentTarget.value)} aria-label={t('Value')} aria-invalid={errors[v.def.key] !== undefined} title={errors[v.def.key] ?? String(v.value ?? '')} data-testid="value" />
                                    <span class="lgc-unit lgc-muted" title={v.def.unit ?? ''}>{v.def.unit ?? ''}</span>
                                    <button class="hmm-button" type="submit" disabled={drafts[v.def.key] === undefined} data-testid="value-set">{t('Set')}</button>
                                </form>
                            {/if}
                        </td>
                        <td data-label={t('Last written')}><span class="lgc-cut">{dateTime(v.time, i18n.language)}</span></td>
                        <td class="lgc-cell-actions lgc-col-actions">
                            {#if !v.system}
                                <button class="hmm-button" onclick={() => (editing = v)} data-testid="variable-edit">{t('Edit')}</button>
                                <button class="hmm-button" onclick={() => (removing = v)} disabled={v.users.length > 0} title={v.users.length > 0 ? t('Used by: {programs}', {programs: users}) : ''} data-testid="variable-delete">{t('Delete')}</button>
                            {/if}
                        </td>
                    </tr>
                {/each}
            </tbody>
        </table>
    </div>
    {#if shown.length === 0}
        <p class="lgc-muted">{t('No variable matches the search.')}</p>
    {/if}
    {#if hidden > 0}
        <label class="lgc-check lgc-small"><input type="checkbox" bind:checked={internal} data-testid="show-internal-variables" /> {t('show the {n} internal variables', {n: hidden}, hidden)}</label>
    {/if}
</section>

{#if creating || editing}
    <VariableDialog
        view={editing}
        variables={overview.variables}
        onclose={() => { creating = false; editing = null; }}
        onversions={() => { versionsOf = editing; editing = null; }}
        onsave={async (body) => {
            const error = await onsave(editing?.def.key ?? '', body);
            if (!error) { creating = false; editing = null; }
            return error;
        }}
    />
{/if}
{#if versionsOf}<VersionsDialog kind="variables" key={versionsOf.def.key} name={variableName(versionsOf.def)} onclose={() => (versionsOf = null)} onrestored={onrefresh} />{/if}
<ConfirmDialog
    open={removing !== null}
    title={removing?.def.helper ? t('Delete the device helper?') : t('Delete the variable?')}
    message={removing?.def.helper
        ? t('The device helper is removed with its variables and their values: {names}.', {names: helperOf(removing).map((o) => variableName(o.def)).join(', ')})
        : t('"{name}" and its value are removed.', {name: removing ? variableName(removing.def) : ''})}
    confirmLabel={t('Delete')}
    danger
    testId="confirm-delete"
    onconfirm={() => { if (removing) (removing.def.helper ? ondeletehelper : ondelete)(removing.def.key); removing = null; }}
    onclose={() => (removing = null)}
/>
{#if addingHelper}<HelperDialog onclose={() => (addingHelper = false)} oncreated={() => { addingHelper = false; onrefresh(); }} />{/if}
