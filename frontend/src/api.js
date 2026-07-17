// REST API client for the dnsforvpn backend.
// Method names mirror the former Wails bindings so panel code stays familiar.

async function req(method, url, body) {
  const opt = { method, headers: {} };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(url, opt);
  if (!r.ok) {
    const text = (await r.text()).trim();
    throw new Error(text || r.statusText);
  }
  return r.json();
}

export const GetConfig = () => req('GET', '/api/config');
export const SaveConfig = (cfg) => req('PUT', '/api/config', cfg);
export const GetStatus = () => req('GET', '/api/status');
export const Start = () => req('POST', '/api/start');
export const Stop = () => req('POST', '/api/stop');
export const CheckLatency = () => req('GET', '/api/latency');
export const CheckServersLatency = (servers) =>
  req('POST', '/api/latency', { servers });
export const GetCacheStats = () => req('GET', '/api/stats/cache');
export const GetQueryStats = () => req('GET', '/api/stats/query');
export const ListCache = () => req('GET', '/api/cache');
export const QueryCache = (domain) =>
  req('GET', '/api/cache?domain=' + encodeURIComponent(domain));
export const GetCustomDNS = () => req('GET', '/api/custom-dns');
export const SetCustomDNS = (domain, ips) =>
  req('PUT', '/api/custom-dns', { domain, ips });
export const DeleteCustomDNS = (domain) =>
  req('DELETE', '/api/custom-dns?domain=' + encodeURIComponent(domain));
