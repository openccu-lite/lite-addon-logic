<script lang="ts">
    // What a condition line or an activity can do beyond the CCU, in a form of its own so the
    // editor's rows keep their shape: a line that must have held for a while ("hält seit"), an
    // activity that switches on for a time ("ein für") or is delayed at random. The row says
    // in words what is set here. Nothing changes before "Apply".
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {canOnFor, delayProblem, holdProblem, onForProblem, type DraftAction, type DraftLine, type Env} from './editor.js';

    let {of, env, onclose}: {of: DraftLine | DraftAction; env: Env; onclose: () => void} = $props();

    // svelte-ignore state_referenced_locally
    let form = $state({...of});
    let tried = $state(false);

    const line = $derived('op' in form ? (form as DraftLine) : null);
    const action = $derived('op' in form ? null : (form as DraftAction));
    const problem = $derived(line ? holdProblem(line) : onForProblem(action!, env) || delayProblem(action!));
    const UNITS = ['s', 'min', 'h'];

    function save() {
        tried = true;
        if (problem) return;
        if (line) {
            const {held, holdN, holdUnit} = line;
            Object.assign(of, {held, holdN, holdUnit});
        } else {
            const {timed, onForN, onForUnit, delayed, random, delayN, delayUnit, upToN, upToUnit} = action!;
            Object.assign(of, {timed, onForN, onForUnit, delayed, random, delayN, delayUnit, upToN, upToUnit});
        }
        onclose();
    }

    function setRandom(a: DraftAction, on: boolean) {
        a.random = on;
        if (on) a.delayed = true;
        else if (a.delayN.trim() === '') a.delayed = false;
    }
</script>

{#snippet unit(value: string, set: (unit: string) => void, disabled: boolean, testId: string)}
    <select class="hmm-select lgc-opt-unit" aria-label={t('Unit')} {value} {disabled} onchange={(e) => set(e.currentTarget.value)} data-testid={testId}>
        {#each UNITS as u (u)}<option value={u}>{t('unit.' + u)}</option>{/each}
    </select>
{/snippet}

<Dialog open title={line ? t('More options of the line') : t('More options of the activity')} width="40rem" {onclose} testId="options-dialog">
    <form class="lgc-opt" onsubmit={(e) => { e.preventDefault(); save(); }}>
        {#if line}
            <div class="lgc-opt-row">
                <label class="lgc-check"><input type="checkbox" bind:checked={line.held} data-testid="opt-held" /> {t('only true once the comparison has held for at least')}</label>
                <span class="lgc-opt-pair"><input class="hmm-input lgc-narrow" type="text" inputmode="numeric" aria-label={t('How long')} bind:value={line.holdN} disabled={!line.held} data-testid="opt-hold" />{@render unit(line.holdUnit, (u) => (line.holdUnit = u), !line.held, 'opt-hold-unit')}</span>
                <HelpLink topic="additions" />
            </div>
            <p class="lgc-small lgc-muted">{t('The moment that time is reached triggers the program, unless the line is "check only". When the comparison ends, the line is false at once.')}</p>
        {:else if action}
            <div class="lgc-opt-row">
                <label class="lgc-check"><input type="checkbox" bind:checked={action.timed} disabled={!canOnFor(action)} data-testid="opt-timed" /> {t('on for')}</label>
                <span class="lgc-opt-pair"><input class="hmm-input lgc-narrow" type="text" inputmode="numeric" aria-label={t('How long')} bind:value={action.onForN} disabled={!action.timed || !canOnFor(action)} data-testid="opt-on-for" />{@render unit(action.onForUnit, (u) => (action.onForUnit = u), !action.timed || !canOnFor(action), 'opt-on-for-unit')}</span>
                <HelpLink topic="additions" />
            </div>
            <p class="lgc-small lgc-muted">{canOnFor(action) ? t('The channel\'s on-time is sent together with the value, in one command: the device switches back by itself.') : t('"On for" belongs to a fixed value written to a device.')}</p>
            <div class="lgc-opt-row">
                <label class="lgc-check"><input type="checkbox" checked={action.delayed && action.random} onchange={(e) => setRandom(action, e.currentTarget.checked)} data-testid="opt-random" /> {t('delayed at random, from')}</label>
                <span class="lgc-opt-pair"><input class="hmm-input lgc-narrow" type="text" inputmode="numeric" placeholder="0" aria-label={t('Shortest delay')} bind:value={action.delayN} disabled={!(action.delayed && action.random)} data-testid="opt-from" />{@render unit(action.delayUnit, (u) => (action.delayUnit = u), !(action.delayed && action.random), 'opt-from-unit')}</span>
                <span>{t('to')}</span>
                <span class="lgc-opt-pair"><input class="hmm-input lgc-narrow" type="text" inputmode="numeric" aria-label={t('Longest delay')} bind:value={action.upToN} disabled={!(action.delayed && action.random)} data-testid="opt-up-to" />{@render unit(action.upToUnit, (u) => (action.upToUnit = u), !(action.delayed && action.random), 'opt-up-to-unit')}</span>
            </div>
            <p class="lgc-small lgc-muted">{t('The delay is drawn anew, to the second, every time the activity is due.')}</p>
        {/if}
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="opt-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="opt-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
