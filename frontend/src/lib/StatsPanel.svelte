<script>
  import { onMount } from 'svelte';

  let go = window.go?.main?.App;
  let cacheStats = { size: 0, hit_rate: 0, mem_bytes: 0, disk_bytes: 0 };
  let queryStats = { total_queries: 0, cache_hits: 0, cache_misses: 0, total_errors: 0, avg_latency_ms: 0 };
  let timer;

  async function refresh() {
    if (!go) return;
    try {
      cacheStats = await go.GetCacheStats();
      queryStats = await go.GetQueryStats();
    } catch { /* server not started yet */ }
  }

  onMount(() => {
    refresh();
    timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  });

  $: hitRate = queryStats.total_queries > 0
    ? (queryStats.cache_hits / queryStats.total_queries * 100).toFixed(1)
    : '0.0';

  $: cacheSize = queryStats.cache_hits + queryStats.cache_misses;

  function fmt(n) {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
    return String(n);
  }

  function fmtBytes(n) {
    if (n >= 1_048_576) return (n / 1_048_576).toFixed(1) + ' MB';
    if (n >= 1_024) return (n / 1_024).toFixed(1) + ' KB';
    return n + ' B';
  }
</script>

<div class="stats">
  <h3>Query Statistics</h3>

  <div class="cards">
    <div class="card">
      <span class="value">{fmt(queryStats.total_queries)}</span>
      <span class="label">Total Queries</span>
    </div>
    <div class="card">
      <span class="value">{hitRate}%</span>
      <span class="label">Cache Hit Rate</span>
    </div>
    <div class="card">
      <span class="value">{queryStats.avg_latency_ms.toFixed(1)} ms</span>
      <span class="label">Avg Latency</span>
    </div>
    <div class="card">
      <span class="value">{fmt(queryStats.total_errors)}</span>
      <span class="label">Errors</span>
    </div>
  </div>

  <div class="bar-wrap">
    <div class="bar-label">Hits ({queryStats.cache_hits}) / Misses ({queryStats.cache_misses})</div>
    <div class="bar-bg">
      {#if cacheSize > 0}
        <span class="bar-hit" style="width: {hitRate}%"></span>
        <span class="bar-miss" style="width: {100 - Number(hitRate)}%"></span>
      {/if}
    </div>
  </div>

  <h3 style="margin-top: 24px">Cache Storage</h3>

  <div class="cards">
    <div class="card">
      <span class="value">{fmt(cacheStats.size)}</span>
      <span class="label">Entries</span>
    </div>
    <div class="card">
      <span class="value">{fmtBytes(cacheStats.mem_bytes)}</span>
      <span class="label">Memory</span>
    </div>
    <div class="card">
      <span class="value">{fmtBytes(cacheStats.disk_bytes)}</span>
      <span class="label">Disk</span>
    </div>
    <div class="card">
      <span class="value">{(cacheStats.hit_rate * 100).toFixed(1)}%</span>
      <span class="label">Cache Hit Rate</span>
    </div>
  </div>
</div>

<style>
  .stats {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  h3 {
    color: #4fc3f7;
    font-size: 0.95em;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
  }

  .card {
    background: #22223a;
    border-radius: 8px;
    padding: 14px 10px;
    text-align: center;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .value {
    font-size: 1.3em;
    font-weight: 600;
    color: #e0e0e0;
    font-variant-numeric: tabular-nums;
  }

  .label {
    font-size: 0.72em;
    color: #888;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .bar-wrap {
    margin-top: 8px;
  }

  .bar-label {
    font-size: 0.75em;
    color: #888;
    margin-bottom: 4px;
  }

  .bar-bg {
    height: 12px;
    background: #333;
    border-radius: 6px;
    overflow: hidden;
    display: flex;
  }

  .bar-hit {
    background: #66bb6a;
    transition: width 0.5s ease;
  }

  .bar-miss {
    background: #ef5350;
    transition: width 0.5s ease;
  }
</style>
