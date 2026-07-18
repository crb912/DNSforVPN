<script>
  import { onMount } from 'svelte';
  import * as api from '../api.js';
  import { CATALOG } from '../doh-catalog.js';

  // Only active (configured) servers are shown; candidate selection lives in
  // the Config tab. Display names come from the catalog when the URL matches.
  const GROUPS = [
    { id: 'direct', label: 'Direct', hint: 'probed without proxy' },
    { id: 'proxy',  label: 'Proxy',  hint: 'probed through configured proxy' },
  ];

  let servers = [];
  let loading = false;
  let lastCheck = '';

  const keyOf = (s) => s.group + '|' + s.url;
  const hostOf = (url) => { try { return new URL(url).hostname; } catch { return url; } };
  const nameOf = (url) => CATALOG.find((c) => c.url === url)?.name ?? hostOf(url);

  onMount(async () => {
    let cfg = null;
    try { cfg = await api.GetConfig(); } catch { /* config unreachable */ }
    const direct = cfg?.doh_servers?.direct_servers ?? [];
    const proxy = cfg?.doh_servers?.proxy_servers ?? [];
    servers = [
      ...direct.map((url) => ({ group: 'direct', url })),
      ...proxy.map((url) => ({ group: 'proxy', url })),
    ].map((s) => ({ ...s, name: nameOf(s.url), ms: 0, status: '', checked: false }));
    await refresh();
  });

  async function refresh() {
    if (servers.length === 0) return;
    loading = true;
    try {
      const res = (await api.CheckLatency()) ?? [];
      const byUrl = {};
      for (const r of res) byUrl[r.server_url] = r;
      servers = servers.map((s) => {
        const r = byUrl[s.url];
        return r
          ? { ...s, ms: r.latency_ms, status: r.status, checked: true }
          : { ...s, ms: 0, status: 'error', checked: true };
      });
      lastCheck = new Date().toLocaleTimeString();
    } catch { /* keep previous data */ }
    loading = false;
  }

  function barWidth(ms) {
    return Math.min(100, Math.max(2, ms / 3)) + '%';
  }

  // Thresholds calibrated for keep-alive warm connections: direct servers
  // answer in ~50-100 ms, proxied ones in ~200-300 ms.
  function barColor(ms, status) {
    if (status !== 'ok') return '#ef5350';
    if (ms < 100) return '#66bb6a';
    if (ms < 300) return '#ffa726';
    return '#ef5350';
  }
</script>

<div class="latency">
  <header>
    <h3>DNS Server Latency</h3>
    <div class="actions">
      {#if lastCheck}<span class="last">last checked {lastCheck}</span>{/if}
      <button class="refresh" on:click={refresh} disabled={loading}>
        {loading ? '...' : 'Refresh'}
      </button>
    </div>
  </header>

  {#if servers.length === 0}
    <p class="empty">No active DoH servers — enable some in the Config tab.</p>
  {/if}

  {#each GROUPS as g}
    {@const rows = servers.filter((s) => s.group === g.id)}
    {#if rows.length > 0}
      <div class="group-head">
        <span class="icon">
          {#if g.id === 'direct'}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
                 stroke-linecap="round" stroke-linejoin="round">
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon>
            </svg>
          {:else}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
                 stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path>
            </svg>
          {/if}
        </span>
        <span class="glabel">{g.label}</span>
        <span class="ghint">{g.hint}</span>
      </div>

      <div class="table">
        {#each rows as srv (keyOf(srv))}
          <div class="row">
            <span class="name" title={srv.url}>{srv.name}</span>
            <span class="url" title={srv.url}>{srv.url}</span>
            <span class="bar-bg">
              {#if srv.checked}
                <span
                  class="bar"
                  style="width: {barWidth(srv.ms)}; background: {barColor(srv.ms, srv.status)}"
                ></span>
              {/if}
            </span>
            <span class="ms">{srv.checked ? srv.ms.toFixed(0) + ' ms' : '—'}</span>
            <span class="status" class:ok={srv.status === 'ok'} class:err={srv.checked && srv.status !== 'ok'}>
              {srv.checked ? srv.status : ''}
            </span>
          </div>
        {/each}
      </div>
    {/if}
  {/each}
</div>

<style>
  .latency {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  h3 { color: #4fc3f7; font-size: 0.95em; }

  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .last { color: #777; font-size: 0.72em; }

  .empty { color: #777; font-size: 0.8em; }

  .refresh {
    padding: 4px 12px;
    font-size: 0.75em;
    border: 1px solid #444;
    border-radius: 4px;
    background: #2a2a3e;
    color: #e0e0e0;
    cursor: pointer;
  }

  .refresh:hover { background: #3a3a5e; }

  .group-head {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-top: 8px;
    padding-bottom: 2px;
    border-bottom: 1px solid #333;
  }

  .icon {
    display: inline-flex;
    width: 15px;
    height: 15px;
    color: #4fc3f7;
  }

  .icon svg { width: 100%; height: 100%; }

  .glabel { font-size: 0.85em; font-weight: 600; color: #cfd8dc; }

  .ghint { color: #666; font-size: 0.72em; }

  .table {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 10px;
    background: #22223a;
    border-radius: 4px;
    font-size: 0.82em;
  }

  .name {
    min-width: 130px;
    white-space: nowrap;
  }

  .url {
    min-width: 0;
    width: 210px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #777;
    font-size: 0.85em;
  }

  .bar-bg {
    flex: 1;
    height: 10px;
    background: #333;
    border-radius: 5px;
    overflow: hidden;
  }

  .bar {
    display: block;
    height: 100%;
    border-radius: 5px;
    transition: width 0.5s ease;
  }

  .ms {
    min-width: 50px;
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .status {
    min-width: 50px;
    font-size: 0.75em;
    text-transform: uppercase;
  }

  .ok { color: #66bb6a; }
  .err { color: #ef5350; }
</style>
