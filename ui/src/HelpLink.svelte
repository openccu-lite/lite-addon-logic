<script lang="ts">
    // The ?-icon beside a feature or a group of fields: opens the guide's section about it in
    // the page's language, in a new tab. The anchors are the same in both languages
    // (docs/de/handbuch.md, docs/en/guide.md); a test holds every anchor used here to the guide.
    import {i18n, t} from 'occulite-ui/i18n';

    import {docUrl} from './help.js';

    let {topic}: {topic: string} = $props();
</script>

<a class="lgc-helplink" href={docUrl(i18n.language, topic)} target="_blank" rel="noopener" title={t('Open the guide')} aria-label={t('Open the guide')} data-testid="help-{topic}">?</a>
