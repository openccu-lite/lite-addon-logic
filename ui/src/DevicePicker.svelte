<script lang="ts">
    // The device picker: every channel of the system by name, with search and filters for the
    // interface, the room and the device type - instead of the CCU's popup table. A click on a
    // row chooses the channel.
    import {Dialog} from 'occulite-ui';
    import {t} from 'occulite-ui/i18n';

    import HelpLink from './HelpLink.svelte';
    import {filterRows, pickerOptions, pickerRows, type Device} from './editor.js';

    let {devices, loading, error, onpick, onclose}: {devices: Device[]; loading: boolean; error: string; onpick: (channel: string) => void; onclose: () => void} = $props();

    const LIMIT = 200;
    let filter = $state({query: '', interface: '', room: '', type: ''});
    const rows = $derived(pickerRows(devices));
    const options = $derived(pickerOptions(rows));
    const found = $derived(filterRows(rows, filter));
</script>

<Dialog open title={t('Choose a device')} width="56rem" height="38rem" {onclose} testId="device-picker">
    <div class="lgc-picker">
        <div class="lgc-picker-filters">
            <input class="hmm-input lgc-picker-search" type="search" placeholder={t('Search name, room, type, address…')} aria-label={t('Search name, room, type, address…')} bind:value={filter.query} data-testid="picker-search" />
            <select class="hmm-select" aria-label={t('Room')} bind:value={filter.room} data-testid="picker-room">
                <option value="">{t('all rooms')}</option>
                {#each options.rooms as room (room)}<option value={room}>{room}</option>{/each}
            </select>
            <select class="hmm-select" aria-label={t('Device type')} bind:value={filter.type} data-testid="picker-type">
                <option value="">{t('all device types')}</option>
                {#each options.types as type (type)}<option value={type}>{type}</option>{/each}
            </select>
            <select class="hmm-select" aria-label={t('Interface')} bind:value={filter.interface} data-testid="picker-interface">
                <option value="">{t('all interfaces')}</option>
                {#each options.interfaces as iface (iface)}<option value={iface}>{iface}</option>{/each}
            </select>
            <HelpLink topic="editor-devices" />
        </div>
        <div class="lgc-picker-list">
            <table class="lgc-grid lgc-fixed lgc-cardgrid" data-testid="picker-list">
                <thead>
                    <tr>
                        <th class="lgc-w-name">{t('Channel')}</th>
                        <th>{t('Device')}</th>
                        <th class="lgc-c-note">{t('Channel type')}</th>
                        <th class="lgc-w-type">{t('Room')}</th>
                        <th class="lgc-w-stamp">{t('Address')}</th>
                    </tr>
                </thead>
                <tbody>
                    {#each found.slice(0, LIMIT) as row (row.channel)}
                        <tr data-testid="pick-{row.channel}">
                            <td data-label={t('Channel')}><button class="lgc-linklike lgc-cut" title={row.name || row.address} onclick={() => onpick(row.channel)} data-testid="pick">{row.name || row.address}</button></td>
                            <td data-label={t('Device')}><span class="lgc-cut" title={row.device + ' · ' + row.deviceType}>{row.device} <span class="lgc-muted lgc-small">{row.deviceType}</span></span></td>
                            <td class="lgc-c-note" data-label={t('Channel type')}><span class="lgc-cut lgc-small" title={row.channelType}>{row.channelType}</span></td>
                            <td data-label={t('Room')}><span class="lgc-cut" class:lgc-muted={row.rooms.length === 0} title={row.rooms.join(', ')}>{row.rooms.join(', ') || '–'}</span></td>
                            <td data-label={t('Address')}><span class="lgc-cut lgc-mono lgc-small" title={row.interface + '/' + row.address}>{row.address}</span></td>
                        </tr>
                    {/each}
                </tbody>
            </table>
        </div>
        <div class="lgc-small lgc-muted lgc-reserve" data-testid="picker-count">
            {#if loading}{t('Loading…')}
            {:else if error}<span class="lgc-error-inline">{t('The system did not list its devices: {error}', {error})}</span>
            {:else if found.length === 0}{t('No channel matches.')}
            {:else if found.length > LIMIT}{t('{n} channels, the first {limit} shown: narrow the search.', {n: found.length, limit: LIMIT})}
            {:else}{t('{n} channels', {n: found.length}, found.length)}{/if}
        </div>
    </div>
    {#snippet buttons()}
        <button class="hmm-button" onclick={onclose}>{t('Cancel')}</button>
    {/snippet}
</Dialog>
