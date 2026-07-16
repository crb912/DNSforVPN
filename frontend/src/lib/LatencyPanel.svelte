<script>
  import { onMount } from 'svelte';

  let go = window.go?.main?.App;
  let latencies = [];
  let loading = false;
  let timer;

  async function refresh() {
    if (!go) return;
    loading = true;
    try {
      latencies = await go.CheckLatency();
    } catch { latencies = []; }
    loading = false;
  }

  onMount(() => {
    refresh();
    timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  });

  function barWidth(ms) {
    return Math.min(100, Math.max(2, ms / 5)) + '%';
  }

  function barColor(ms, status) {
    if (status !== 'ok') return '#ef5350';
    if (ms < 20) return '#66bb6a';
    if (ms < 100) return '#ffa726';
    return '#ef5350';
  }
</script>

<div class="latency">
  <header>
    <h3>Server Latency</h3>
    <button class="refresh" on:click={refresh} disabled={loading}>
      {loading ? '...' : 'Refresh'}
    </button>
  </header>

  {#if latencies.length === 0}
    <p class="empty">Start the server to see latency data</p>
  {:else}
    <div class="table">
      {#each latencies as srv}
        <div class="row">
          <span class="url" title={srv.server_url}>{srv.server_url}</span>
          <span class="bar-bg">
            <span
              class="bar"
              style="width: {barWidth(srv.latency_ms)}; background: {barColor(srv.latency_ms, srv.status)}"
            ></span>
          </span>
          <span class="ms">{srv.latency_ms.toFixed(0)} ms</span>
          <span class="status" class:ok={srv.status === 'ok'} class:err={srv.status !== 'ok'}>
            {srv.status}
          </span>
        </div>
      {/each}
    </div>
  {/if}
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

  .empty { color: #888; font-size: 0.85em; }

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

  .url {
    min-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
