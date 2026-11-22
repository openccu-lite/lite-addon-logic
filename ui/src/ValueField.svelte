<script lang="ts">
    // One value of a condition line or an activity, entered as its kind asks: a choice for a
    // logic value, a list or an ENUM, a field for a number or a text. The field keeps what was
    // typed ("21," on the way to "21,5"); the value beside it is what that means.
    import {parseTyped, typedText, type ValueKind} from './editor.js';
    import type {Value} from './model.js';

    let {value = $bindable(), kind, label, testId, invalid = false}: {value: Value; kind: ValueKind; label: string; testId: string; invalid?: boolean} = $props();

    // svelte-ignore state_referenced_locally
    let text = $state(typedText(value));
    $effect(() => {
        // the value was set from outside (another datapoint chosen): the field follows
        if (parseTyped(kind.kind, text) !== value) text = typedText(value);
    });
    const index = $derived((kind.options ?? []).findIndex((o) => o.value === value));
</script>

{#if kind.kind === 'bool' || kind.kind === 'choice'}
    <select class="hmm-select lgc-ed-value" class:lgc-invalid={invalid} aria-label={label} value={String(index)} onchange={(e) => (value = kind.options?.[Number(e.currentTarget.value)]?.value)} data-testid={testId}>
        {#if index < 0}<option value="-1">{typedText(value) || '–'}</option>{/if}
        {#each kind.options ?? [] as option, i (i)}<option value={String(i)}>{option.label}</option>{/each}
    </select>
{:else}
    <span class="lgc-ed-value lgc-ed-typed">
        <input class="hmm-input" class:lgc-invalid={invalid} type="text" inputmode={kind.kind === 'number' ? 'decimal' : 'text'} aria-label={label} value={text}
            oninput={(e) => { text = e.currentTarget.value; value = parseTyped(kind.kind, text); }} data-testid={testId} />
        <span class="lgc-ed-unit lgc-small lgc-muted">{kind.unit ?? ''}</span>
    </span>
{/if}
