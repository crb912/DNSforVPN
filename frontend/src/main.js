import App from './App.svelte';

const app = new App({
  target: document.getElementById('app'),
});

// PWA: cache the app shell so the UI is installable and loads offline.
// API calls always go to the network (see public/sw.js).
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  });
}

export default app;
