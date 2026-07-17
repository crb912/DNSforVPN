<script>
  import { onMount } from 'svelte';
  import * as api from './api.js';
  import ConfigPanel from './lib/ConfigPanel.svelte';
  import LatencyPanel from './lib/LatencyPanel.svelte';
  import StatsPanel from './lib/StatsPanel.svelte';
  import CachePanel from './lib/CachePanel.svelte';
  import CustomDNSPanel from './lib/CustomDNSPanel.svelte';

  let activeTab = 'status';

  let status = 'stopped';
  let statusTimer;

  onMount(() => {
    const poll = async () => {
      try {
        status = await api.GetStatus();
      } catch { status = 'error'; }
    };
    poll();
    statusTimer = setInterval(poll, 2000);
    return () => clearInterval(statusTimer);
  });

  async function handleStart() {
    try {
      status = await api.Start();
    } catch (err) {
      status = 'start error: ' + err.message;
    }
  }

  async function handleStop() {
    try {
      status = await api.Stop();
    } catch { /* poll will refresh */ }
  }

  const tabs = [
    { id: 'status', label: 'Status & Latency' },
    { id: 'config', label: 'Configuration' },
    { id: 'stats', label: 'Statistics' },
    { id: 'cache', label: 'Cache' },
    { id: 'custom', label: 'Custom DNS' },
  ];
</script>

<main class="app">
  <header class="toolbar">
    <h1>DNSforVPN</h1>
    <div class="controls">
      <span class:running={status === 'running'} class:stopped={status !== 'running'}>
        ● {status}
      </span>
      <button on:click={handleStart} disabled={status === 'running'}>Start</button>
      <button on:click={handleStop}  disabled={status !== 'running'}>Stop</button>
    </div>
  </header>

  <nav>
    {#each tabs as tab}
      <button
        class:active={activeTab === tab.id}
        on:click={() => (activeTab = tab.id)}
      >
        {tab.label}
      </button>
    {/each}
  </nav>

  <section class="content">
    {#if activeTab === 'status'}
      <LatencyPanel />
    {/if}
    {#if activeTab === 'config'}
      <ConfigPanel />
    {/if}
    {#if activeTab === 'stats'}
      <StatsPanel />
    {/if}
    {#if activeTab === 'cache'}
      <CachePanel />
    {/if}
    {#if activeTab === 'custom'}
      <CustomDNSPanel />
    {/if}
  </section>
</main>

<style>
  .app {
    max-width: 960px;
    margin: 0 auto;
    padding: 16px;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
  }

  .toolbar h1 {
    font-size: 1.4em;
    color: #4fc3f7;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .controls span {
    font-size: 0.85em;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .running { color: #66bb6a; }
  .stopped { color: #ef5350; }

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

  nav {
    display: flex;
    gap: 4px;
    margin-bottom: 16px;
    border-bottom: 1px solid #333;
  }

  nav button {
    border: none;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    padding: 8px 20px;
    background: transparent;
  }

  nav button.active {
    border-bottom-color: #4fc3f7;
    color: #4fc3f7;
  }

  .content {
    padding: 8px 0;
  }
</style>
