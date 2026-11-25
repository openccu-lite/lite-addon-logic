<script lang="ts">
    // The import: choose the source, read it into a dry run, look at the report, then take it
    // over - or not. Nothing is written before "Import".
    import {i18n, t} from 'occulite-ui/i18n';

    import {apiUrl} from './api.js';
    import HelpLink from './HelpLink.svelte';
    import {importStatus, noteText, reportHasScript, reportTotals, type Codes, type Preview, type Report} from './model.js';

    let {
        system,
        preview,
        applied,
        codes,
        busy,
        error,
        onread,
        onapply,
        ondiscard,
        ondone,
    }: {
        system: boolean;
        preview: Preview | null;
        applied: Report | null;
        codes: Codes;
        busy: boolean;
        error: string;
        onread: (file: File | null, allInactive: boolean) => void;
        onapply: () => void;
        ondiscard: () => void;
        ondone: () => void;
    } = $props();

    let file = $state<File | null>(null);
    let allInactive = $state(false);

    const report = $derived(preview?.report ?? null);
    const totals = $derived(report ? reportTotals(report) : null);
    const note = (n: {code: string; args?: string[]}) => noteText(n, codes, i18n.language);
</script>

{#if applied}
    {@const done = reportTotals(applied)}
    <section class="lgc-section" data-testid="import-done">
        <h2>{t('Imported')}</h2>
        <p>{t('Imported: {programs} ({inactive} of them inactive) and {variables}.', {programs: t('{n} programs', {n: done.imported + done.inactive}, done.imported + done.inactive), inactive: done.inactive, variables: t('{n} system variables', {n: done.variables}, done.variables)})}</p>
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" onclick={ondone} data-testid="import-to-programs">{t('To the programs')}</button>
        </div>
    </section>
{:else if report && totals}
    <section class="lgc-section lgc-section-wide" data-testid="import-report">
        <div class="lgc-section-head">
            <h2>{t('Dry run: nothing is written yet')}</h2>
            <HelpLink topic="import-report" />
        </div>
        <p data-testid="import-summary">
            {t('Would be imported: {programs} ({inactive} of them inactive) and {variables}; not imported: {skipped}.', {programs: t('{n} programs', {n: totals.imported + totals.inactive}, totals.imported + totals.inactive), inactive: totals.inactive, variables: t('{n} system variables', {n: totals.variables}, totals.variables), skipped: t('{n} programs', {n: totals.skipped}, totals.skipped)})}
            {#if report.all_inactive}<strong>{t('Every program is imported inactive.')}</strong>{/if}
        </p>
        {#if totals.missing > 0}
            <div class="lgc-warning" data-testid="import-missing">{t('{n} channels that programs use do not exist on this system: those programs stay inactive.', {n: totals.missing}, totals.missing)}</div>
        {/if}
        {#if reportHasScript(report)}
            <div class="lgc-warning" data-testid="import-scripts">
                {t('Some programs hold a script. Scripts do not run here: such a program stays inactive and its script is kept as text. For such tasks Node-RED (RedMatic), Home Assistant or ioBroker are the right tools.')}
                <HelpLink topic="import-what" />
            </div>
        {/if}
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" onclick={onapply} disabled={busy} data-testid="import-apply">{t('Import')}</button>
            <button class="hmm-button" onclick={ondiscard} disabled={busy} data-testid="import-discard">{t('Discard')}</button>
            <a class="hmm-button" href={apiUrl(`api/v1/import/${preview?.id}/report.md?lang=${i18n.language}`)} download data-testid="import-download">{t('Save the report')}</a>
            <span class="lgc-error-inline lgc-reserve" role="alert" data-testid="import-error">{error}</span>
        </div>

        <h3>{t('Programs')}</h3>
        <p class="lgc-help" data-testid="import-quirks">{t('The notes say where a program does what it did on the CCU and may surprise all the same. The guide lists these quirks in plain words.')} <HelpLink topic="quirks" /></p>
        <div class="lgc-tablewrap">
            <table class="lgc-grid lgc-cardgrid" data-testid="import-programs">
                <thead><tr><th class="lgc-col-state">{t('Result')}</th><th>{t('Program')}</th><th>{t('Notes')}</th></tr></thead>
                <tbody>
                    {#each report.programs as p (p.id)}
                        {@const st = importStatus(p)}
                        <tr data-testid="import-program">
                            <td class="lgc-col-state" data-label={t('Result')}><span class={'lgc-pill lgc-pill-wide ' + st.kind}>{st.label}</span></td>
                            <td data-label={t('Program')}>{p.name}{#if p.updated}<div class="lgc-muted lgc-small">{t('updates the file of an earlier import')}</div>{/if}</td>
                            <td data-label={t('Notes')}>
                                <ul class="lgc-reasons">
                                    {#each p.reasons ?? [] as r (r.code + (r.args ?? []).join())}<li class="lgc-reason">{note(r)}</li>{/each}
                                    {#each p.notes ?? [] as n (n.code + (n.args ?? []).join())}<li>{note(n)}</li>{/each}
                                </ul>
                            </td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>

        <details class="lgc-details" data-testid="import-variables">
            <summary>{t('System variables')} ({totals.variables})</summary>
            <div class="lgc-tablewrap">
                <table class="lgc-grid lgc-grid-compact lgc-cardgrid">
                    <thead><tr><th>{t('Name')}</th><th>{t('Type')}</th><th>{t('Value')}</th><th>{t('Notes')}</th></tr></thead>
                    <tbody>
                        {#each report.variables as v (v.id)}
                            <tr class:lgc-off={v.status !== 'imported'}>
                                <td data-label={t('Name')}>{v.name}</td>
                                <td data-label={t('Type')}>{v.type ? t('type.' + v.type) : '–'}</td>
                                <td data-label={t('Value')}><span class="lgc-clip" title={v.value ?? ''}>{v.value ?? '–'}</span></td>
                                <td data-label={t('Notes')}>{v.status !== 'imported' ? t('not imported') + ': ' : ''}{(v.notes ?? []).map(note).join('; ')}</td>
                            </tr>
                        {/each}
                    </tbody>
                </table>
            </div>
        </details>

        {#if report.devices.length > 0}
            <details class="lgc-details" open={totals.missing > 0} data-testid="import-devices">
                <summary>{t('Devices that programs use')} ({report.devices.length})</summary>
                <div class="lgc-tablewrap">
                    <table class="lgc-grid lgc-grid-compact lgc-cardgrid">
                        <thead><tr><th>{t('Channel')}</th><th>{t('Name on the CCU')}</th><th>{t('Device')}</th><th>{t('Rooms')}</th><th>{t('On this system')}</th><th>{t('Programs')}</th></tr></thead>
                        <tbody>
                            {#each report.devices as d (d.channel)}
                                <tr data-testid="import-device">
                                    <td data-label={t('Channel')} class="lgc-mono lgc-small">{d.channel}</td>
                                    <td data-label={t('Name on the CCU')}>{d.name ?? '–'}</td>
                                    <td data-label={t('Device')}>{d.device ?? '–'}{d.type ? ` (${d.type})` : ''}</td>
                                    <td data-label={t('Rooms')}>{(d.rooms ?? []).join(', ') || '–'}</td>
                                    <td data-label={t('On this system')}><span class={'lgc-pill ' + (d.known ? 'ok' : 'bad')}>{d.known ? t('present') : t('missing')}</span></td>
                                    <td data-label={t('Programs')}>{d.programs.join(', ')}</td>
                                </tr>
                            {/each}
                        </tbody>
                    </table>
                </div>
            </details>
        {/if}

        {#if (report.repairs ?? []).length > 0}
            <details class="lgc-details">
                <summary>{t('Repaired names')} ({report.repairs?.length})</summary>
                <ul>{#each report.repairs ?? [] as r (r.ID)}<li>{r.Before} → {r.After}</li>{/each}</ul>
            </details>
        {/if}
        <p class="lgc-muted lgc-small">
            {t('Source')}: {report.kind === 'sbk' ? t('system backup (.sbk)') : t('ReGa database')}{report.firmware ? ' · ' + t('firmware {v}', {v: report.firmware}) : ''}
            · {report.location.latitude ? t('location {lat}, {lon}', {lat: report.location.latitude, lon: report.location.longitude ?? ''}) : t('no location in the source')}
        </p>
    </section>
{:else}
    <section class="lgc-section" data-testid="import-start">
        <div class="lgc-section-head">
            <h2>{t('Import programs and system variables of a CCU')}</h2>
            <HelpLink topic="import" />
        </div>
        <p>{t('The import only reads. It first shows what it would do; nothing is written before you say so.')}</p>
        <p class="lgc-help" data-testid="import-from-ccu">{t('Coming from a CCU? The guide has the switch step by step.')} <HelpLink topic="from-ccu" /></p>

        <h3>{t('From a file')} <HelpLink topic="import-sources" /></h3>
        <p class="lgc-help">{t('A system backup (.sbk) of a CCU or OpenCCU, or its ReGa database (homematic.regadom).')}</p>
        <div class="lgc-row">
            <input type="file" accept=".sbk,.regadom,application/octet-stream" onchange={(e) => (file = e.currentTarget.files?.[0] ?? null)} data-testid="import-file" />
            <button class="hmm-button lgc-primary" onclick={() => onread(file, allInactive)} disabled={!file || busy} data-testid="import-read-file">{t('Read the file')}</button>
        </div>

        <h3>{t('From the database kept on the system')}</h3>
        {#if system}
            <p class="lgc-help">{t('After the switch from (Open)CCU the old ReGa database stays on the system. It is as it was on the day of the switch.')}</p>
            <div class="lgc-row">
                <button class="hmm-button lgc-primary" onclick={() => onread(null, allInactive)} disabled={busy} data-testid="import-read-system">{t('Read the kept database')}</button>
            </div>
        {:else}
            <p class="lgc-help" data-testid="import-no-system">{t('The system keeps no ReGa database of a CCU.')}</p>
        {/if}

        <h3>{t('Options')} <HelpLink topic="import-dry-run" /></h3>
        <label class="lgc-check"><input type="checkbox" bind:checked={allInactive} data-testid="import-all-inactive" /> {t('import every program inactive')}</label>
        <p class="lgc-help">{t('The right choice for the backup of somebody else\'s CCU: nothing starts switching.')}</p>
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="import-error">{busy ? t('Reading…') : error}</div>
    </section>
{/if}
