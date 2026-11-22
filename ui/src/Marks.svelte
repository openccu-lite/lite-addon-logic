<script lang="ts">
    // The marks of a row in a column of their own: the first as a pill of one width, the number
    // of the others beside it; all of them in words as the tooltip. The cell keeps its size
    // whatever is marked.
    import {marksTitle, type Mark} from './model.js';

    let {marks}: {marks: Mark[]} = $props();
</script>

<span class="lgc-marks" title={marksTitle(marks)} data-testid="marks">
    {#if marks[0]}<span class={'lgc-pill ' + marks[0].kind} data-testid="flag-{marks[0].id}">{marks[0].label}</span>{/if}
    {#if marks.length > 1}<span class="lgc-pill lgc-pill-more" data-testid="marks-more">+{marks.length - 1}</span>{/if}
</span>
