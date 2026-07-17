<script>
  import { onMount } from 'svelte';
  import * as api from '../api.js';

  let config = {
    doh_servers: { direct_servers: [], proxy_servers: [], bootstrap_server: '' },
    dns: { host: '0.0.0.0', port: 53 },
    cache: { db_path: 'data/dns_cache.db', max_hot_size: 100000, save_interval: 72 },
    proxy: { enable_proxy: false, http: '', https: '', rule_file: 'gfwlist.txt', rule_file_url: '' },
    web: { host: '127.0.0.1', port: 8080, username: '', password: '' },
    logging: { level: 'info' },
  };
  let saved = false;
  let error = '';

  onMount(load);

  async function load() {
    try { config = await api.GetConfig(); } catch (e) { error = String(e); }
  }

  async function save() {
    error = '';
    try {
      await api.SaveConfig(config);
      saved = true;
      setTimeout(() => (saved = false), 2000);
    } catch (e) { error = String(e); }
  }

  function addServer(list) {
    config.doh_servers[list] = [...config.doh_servers[list], ''];
  }

  function removeServer(list, i) {
    config.doh_servers[list] = config.doh_servers[list].filter((_, j) => j !== i);
  }
</script>

<div class="config">
  <button class="load-btn" on:click={load}>Load Config</button>

  <h3>DNS Listen</h3>
  <label>Host: <input bind:value={config.dns.host} /></label>
  <label>Port: <input type="number" bind:value={config.dns.port} /></label>

  <h3>Direct DoH Servers</h3>
  {#each config.doh_servers.direct_servers as srv, i}
    <div class="row">
      <input bind:value={config.doh_servers.direct_servers[i]} placeholder="https://doh.pub/dns-query" />
      <button class="sm danger" on:click={() => removeServer('direct_servers', i)}>×</button>
    </div>
  {/each}
  <button class="sm" on:click={() => addServer('direct_servers')}>+ Add</button>

  <h3>Proxy DoH Servers</h3>
  {#each config.doh_servers.proxy_servers as srv, i}
    <div class="row">
      <input bind:value={config.doh_servers.proxy_servers[i]} placeholder="https://dns.google/dns-query" />
      <button class="sm danger" on:click={() => removeServer('proxy_servers', i)}>×</button>
    </div>
  {/each}
  <button class="sm" on:click={() => addServer('proxy_servers')}>+ Add</button>

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

  <footer>
    <button class="save-btn" on:click={save}>Save Config</button>
    {#if saved}<span class="ok">✓ Saved</span>{/if}
    {#if error}<span class="err">{error}</span>{/if}
  </footer>
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

  .row {
    display: flex;
    gap: 4px;
  }

  .row input { flex: 1; }

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
  .load-btn { align-self: flex-start; }

  footer {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid #333;
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
