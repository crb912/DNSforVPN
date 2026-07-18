<script>
  import { onMount } from 'svelte';
  import * as api from '../api.js';
  import { CATALOG } from '../doh-catalog.js';

  let config = {
    doh_servers: { direct_servers: [], proxy_servers: [], bootstrap_server: '' },
    dns: { host: '0.0.0.0', port: 53, mode: 'rules' },
    cache: { db_path: 'data/dns_cache.db', max_hot_size: 100000, save_interval: 72 },
    proxy: { enable_proxy: false, http: '', https: '', rule_file: 'gfwlist.txt', rule_file_url: '' },
    web: { host: '127.0.0.1', port: 8080, username: '', password: '' },
    logging: { level: 'info' },
  };
  let saved = false;
  let error = '';
  let newUrl = { direct_servers: '', proxy_servers: '' };

  onMount(load);

  async function load() {
    try {
      config = await api.GetConfig();
      config.dns.mode ||= 'rules'; // configs written before mode existed
    } catch (e) { error = String(e); }
  }

  async function save() {
    error = '';
    try {
      await api.SaveConfig(config);
      saved = true;
      setTimeout(() => (saved = false), 2000);
    } catch (e) { error = String(e); }
  }

  const hostOf = (url) => { try { return new URL(url).hostname; } catch { return url; } };

  // Catalog rows removed via × are hidden; the set is persisted in
  // localStorage. Re-adding the same URL via + Add unhides it.
  const HIDDEN_KEY = 'config.hiddenCatalog.v1';
  let hidden = loadHidden();

  function loadHidden() {
    try { return new Set(JSON.parse(localStorage.getItem(HIDDEN_KEY)) ?? []); } catch { return new Set(); }
  }

  function saveHidden() {
    try { localStorage.setItem(HIDDEN_KEY, JSON.stringify([...hidden])); } catch { /* private mode */ }
  }

  // Candidate rows for one group: visible catalog entries first, then any
  // configured URL missing from the catalog (custom). `enabled` mirrors
  // config membership.
  function buildRows(inCfg, group, hiddenSet) {
    const cfg = inCfg ?? [];
    const rows = CATALOG.filter((c) => c.group === group && !hiddenSet.has(c.url))
      .map((c) => ({ ...c, custom: false }));
    for (const url of cfg) {
      if (url && !rows.some((r) => r.url === url)) {
        rows.push({ name: hostOf(url), url, custom: true });
      }
    }
    return rows.map((r) => ({ ...r, enabled: cfg.includes(r.url) }));
  }

  $: directRows = buildRows(config.doh_servers.direct_servers, 'direct', hidden);
  $: proxyRows = buildRows(config.doh_servers.proxy_servers, 'proxy', hidden);

  function toggle(listKey, url, on) {
    const cur = config.doh_servers[listKey] ?? [];
    const next = on ? [...cur, url] : cur.filter((u) => u !== url);
    config.doh_servers[listKey] = [...new Set(next)];
  }

  // × removes the row entirely: out of config, and for catalog entries out of
  // the candidate list as well.
  function removeRow(listKey, row) {
    toggle(listKey, row.url, false);
    if (!row.custom) {
      hidden = new Set(hidden).add(row.url);
      saveHidden();
    }
  }

  function addCustom(listKey) {
    const url = newUrl[listKey].trim();
    if (!url) return;
    if (hidden.has(url)) {
      hidden.delete(url);
      hidden = new Set(hidden);
      saveHidden();
    }
    toggle(listKey, url, true);
    newUrl[listKey] = '';
  }
</script>

