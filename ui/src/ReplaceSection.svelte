<script lang="ts">
    // "Gerät ersetzen": after a device was swapped for another of its kind, everything that
    // names the old one - condition lines, activities, computed values - names the new one, in
    // one step. First a look at what would change and whether the new device has what is named.
    import {i18n, t} from 'occulite-ui/i18n';

    import ConfirmDialog from './ConfirmDialog.svelte';
    import HelpLink from './HelpLink.svelte';
    import {get, post} from './api.js';
    import {candidates, deviceLabel, idOf, replacedText, replacementText, usedLabel, type Replacement, type SystemDevice, type UsedDevice} from './replace.js';

    // (tick changes whenever the page has asked the addon anew: the lists follow)
    let {tick, onchanged}: {tick: string; onchanged: () => void} = $props();

    let used = $state<UsedDevice[]>([]);
    let system = $state(true);
    let devices = $state<SystemDevice[]>([]);
    let from = $state('');
    let to = $state('');
    let preview = $state<Replacement | null>(null);
    let error = $state('');
    let done = $state('');
    let busy = $state(false);
    let ask = $state(false);

    const old = $derived(used.find((d) => idOf(d) === from));
    const targets = $derived(candidates(old, devices));
    const target = $derived(targets.find((d) => idOf(d) === to));

    async function load() {
        try {
            const [u, d] = await Promise.all([get<{devices: UsedDevice[]; system: boolean}>(`api/v1/devices/used?lang=${i18n.language}`), get<{devices: SystemDevice[]}>(`api/v1/devices?lang=${i18n.language}`).catch(() => ({devices: []}))]);
            used = u.devices;
            system = u.system;
            devices = d.devices;
            if (!used.some((x) => idOf(x) === from)) {
                from = '';
                to = '';
                preview = null;
            }
        } catch {
            // the page's own banner says when the addon does not answer
        }
    }

    $effect(() => {
        void tick;
        load();
    });

    const body = () => ({from: {interface: old!.interface, address: old!.address}, to: {interface: target!.interface, address: target!.address}});

    async function look() {
        preview = null;
        error = '';
        done = '';
        if (!old || !target) return;
        busy = true;
        try {
            preview = await post<Replacement>('api/v1/devices/replace', {...body(), dry_run: true});
        } catch (e) {
            error = (e as Error).message;
        } finally {
            busy = false;
        }
    }

    async function replace() {
        ask = false;
        if (!old || !target) return;
        busy = true;
        error = '';
        try {
            const r = await post<Replacement>('api/v1/devices/replace', body());
            done = replacedText(r);
            preview = null;
            from = '';
            to = '';
            onchanged();
            await load();
        } catch (e) {
            error = (e as Error).message;
        } finally {
            busy = false;
        }
    }
</script>

<section class="lgc-section" data-testid="settings-replace">
    <div class="lgc-section-head lgc-head-tight">
        <h2>{t('Replace a device')}</h2>
        <HelpLink topic="replace-device" />
    </div>
    <p class="lgc-muted">{t('After a device was swapped for another one of its type: every program and variable that names the old device names the new one, in one step.')}</p>
    <div class="lgc-settings-form lgc-replace-form">
        <label class="lgc-field">
            <span>{t('The device programs name')}</span>
            <select class="hmm-select lgc-replace-wide" bind:value={from} onchange={() => { to = ''; preview = null; error = ''; done = ''; }} disabled={used.length === 0} data-testid="replace-from">
                <option value="">{used.length === 0 ? t('No program or variable names a device.') : t('Choose…')}</option>
                {#each used as d (idOf(d))}<option value={idOf(d)}>{usedLabel(d, system)}</option>{/each}
            </select>
        </label>
        <label class="lgc-field">
            <span>{t('The device in its place')}</span>
            <select class="hmm-select lgc-replace-wide" bind:value={to} onchange={look} disabled={!old || targets.length === 0} data-testid="replace-to">
                <option value="">{!old ? t('Choose…') : targets.length === 0 ? (old.type ? t('The system has no other device of the type {type}.', {type: old.type}) : t('The system has no other device.')) : t('Choose…')}</option>
                {#each targets as d (idOf(d))}<option value={idOf(d)}>{deviceLabel(d)}</option>{/each}
            </select>
        </label>
        <!-- what would change, or what stands against it: the place is there whether or not something is chosen -->
        <div class="lgc-replace-preview" role="status" data-testid="replace-preview">
            {#if error}<span class="lgc-bad">{error}</span>
            {:else if done}{done}
            {:else if preview}
                {#if preview.can_apply}<span data-testid="replace-would">{replacementText(preview)}</span>{/if}
                {#each preview.problems as p (p)}<div class="lgc-bad" data-testid="replace-problem">{p}</div>{/each}
            {:else if !system}<span class="lgc-bad">{t('The system does not list its devices right now: nothing can be replaced.')}</span>
            {:else}&nbsp;{/if}
        </div>
        <div class="lgc-actions">
            <button class="hmm-button lgc-primary" type="button" disabled={busy || !preview?.can_apply} onclick={() => (ask = true)} data-testid="replace-apply">{t('Replace')}</button>
        </div>
    </div>
</section>
<ConfirmDialog
    open={ask}
    title={t('Replace the device?')}
    message={old && target ? t('Everything that names {old} will name {new}. Nothing runs because of it; every changed program and variable gets a new version.', {old: deviceLabel(old), new: deviceLabel(target)}) : ''}
    confirmLabel={t('Replace')}
    testId="confirm-replace"
    onconfirm={replace}
    onclose={() => (ask = false)}
/>
