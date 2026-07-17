<script>
  import { onMount } from 'svelte';
  import * as api from '../api.js';

  // Well-known public DoH providers. Configured servers not listed here
  // are appended at runtime so nothing in the user's config is hidden.
  const CATALOG = [
    { group: 'direct', name: 'DNSPod (腾讯)',   url: 'https://doh.pub/dns-query' },
    { group: 'direct', name: 'AliDNS (阿里)',   url: 'https://dns.alidns.com/dns-query' },
    { group: 'direct', name: '360 安全 DNS',    url: 'https://doh.360.cn/dns-query' },
    { group: 'direct', name: 'OneDNS',          url: 'https://doh.onedns.net/dns-query' },
    { group: 'proxy',  name: 'Google',          url: 'https://dns.google/dns-query' },
    { group: 'proxy',  name: 'Cloudflare',      url: 'https://1.1.1.1/dns-query' },
    { group: 'proxy',  name: 'Quad9 (IBM)',     url: 'https://dns.quad9.net/dns-query' },
    { group: 'proxy',  name: 'AdGuard',         url: 'https://dns.adguard-dns.com/dns-query' },
    { group: 'proxy',  name: 'OpenDNS (Cisco)', url: 'https://doh.opendns.com/dns-query' },
    { group: 'proxy',  name: 'Mullvad',         url: 'https://dns.mullvad.net/dns-query' },
    { group: 'proxy',  name: 'DNS.SB',          url: 'https://doh.dns.sb/dns-query' },
    { group: 'proxy',  name: 'SWITCH',          url: 'https://dns.switch.ch/dns-query' },
  ];

  const GROUPS = [
    { id: 'direct', label: 'Direct', hint: 'probed without proxy' },
    { id: 'proxy',  label: 'Proxy',  hint: 'probed through configured proxy' },
  ];

  const STORE_KEY = 'latency.enabled.v1';

  let servers = [];
  let loading = false;
  let lastCheck = '';

  const keyOf = (s) => s.group + '|' + s.url;
  const hostOf = (url) => { try { return new URL(url).hostname; } catch { return url; } };

  function loadEnabled() {
    try { return JSON.parse(localStorage.getItem(STORE_KEY)) ?? {}; } catch { return {}; }
  }

  function saveEnabled() {
    const m = {};
    for (const s of servers) m[keyOf(s)] = s.enabled;
    try { localStorage.setItem(STORE_KEY, JSON.stringify(m)); } catch { /* private mode */ }
  }

  onMount(async () => {
    let cfg = null;
    try { cfg = await api.GetConfig(); } catch { /* config unreachable */ }
    const confDirect = cfg?.doh_servers?.direct_servers ?? [];
    const confProxy = cfg?.doh_servers?.proxy_servers ?? [];

    const list = CATALOG.map((e) => ({
      ...e,
      configured: (e.group === 'direct' ? confDirect : confProxy).includes(e.url),
    }));
    for (const url of confDirect) {
      if (!list.some((s) => s.group === 'direct' && s.url === url)) {
        list.push({ group: 'direct', name: hostOf(url), url, configured: true });
      }
    }
    for (const url of confProxy) {
      if (!list.some((s) => s.group === 'proxy' && s.url === url)) {
        list.push({ group: 'proxy', name: hostOf(url), url, configured: true });
      }
    }

    const stored = loadEnabled();
    servers = list.map((s) => ({
      ...s,
      enabled: stored[keyOf(s)] ?? s.configured,
      ms: 0,
      status: '',
      checked: false,
    }));
    await refresh();
  });

  async function refresh() {
    const targets = servers
      .filter((s) => s.enabled)
      .map((s) => ({ url: s.url, via_proxy: s.group === 'proxy' }));
    if (targets.length === 0) return;
    loading = true;
    try {
      const res = (await api.CheckServersLatency(targets)) ?? [];
      const byUrl = {};
      for (const r of res) byUrl[r.server_url] = r;
      servers = servers.map((s) => {
        if (!s.enabled) return s;
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

  {#each GROUPS as g}
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
      {#each servers.filter((s) => s.group === g.id) as srv (keyOf(srv))}
        <label class="row" class:disabled={!srv.enabled}>
          <input type="checkbox" bind:checked={srv.enabled} on:change={saveEnabled} />
          <span class="name" title={srv.url}>
            {srv.name}
            {#if srv.configured}<span class="badge">in use</span>{/if}
          </span>
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
        </label>
      {/each}
    </div>
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
    cursor: pointer;
  }

  .row.disabled { opacity: 0.55; }

  .row input[type='checkbox'] {
    margin: 0;
    accent-color: #4fc3f7;
    cursor: pointer;
  }

  .name {
    min-width: 130px;
    white-space: nowrap;
  }

  .badge {
    margin-left: 6px;
    padding: 0 5px;
    font-size: 0.68em;
    color: #66bb6a;
    border: 1px solid #66bb6a55;
    border-radius: 3px;
    vertical-align: 1px;
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
