<script lang="ts">
    // The addon's settings: the location, for sunrise and sunset of the astro time modules; the
    // pause between write commands and the loop guard. Each group saves the whole settings with
    // its own part changed.
    import {i18n, t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import RemoteSection from './RemoteSection.svelte';
    import ReplaceSection from './ReplaceSection.svelte';
    import SecretsSection from './SecretsSection.svelte';
    import {RUNTIME, runtimeFormOf, runtimeOf, wallTime, type Overview, type RuntimeForm, type Settings} from './model.js';

    let {overview, onsave, onchanged}: {overview: Overview; onsave: (s: Settings) => Promise<string>; onchanged: () => void} = $props();

    const programNames = $derived(Object.fromEntries(overview.programs.map((p) => [p.program.key, p.program.name])));

    // the fields follow the stored location until somebody types
    let latitude = $state('');
    let longitude = $state('');
    let touched = $state(false);
    let error = $state('');
    let saved = $state('');
    const stored = $derived(overview.settings.location);
    $effect(() => {
        if (!touched) {
            latitude = stored ? String(stored.latitude) : '';
            longitude = stored ? String(stored.longitude) : '';
        }
    });
    const number = (s: string) => (s.trim() === '' ? NaN : Number(s.trim().replace(',', '.')));

    async function save(e: Event) {
        e.preventDefault();
        saved = '';
        const lat = number(latitude);
        const lon = number(longitude);
        if (Number.isNaN(lat) || Number.isNaN(lon)) {
            error = t('Latitude and longitude are numbers, like 52.52 and 13.405.');
            return;
        }
        error = await onsave({...overview.settings, location: {latitude: lat, longitude: lon}});
        if (!error) {
            touched = false;
            saved = t('The location was saved.');
        }
    }

    async function clear() {
        saved = '';
        error = await onsave({...overview.settings, location: undefined});
        if (!error) {
            touched = false;
            saved = t('The location was removed.');
        }
    }

    // the pause between write commands and the loop guard: the fields follow what is stored
    // until somebody types
    let runtime = $state<RuntimeForm>({pacing: '', runs: '', window: '', pause: ''});
    let runtimeTouched = $state(false);
    let runtimeError = $state('');
    let runtimeSaved = $state('');
    const standard = runtimeFormOf({});
    $effect(() => {
        if (!runtimeTouched) runtime = runtimeFormOf(overview.settings);
    });
    const isStandard = $derived(JSON.stringify(runtimeFormOf(overview.settings)) === JSON.stringify(standard) && !runtimeTouched);

    async function storeRuntime(form: RuntimeForm) {
        runtimeSaved = '';
        const parsed = runtimeOf(form);
        if (parsed.error) {
            runtimeError = parsed.error;
            return;
        }
        runtimeError = await onsave({location: overview.settings.location, pacing_ms: parsed.pacing_ms, loop_guard: parsed.loop_guard});
        if (!runtimeError) {
            runtimeTouched = false;
            runtimeSaved = t('The settings were saved.');
        }
    }
</script>

<section class="lgc-section" data-testid="settings">
    <div class="lgc-section-head lgc-head-tight">
        <h2>{t('Location')}</h2>
        <HelpLink topic="time-location" />
    </div>
    <p class="lgc-muted">{t('Where the system stands: sunrise and sunset of the astro time modules are computed for it.')}</p>
    <form class="lgc-settings-form" onsubmit={save}>
        <label class="lgc-field">
            <span>{t('Latitude')}</span>
            <input class="hmm-input" type="text" inputmode="decimal" bind:value={latitude} oninput={() => (touched = true)} placeholder="52.52" data-testid="latitude" />
            <span class="lgc-muted lgc-small">{t('degrees, north positive')}</span>
        </label>
        <label class="lgc-field">
            <span>{t('Longitude')}</span>
            <input class="hmm-input" type="text" inputmode="decimal" bind:value={longitude} oninput={() => (touched = true)} placeholder="13.405" data-testid="longitude" />
            <span class="lgc-muted lgc-small">{t('degrees, east positive')}</span>
        </label>
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" type="submit" data-testid="save-location">{t('Save')}</button>
            <button class="hmm-button" type="button" onclick={clear} disabled={!stored} data-testid="clear-location">{t('Remove the location')}</button>
        </div>
        <!-- one line that is always there: the page does not jump when a message comes -->
        <p class="lgc-settings-message" role="status" data-testid="settings-message">
            {#if error}<span class="lgc-bad">{error}</span>{:else if saved}{saved}{:else if !stored}{t('No location is set.')}{:else}&nbsp;{/if}
        </p>
    </form>
    <div class="lgc-kv lgc-kv-compact">
        <span>{t('System time')} <HelpLink topic="time-clock" /></span>
        <span data-testid="system-time">{wallTime(overview.now, i18n.language)}</span>
    </div>
</section>

<section class="lgc-section" data-testid="settings-runtime">
    <form class="lgc-settings-form" onsubmit={(e) => { e.preventDefault(); storeRuntime(runtime); }}>
        <div class="lgc-section-head lgc-head-tight lgc-settings-group">
            <h2>{t('Write commands')}</h2>
            <HelpLink topic="pacing" />
        </div>
        <p class="lgc-muted lgc-settings-group">{t('Programs send their write commands to radio devices one after the other, with this pause between two: a program that switches much must not flood the air.')}</p>
        <label class="lgc-field">
            <span>{t('Pause between write commands')}</span>
            <input class="hmm-input lgc-num-input" type="text" inputmode="numeric" bind:value={runtime.pacing} oninput={() => (runtimeTouched = true)} placeholder={standard.pacing} data-testid="pacing" />
            <span class="lgc-muted lgc-small">{t('milliseconds')} ({RUNTIME.pacing.min}–{RUNTIME.pacing.max})</span>
        </label>
        <div class="lgc-section-head lgc-head-tight lgc-settings-group">
            <h2>{t('Loop guard')}</h2>
            <HelpLink topic="loop-guard" />
        </div>
        <p class="lgc-muted lgc-settings-group">{t('A program that runs more often than this is paused and marked: usually it writes what it triggers on.')}</p>
        <label class="lgc-field">
            <span>{t('Runs')}</span>
            <input class="hmm-input lgc-num-input" type="text" inputmode="numeric" bind:value={runtime.runs} oninput={() => (runtimeTouched = true)} placeholder={standard.runs} data-testid="loop-runs" />
            <span class="lgc-muted lgc-small">{t('times')} ({RUNTIME.runs.min}–{RUNTIME.runs.max})</span>
        </label>
        <label class="lgc-field">
            <span>{t('Within')}</span>
            <input class="hmm-input lgc-num-input" type="text" inputmode="numeric" bind:value={runtime.window} oninput={() => (runtimeTouched = true)} placeholder={standard.window} data-testid="loop-window" />
            <span class="lgc-muted lgc-small">{t('seconds')} ({RUNTIME.window.min}–{RUNTIME.window.max})</span>
        </label>
        <label class="lgc-field">
            <span>{t('Pause')}</span>
            <input class="hmm-input lgc-num-input" type="text" inputmode="numeric" bind:value={runtime.pause} oninput={() => (runtimeTouched = true)} placeholder={standard.pause} data-testid="loop-pause" />
            <span class="lgc-muted lgc-small">{t('seconds')} ({RUNTIME.pause.min}–{RUNTIME.pause.max})</span>
        </label>
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" type="submit" data-testid="save-runtime">{t('Save')}</button>
            <button class="hmm-button" type="button" onclick={() => storeRuntime(standard)} disabled={isStandard} data-testid="reset-runtime">{t('Back to the defaults')}</button>
        </div>
        <p class="lgc-settings-message" role="status" data-testid="runtime-message">
            {#if runtimeError}<span class="lgc-bad">{runtimeError}</span>{:else if runtimeSaved}{runtimeSaved}{:else}&nbsp;{/if}
        </p>
    </form>
</section>

<SecretsSection programs={programNames} />
<ReplaceSection tick={overview.now} {onchanged} />
<RemoteSection tick={overview.now} {onchanged} />
