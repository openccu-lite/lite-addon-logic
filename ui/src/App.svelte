<script lang="ts">
    // The page: the programs, the system variables, the import from a CCU. One answer of the
    // addon holds everything the tabs show; it is fetched again after every change and every
    // five seconds while the page is visible.
    import {onMount} from 'svelte';
    import {Notices, Tabs, ThemeSwitch} from 'occulite-ui';
    import {i18n, t} from 'occulite-ui/i18n';
    import {NoticesStore} from 'occulite-ui/notices';
    import {cycleTheme, theme} from 'occulite-ui/theme';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import HistoryTab from './HistoryTab.svelte';
    import ImportTab from './ImportTab.svelte';
    import ProgramsTab from './ProgramsTab.svelte';
    import ScheduleTab from './ScheduleTab.svelte';
    import SettingsTab from './SettingsTab.svelte';
    import TriggersTab from './TriggersTab.svelte';
    import VariablesTab from './VariablesTab.svelte';
    import {ApiError, del, get, post, put, upload} from './api.js';
    import type {Codes, Overview, Preview, Program, Report, Settings, Value, VariableDef} from './model.js';

    const notices = new NoticesStore();
    let tab = $state('programs');
    let version = $state('');
    let loadError = $state('');
    let overview = $state<Overview | null>(null);

    // the import
    let system = $state(false);
    let preview = $state<Preview | null>(null);
    let applied = $state<Report | null>(null);
    let codes = $state<Codes>({});
    let importBusy = $state(false);
    let importError = $state('');

    const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : String(e));

    async function refresh() {
        try {
            overview = await get<Overview>('api/v1/overview');
            loadError = '';
        } catch (e) {
            loadError = message(e);
        }
    }

    async function refreshImport() {
        try {
            const st = await get<{system: boolean; pending?: Preview}>('api/v1/import');
            system = st.system;
            if (!applied) preview = st.pending ?? null;
        } catch {
            /* the banner already says the addon is not reachable */
        }
    }

    onMount(() => {
        get<{version: string}>('api/v1/health').then(
            (h) => (version = h.version),
            (e: Error) => (loadError = e.message),
        );
        get<Codes>('api/v1/import/codes').then(
            (c) => (codes = c),
            () => {},
        );
        refresh();
        refreshImport();
        const timer = setInterval(() => {
            if (document.visibilityState === 'visible') refresh();
        }, 5000);
        return () => clearInterval(timer);
    });

    // an action: do it, say what went wrong, show the new state
    async function act(f: () => Promise<unknown>): Promise<boolean> {
        try {
            await f();
            await refresh();
            return true;
        } catch (e) {
            notices.push('error', message(e));
            await refresh();
            return false;
        }
    }

    const setActive = (key: string, active: boolean) => act(() => post(`api/v1/programs/${key}/active`, {active}));
    const setAtStart = (key: string, at_start: boolean) => act(() => post(`api/v1/programs/${key}/at-start`, {at_start}));
    const run = async (key: string, mode: string) => {
        if (await act(() => post(`api/v1/programs/${key}/run`, {mode}))) notices.push('info', mode === 'then' ? t('The first "Then" was run.') : t('The program was evaluated.'));
    };
    const setValue = (key: string, value: Value) => act(() => post(`api/v1/variables/${key}/value`, {value}));
    const acknowledge = (key: string) => act(() => post(`api/v1/variables/${key}/acknowledge`));
    const acknowledgeAll = () => act(() => post('api/v1/alarms/acknowledge'));
    const remove = (key: string) => act(() => del(`api/v1/variables/${key}`));
    const removeHelper = (key: string) => act(() => del(`api/v1/helpers/${key}`));

    async function save(key: string, body: {variable: VariableDef; renames: Record<string, string>; value?: Value}): Promise<string> {
        try {
            if (key) await put(`api/v1/variables/${key}`, {variable: body.variable, renames: body.renames});
            else await post('api/v1/variables', {variable: body.variable, value: body.value});
            await refresh();
            return '';
        } catch (e) {
            return message(e);
        }
    }

    // a program from the editor: created (key '') or replaced
    async function saveProgram(key: string, program: Program): Promise<{key?: string; error?: string}> {
        try {
            const out = key ? await put<{program: Program}>(`api/v1/programs/${key}`, {program}) : await post<{program: Program}>('api/v1/programs', {program});
            await refresh();
            return {key: out.program.key};
        } catch (e) {
            return {error: message(e)};
        }
    }
    const runPending = async (id: number) => {
        if (await act(() => post(`api/v1/pending/${id}/run`))) notices.push('info', t('The delayed activity was carried out.'));
    };
    const cancelPending = async (id: number) => {
        if (await act(() => del(`api/v1/pending/${id}`))) notices.push('info', t('The delayed activity was cancelled.'));
    };
    // the program the Programs tab shows (from another tab: its name was clicked there)
    let openProgram = $state('');
    // the search of the Triggers tab (kept while the page is open)
    let triggerQuery = $state('');
    // the History tab's program and search (kept while the page is open)
    let historyProgram = $state('');
    let historyQuery = $state('');
    const deleteProgram = (key: string) => act(() => del(`api/v1/programs/${key}`));

    // while the editor holds changes that are not saved, another tab is asked for first
    let editorDirty = $state(false);
    let wanted = $state('');
    function go(to: string) {
        if (editorDirty && to !== tab) {
            wanted = to;
            return;
        }
        // (the Programs tab shows its list again when it is come back to)
        if (to !== tab) openProgram = '';
        tab = to;
    }

    async function saveSettings(settings: Settings): Promise<string> {
        try {
            await put('api/v1/settings', settings);
            await refresh();
            return '';
        } catch (e) {
            return message(e);
        }
    }

    async function read(file: File | null, allInactive: boolean) {
        importBusy = true;
        importError = '';
        applied = null;
        const query = `lang=${i18n.language}${allInactive ? '&all_inactive=1' : ''}`;
        try {
            preview = file ? await upload<Preview>(`api/v1/import/preview?${query}`, file) : await post<Preview>(`api/v1/import/preview?from=system&${query}`);
        } catch (e) {
            importError = message(e);
        }
        importBusy = false;
    }

    async function apply() {
        if (!preview) return;
        importBusy = true;
        importError = '';
        try {
            const out = await post<{report: Report}>(`api/v1/import/${preview.id}/apply`);
            applied = out.report;
            preview = null;
            await refresh();
        } catch (e) {
            importError = message(e);
        }
        importBusy = false;
    }

    async function discard() {
        if (!preview) return;
        try {
            await del(`api/v1/import/${preview.id}`);
        } catch {
            /* gone already */
        }
        preview = null;
        importError = '';
    }

    const tabs = $derived([
        {id: 'programs', label: t('Programs')},
        {id: 'variables', label: t('System variables')},
        {id: 'schedule', label: t('Schedule')},
        {id: 'triggers', label: t('Triggers')},
        {id: 'history', label: t('History')},
        {id: 'import', label: t('Import')},
        {id: 'settings', label: t('Settings')},
    ]);
    const alarms = $derived(overview?.alarms.length ?? 0);