<div class="config">
  <div class="topbar">
    <button on:click={load}>Load Config</button>
    <button class="save-btn" on:click={save}>Save Config</button>
    {#if saved}<span class="ok">✓ Saved</span>{/if}
    {#if error}<span class="err">{error}</span>{/if}
  </div>

  <h3>DNS Listen</h3>
  <label>Host: <input bind:value={config.dns.host} /></label>
  <label>Port: <input type="number" bind:value={config.dns.port} /></label>
  <div class="mode-row">
    <span class="mode-label">DNS Mode:</span>
    <label class="mode-opt">
      <input type="radio" name="dns_mode" value="direct" bind:group={config.dns.mode} /> Direct
    </label>
    <label class="mode-opt">
      <input type="radio" name="dns_mode" value="proxy" bind:group={config.dns.mode} /> Proxy
    </label>
    <label class="mode-opt">
      <input type="radio" name="dns_mode" value="rules" bind:group={config.dns.mode} /> Rules
    </label>
  </div>

  <h3>Direct DoH Servers</h3>
  <div class="servers">
    {#each directRows as c (c.url)}
      <div class="srv-row">
        <label class="srv">
          <input
            type="checkbox"
            checked={c.enabled}
            on:change={(e) => toggle('direct_servers', c.url, e.currentTarget.checked)}
          />
          <span class="srv-name">{c.name}</span>
          <span class="srv-url" title={c.url}>{c.url}</span>
        </label>
        <button class="sm danger" title="Remove from candidates"
                on:click={() => removeRow('direct_servers', c)}>×</button>
      </div>
    {/each}
    <div class="srv-row add">
      <input
        bind:value={newUrl.direct_servers}
        placeholder="Add custom DoH URL, e.g. https://example.com/dns-query"
        on:keydown={(e) => e.key === 'Enter' && addCustom('direct_servers')}
      />
      <button class="sm" on:click={() => addCustom('direct_servers')}>+ Add</button>
    </div>
  </div>

  <h3>Proxy DoH Servers</h3>
  <div class="servers">
    {#each proxyRows as c (c.url)}
      <div class="srv-row">
        <label class="srv">
          <input
            type="checkbox"
            checked={c.enabled}
            on:change={(e) => toggle('proxy_servers', c.url, e.currentTarget.checked)}
          />
          <span class="srv-name">{c.name}</span>
          <span class="srv-url" title={c.url}>{c.url}</span>
        </label>
        <button class="sm danger" title="Remove from candidates"
                on:click={() => removeRow('proxy_servers', c)}>×</button>
      </div>
    {/each}
    <div class="srv-row add">
      <input
        bind:value={newUrl.proxy_servers}
        placeholder="Add custom DoH URL, e.g. https://example.com/dns-query"
        on:keydown={(e) => e.key === 'Enter' && addCustom('proxy_servers')}
      />
      <button class="sm" on:click={() => addCustom('proxy_servers')}>+ Add</button>
    </div>
  </div>

  <h3>Bootstrap</h3>
  <input bind:value={config.doh_servers.bootstrap_server} placeholder="223.5.5.5" />

  <h3>Proxy Settings</h3>
  <label>
    <input type="checkbox" bind:checked={config.proxy.enable_proxy} />
    Enable Proxy
  </label>
  {#if config.proxy.enable_proxy}
    <label>HTTP: <input bind:value={config.proxy.http} /></label>
    <label>HTTPS: <input bind:value={config.proxy.https} /></label>
  {/if}

  <h3>Cache</h3>
  <label>DB Path: <input bind:value={config.cache.db_path} /></label>
  <label>Hot Size: <input type="number" bind:value={config.cache.max_hot_size} /></label>
  <label>Save Interval (h): <input type="number" bind:value={config.cache.save_interval} /></label>

  <h3>Web UI</h3>
  <label>Host: <input bind:value={config.web.host} placeholder="127.0.0.1" /></label>
  <label>Port: <input type="number" bind:value={config.web.port} placeholder="8080" /></label>
  <label>Username: <input bind:value={config.web.username} placeholder="(optional)" /></label>
  <label>Password: <input type="password" bind:value={config.web.password} placeholder="(empty = no auth)" /></label>

  <h3>Logging</h3>
  <select bind:value={config.logging.level}>
    <option>debug</option>
    <option>info</option>
    <option>warn</option>
    <option>error</option>
  </select>
</div>

<style>
  .config {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  h3 {
    margin-top: 16px;
    color: #4fc3f7;
    font-size: 0.95em;
    border-bottom: 1px solid #333;
    padding-bottom: 4px;
  }

  label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.85em;
  }

  input, select {
    flex: 1;
    padding: 5px 8px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #2a2a3e;
    color: #e0e0e0;
    font-size: 0.85em;
  }

  input[type="checkbox"] { flex: none; }

  .servers {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .srv-row {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .srv {
    flex: 1;
    padding: 4px 8px;
    background: #22223a;
    border-radius: 4px;
    cursor: pointer;
  }

  .srv input[type='checkbox'] {
    margin: 0;
    accent-color: #4fc3f7;
    cursor: pointer;
  }

  .srv-name {
    min-width: 130px;
    white-space: nowrap;
  }

  .srv-url {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: #777;
    font-size: 0.85em;
  }

  .add input { flex: 1; }

  button {
    padding: 6px 16px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #2a2a3e;
    color: #e0e0e0;
    cursor: pointer;
    font-size: 0.85em;
  }

  button:hover { background: #3a3a5e; }

  .sm { padding: 4px 12px; font-size: 0.75em; }
  .danger { color: #ef5350; }

  .topbar {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .mode-row {
    display: flex;
    align-items: center;
    gap: 16px;
    font-size: 0.85em;
  }

  .mode-label { color: #cfd8dc; }

  .mode-opt {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }

  .mode-opt input[type='radio'] {
    margin: 0;
    accent-color: #4fc3f7;
    cursor: pointer;
    flex: none;
  }

  .save-btn {
    background: #1565c0;
    border-color: #1565c0;
    padding: 8px 24px;
  }

  .save-btn:hover { background: #1976d2; }

  .ok { color: #66bb6a; font-size: 0.85em; }
  .err { color: #ef5350; font-size: 0.85em; }
</style>
