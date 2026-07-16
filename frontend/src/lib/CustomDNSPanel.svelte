<script>
  import { onMount } from 'svelte';

  let go = window.go?.main?.App;
  let entries = [];
  let domain = '';
  let ips = '';
  let error = '';
  let notice = '';

  async function refresh() {
    if (!go) return;
    try {
      entries = (await go.GetCustomDNS()) ?? [];
    } catch (e) {
      error = String(e);
    }
  }

  onMount(refresh);

  async function save() {
    if (!go) return;
    error = '';
    notice = '';
    const list = ips.split(',').map(s => s.trim()).filter(s => s !== '');
    try {
      await go.SetCustomDNS(domain.trim(), list);
      notice = `Saved: ${domain.trim().toLowerCase()}`;
      domain = '';
      ips = '';
      await refresh();
    } catch (e) {
      error = String(e);
    }
  }

  async function remove(d) {
    if (!go) return;
    error = '';
    notice = '';
    try {
      await go.DeleteCustomDNS(d);
      notice = `Deleted: ${d}`;
      await refresh();
    } catch (e) {
      error = String(e);
    }
  }
</script>

<div class="custom-dns">
  <h3>Add / Update Override</h3>
  <div class="form">
    <input
      class="domain-input"
      bind:value={domain}
      placeholder="Domain, e.g. myapp.local"
    />
    <input
      class="ips-input"
      bind:value={ips}
      placeholder="IPs, comma separated, e.g. 10.0.0.1, 10.0.0.2"
    />
    <button on:click={save} disabled={domain.trim() === '' || ips.trim() === ''}>
      Save
    </button>
  </div>

  {#if error}
    <div class="error">{error}</div>
  {/if}
  {#if notice}
    <div class="notice">{notice}</div>
  {/if}

  <h3>Overrides ({entries.length})</h3>
  <div class="list">
    {#each entries as e (e.domain)}
      <div class="row">
        <span class="name">{e.domain}</span>
        <span class="chips">
          {#each e.ips as ip}
            <span class="chip">{ip}</span>
          {/each}
        </span>
        <button class="sm danger" on:click={() => remove(e.domain)}>×</button>
      </div>
    {:else}
      <div class="empty">No custom DNS overrides</div>
    {/each}
  </div>

  <p class="hint">
    Overrides answer A / AAAA queries for the exact domain with a fixed TTL of 300s,
    taking precedence over cache and upstream resolvers.
  </p>
</div>

<style>
  .custom-dns {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  h3 {
    color: #4fc3f7;
    font-size: 0.95em;
    margin: 8px 0 0;
  }

  .form {
    display: flex;
    gap: 8px;
  }

  input {
    padding: 6px 10px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #1a1a2e;
    color: #e0e0e0;
    font-size: 0.85em;
  }

  .domain-input { width: 220px; }
  .ips-input { flex: 1; }

  button {
    padding: 6px 16px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #2a2a3e;
    color: #e0e0e0;
    cursor: pointer;
    font-size: 0.85em;
  }

  button:hover:not(:disabled) { background: #3a3a5e; }
  button:disabled { opacity: 0.4; cursor: not-allowed; }

  button.sm {
    padding: 2px 10px;
  }

  button.danger {
    color: #ef5350;
    border-color: #5e2a2a;
  }

  .error { color: #ef5350; font-size: 0.85em; }
  .notice { color: #66bb6a; font-size: 0.85em; }

  .list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    background: #22223a;
    border-radius: 6px;
    padding: 8px 12px;
  }

  .name {
    font-weight: 600;
    color: #e0e0e0;
    min-width: 180px;
    word-break: break-all;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    flex: 1;
  }

  .chip {
    background: #1a1a2e;
    border: 1px solid #444;
    border-radius: 10px;
    padding: 2px 10px;
    font-family: monospace;
    font-size: 0.8em;
    color: #4fc3f7;
  }

  .empty {
    color: #666;
    text-align: center;
    padding: 16px;
  }

  .hint {
    font-size: 0.75em;
    color: #888;
    margin-top: 8px;
  }
</style>
