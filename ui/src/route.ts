// The page's tab in the address (occulited B-184): openccu-lite links to the import as
// `/addons/logic/#/import` (the manifest's ui.regadom_import_url), so the hash names the tab the
// page opens on, and a tab change keeps it in step while the address carries one.

/** The tab a hash such as `#/import` or `#import` names, or null for none the page has. */
export function tabFromHash(hash: string, tabs: readonly string[]): string | null {
    const id = hash.replace(/^#\/?/, '').split(/[/?]/)[0] ?? '';
    return tabs.includes(id) ? id : null;
}

/** The hash for a tab. */
export function hashFor(tab: string): string {
    return `#/${tab}`;
}