</script>

<div class="lgc-app" lang={i18n.language}>
    <header class="lgc-header">
        <h1>Logic</h1>
        <Tabs {tabs} bind:active={() => tab, go} label="Logic" testId="tabs" />
        <HelpLink topic="overview" />
        <button class="lgc-alarm-badge" class:lgc-alarm-none={alarms === 0} onclick={() => go('variables')} disabled={alarms === 0} title={t('Alarm messages')} data-testid="alarm-badge">
            {t('{n} alarms', {n: alarms}, alarms)}
        </button>
        <span class="lgc-spacer"></span>
        {#if version}<span class="lgc-muted lgc-small" data-testid="version">{t('Version {v}', {v: version})}</span>{/if}
        {#if !theme.followsShell}
            <ThemeSwitch theme={theme.choice} onclick={cycleTheme} />
        {/if}
    </header>
    {#if loadError}
        <div class="lgc-banner" role="alert">{t('The page could not reach the addon: {error}', {error: loadError})}</div>
    {/if}
    {#if overview?.notices.state}<div class="lgc-banner" role="alert">{overview.notices.state}</div>{/if}
    {#if overview?.notices.history}<div class="lgc-banner" role="alert">{overview.notices.history}</div>{/if}
    {#if overview?.notices.remote}<div class="lgc-banner" role="alert" data-testid="remote-notice">{t('The history could not be pushed to its remote. The Settings tab says why.')}</div>{/if}
    {#each overview?.notices.bad_files ?? [] as bad (bad.path)}
        <div class="lgc-banner" role="alert" data-testid="bad-file">{t('The file {path} is not used: {error}', {path: bad.path, error: bad.error})} <HelpLink topic="file-rules" /></div>
    {/each}
    <main class="lgc-page" data-testid="page-{tab}">
        {#if tab === 'import'}
            <ImportTab {system} {preview} {applied} {codes} busy={importBusy} error={importError} onread={read} onapply={apply} ondiscard={discard}
                ondone={() => { applied = null; tab = 'programs'; }} />
        {:else if !overview}
            <p class="lgc-muted">{t('Loading…')}</p>
        {:else if tab === 'programs'}
            <ProgramsTab {overview} bind:open={openProgram} onactive={setActive} onatstart={setAtStart} onrun={run} onimport={() => go('import')} onsettings={() => go('settings')}
                onsave={saveProgram} ondelete={deleteProgram} ondirty={(d) => (editorDirty = d)} onrefresh={refresh} />
        {:else if tab === 'schedule'}
            <ScheduleTab {overview} onopen={(key) => { openProgram = key; tab = 'programs'; }} onrun={runPending} oncancel={cancelPending} />
        {:else if tab === 'triggers'}
            <TriggersTab {overview} bind:query={triggerQuery} onopen={(key) => { openProgram = key; tab = 'programs'; }} />
        {:else if tab === 'history'}
            <HistoryTab {overview} bind:program={historyProgram} bind:query={historyQuery} onopen={(key) => { openProgram = key; tab = 'programs'; }} />
        {:else if tab === 'settings'}
            <SettingsTab {overview} onsave={saveSettings} onchanged={refresh} />
        {:else}
            <VariablesTab {overview} onrefresh={refresh} onvalue={setValue} onacknowledge={acknowledge} onacknowledgeall={acknowledgeAll} onsave={save} ondelete={remove} ondeletehelper={removeHelper} />
        {/if}
    </main>
    <ConfirmDialog open={wanted !== ''} title={t('Discard the changes?')} message={t('The program was changed and is not saved yet.')} confirmLabel={t('Discard')} danger testId="confirm-leave"
        onconfirm={() => { openProgram = ''; tab = wanted; wanted = ''; editorDirty = false; }} onclose={() => (wanted = '')} />
    <Notices notices={notices.visible} ondismiss={(id) => notices.dismiss(id)} />
</div>
