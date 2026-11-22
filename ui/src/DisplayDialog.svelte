<script lang="ts">
    // The display composer: the five lines of a display channel (HmIP-WRCD) with a text, an
    // icon, where the text stands and whether the line is inverted, an optional signal tone -
    // and a preview that follows what is typed. Nothing changes before "Apply".
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {DISPLAY_ICONS, DISPLAY_INTERVALS, DISPLAY_SOUNDS, displayProblem, ICON_GLYPHS, previewText, type DisplayForm} from './display.js';
    import type {Env} from './editor.js';

    let {channel, name, display, env, onsave, onclose}: {
        channel: string;
        /** The channel as the row shows it. */
        name: string;
        display: DisplayForm;
        env: Env;
        onsave: (display: DisplayForm) => void;
        onclose: () => void;
    } = $props();

    // svelte-ignore state_referenced_locally
    let form = $state<DisplayForm>({...display, lines: display.lines.map((l) => ({...l}))});
    let tried = $state(false);
    const problem = $derived(displayProblem(channel, form, env));
    const PLACEHOLDERS = ['{value}', '{var:…}', '{program}', '{trigger}', '{time}'];

    function invert(i: number, on: boolean) {
        Object.assign(form.lines[i]!, on ? {color: 'WHITE', background: 'BLACK'} : {color: 'BLACK', background: 'WHITE'});
    }

    function save() {
        tried = true;
        if (problem) return;
        onsave(form);
    }
</script>

<Dialog open title={t('Display {channel}', {channel: name})} width="46rem" {onclose} testId="display-dialog">
    <form class="lgc-dsp" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <p class="lgc-small lgc-muted lgc-vals-intro">
            {t('Up to five lines, each with a text, an icon or both. A line with neither is left as the display has it.')}
            <HelpLink topic="display" />
        </p>
        <div class="lgc-dsp-body">
            <div class="lgc-dsp-lines">
                {#each form.lines as line, i (i)}
                    <div class="lgc-dsp-row" data-testid="display-line-{i + 1}">
                        <span class="lgc-ed-mark">{i + 1}</span>
                        <input class="hmm-input lgc-dsp-text" type="text" spellcheck="false" autocomplete="off" placeholder={t('Text')} aria-label={t('Text of line {n}', {n: i + 1})} bind:value={line.text} data-testid="display-text" />
                        <select class="hmm-select lgc-dsp-icon" aria-label={t('Icon of line {n}', {n: i + 1})} bind:value={line.icon} data-testid="display-icon">
                            <option value="">{t('no icon')}</option>
                            {#if line.icon && !DISPLAY_ICONS.includes(line.icon)}<option value={line.icon}>{line.icon}</option>{/if}
                            {#each DISPLAY_ICONS.slice(1) as icon (icon)}<option value={icon} title={icon}>{t('icon.' + icon)}</option>{/each}
                        </select>
                        <select class="hmm-select lgc-dsp-align" aria-label={t('Alignment of line {n}', {n: i + 1})} bind:value={line.align} data-testid="display-align">
                            <option value="LEFT">{t('left')}</option>
                            <option value="CENTER">{t('centred')}</option>
                            <option value="RIGHT">{t('right')}</option>
                        </select>
                        <label class="lgc-check lgc-dsp-invert" title={t('White on black instead of black on white')}><input type="checkbox" checked={line.background === 'BLACK'} onchange={(e) => invert(i, e.currentTarget.checked)} data-testid="display-invert" /> <span>{t('inverted')}</span></label>
                    </div>
                {/each}
            </div>
            <div class="lgc-dsp-side">
                <div class="lgc-dsp-preview" aria-label={t('Preview')} data-testid="display-preview">
                    {#each form.lines as line, i (i)}
                        <div class="lgc-dsp-shown" style:background={line.background === 'BLACK' ? '#111' : '#f4f4ee'} style:color={line.color === 'WHITE' ? '#f4f4ee' : '#111'}
                            style:justify-content={line.align === 'LEFT' ? 'flex-start' : line.align === 'RIGHT' ? 'flex-end' : 'center'} data-testid="display-shown-{i + 1}">
                            {#if line.icon}<span class="lgc-dsp-glyph" title={line.icon}>{ICON_GLYPHS[line.icon] ?? '▫'}</span>{/if}<span class="lgc-dsp-chars">{previewText(line.text)}</span>
                        </div>
                    {/each}
                </div>
                <div class="lgc-small lgc-muted">{t('Preview: close to the display, not its exact picture.')}</div>
            </div>
        </div>
        <div class="lgc-dsp-sound">
            <label class="lgc-dsp-label" for="lgc-dsp-sound">{t('Signal tone')}</label>
            <select id="lgc-dsp-sound" class="hmm-select" bind:value={form.sound} data-testid="display-sound">
                <option value="">{t('none')}</option>
                {#each DISPLAY_SOUNDS as sound (sound)}<option value={sound} title={sound}>{t('entry.' + sound)}</option>{/each}
            </select>
            <select class="hmm-select" class:lgc-vals-off={!form.sound} aria-label={t('Repetitions')} bind:value={form.repetitions} disabled={!form.sound} data-testid="display-repetitions">
                <option value={0}>{t('entry.NO_REPETITION')}</option>
                {#each Array.from({length: 14}, (_, i) => i + 1) as n (n)}<option value={n}>{t('{n} repetitions', {n}, n)}</option>{/each}
                <option value={15}>{t('entry.INFINITE_REPETITIONS')}</option>
            </select>
            <select class="hmm-select" class:lgc-vals-off={!form.sound} aria-label={t('Pause between two')} bind:value={form.interval} disabled={!form.sound} data-testid="display-interval">
                {#each DISPLAY_INTERVALS as n (n)}<option value={n}>{t('every {n} s', {n})}</option>{/each}
            </select>
        </div>
        <p class="lgc-small lgc-muted" data-testid="display-placeholders">
            {t('In a text:')} {#each PLACEHOLDERS as p (p)}<code>{p}</code>{' '}{/each}
            <HelpLink topic="display-text" />
        </p>
        <div class="lgc-error-inline lgc-reserve" role="alert" data-testid="display-error">{tried ? problem : ''}</div>
    </form>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
        <button class="hmm-button lgc-primary" onclick={save} data-testid="display-save">{t('Apply')}</button>
    {/snippet}
</Dialog>
