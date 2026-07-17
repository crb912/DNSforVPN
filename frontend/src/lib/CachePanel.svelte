<script>
  import { onMount } from 'svelte';
  import * as api from '../api.js';

  let entries = [];
  let filter = '';
  let error = '';
  let loading = false;

  async function refresh() {
    loading = true;
    error = '';
    try {
      entries = (await api.ListCache()) ?? [];
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  onMount(refresh);

  $: filtered = filter.trim() === ''
    ? entries
    : entries.filter(e => e.domain.toLowerCase().includes(filter.trim().toLowerCase()));

  function fmtTTL(s) {
    if (s <= 0) return '—';
    const m = Math.floor(s / 60);
    const sec = s % 60;
    if (m >= 60) return `${Math.floor(m / 60)}h ${m % 60}m`;
    if (m > 0) return `${m}m ${sec}s`;
    return `${sec}s`;
  }
</script>

<div class="cache-panel">
  <div class="toolbar">
    <input
      class="search"
      bind:value={filter}
      placeholder="Filter by domain…"
    />
    <button on:click={refresh} disabled={loading}>
      {loading ? 'Loading…' : 'Refresh'}
    </button>
  </div>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  <div class="summary">{filtered.length} entries (cap 500)</div>

  <table>
    <thead>
      <tr>
        <th>Domain</th>
        <th>Type</th>
        <th>Records</th>
        <th>TTL Left</th>
        <th>Status</th>
      </tr>
    </thead>
    <tbody>
      {#each filtered as e (e.domain + e.qtype)}
        <tr class:expired={e.expired}>
          <td class="domain">{e.domain}</td>
          <td>{e.qtype}</td>
          <td class="records">
            {#each e.records as r}
              <div class="rec">{r}</div>
            {/each}
          </td>
          <td class="ttl">{e.expired ? '—' : fmtTTL(e.ttl_remaining)}</td>
          <td>
            <span class:ok={!e.expired} class:bad={e.expired}>
              {e.expired ? 'Expired' : 'Active'}
            </span>
          </td>
        </tr>
      {:else}
        <tr><td colspan="5" class="empty">No cached entries</td></tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .cache-panel {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .toolbar {
    display: flex;
    gap: 8px;
  }

  .search {
    flex: 1;
    padding: 6px 10px;
    border: 1px solid #444;
    border-radius: 4px;
    background: #1a1a2e;
    color: #e0e0e0;
    font-size: 0.85em;
  }

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

  .error {
    color: #ef5350;
    font-size: 0.85em;
  }

  .summary {
    font-size: 0.75em;
    color: #888;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.82em;
  }

  th {
    text-align: left;
    color: #4fc3f7;
    font-weight: 600;
    padding: 6px 8px;
    border-bottom: 1px solid #333;
  }

  td {
    padding: 6px 8px;
    border-bottom: 1px solid #2a2a3e;
    vertical-align: top;
    color: #e0e0e0;
  }

  tr.expired td {
    color: #777;
  }

  .domain {
    font-weight: 600;
    word-break: break-all;
  }

  .records .rec {
    font-family: monospace;
    font-size: 0.95em;
    word-break: break-all;
  }

  .ttl {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .ok { color: #66bb6a; }
  .bad { color: #ef5350; }

  .empty {
    text-align: center;
    color: #666;
    padding: 20px;
  }
</style>
