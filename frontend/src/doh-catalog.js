// Well-known public DoH providers, shared by the latency and config panels.
// Configured servers not listed here are appended at runtime so nothing in
// the user's config is hidden.
export const CATALOG = [
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
