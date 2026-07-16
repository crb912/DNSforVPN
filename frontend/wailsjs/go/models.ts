export namespace cache {
	
	export class Stats {
	    Size: number;
	    HitRate: number;
	    MemBytes: number;
	    DiskBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Size = source["Size"];
	        this.HitRate = source["HitRate"];
	        this.MemBytes = source["MemBytes"];
	        this.DiskBytes = source["DiskBytes"];
	    }
	}

}

export namespace main {
	
	export class CacheConfig {
	    db_path: string;
	    max_hot_size: number;
	    save_interval: number;
	
	    static createFrom(source: any = {}) {
	        return new CacheConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.db_path = source["db_path"];
	        this.max_hot_size = source["max_hot_size"];
	        this.save_interval = source["save_interval"];
	    }
	}
	export class CacheEntryView {
	    domain: string;
	    qtype: string;
	    records: string[];
	    ttl_remaining: number;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CacheEntryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.qtype = source["qtype"];
	        this.records = source["records"];
	        this.ttl_remaining = source["ttl_remaining"];
	        this.expired = source["expired"];
	    }
	}
	export class LogConfig {
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new LogConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	    }
	}
	export class ProxyConfig {
	    enable_proxy: boolean;
	    http: string;
	    https: string;
	    rule_file: string;
	    rule_file_url: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable_proxy = source["enable_proxy"];
	        this.http = source["http"];
	        this.https = source["https"];
	        this.rule_file = source["rule_file"];
	        this.rule_file_url = source["rule_file_url"];
	    }
	}
	export class DNSConfig {
	    host: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new DNSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	    }
	}
	export class DOHServers {
	    direct_servers: string[];
	    proxy_servers: string[];
	    bootstrap_server: string;
	
	    static createFrom(source: any = {}) {
	        return new DOHServers(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.direct_servers = source["direct_servers"];
	        this.proxy_servers = source["proxy_servers"];
	        this.bootstrap_server = source["bootstrap_server"];
	    }
	}
	export class Config {
	    doh_servers: DOHServers;
	    dns: DNSConfig;
	    cache: CacheConfig;
	    proxy: ProxyConfig;
	    logging: LogConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doh_servers = this.convertValues(source["doh_servers"], DOHServers);
	        this.dns = this.convertValues(source["dns"], DNSConfig);
	        this.cache = this.convertValues(source["cache"], CacheConfig);
	        this.proxy = this.convertValues(source["proxy"], ProxyConfig);
	        this.logging = this.convertValues(source["logging"], LogConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomDNSEntry {
	    domain: string;
	    ips: string[];
	
	    static createFrom(source: any = {}) {
	        return new CustomDNSEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.ips = source["ips"];
	    }
	}
	
	
	

}

export namespace query {
	
	export class Stats {
	    total_queries: number;
	    cache_hits: number;
	    cache_misses: number;
	    total_errors: number;
	    avg_latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_queries = source["total_queries"];
	        this.cache_hits = source["cache_hits"];
	        this.cache_misses = source["cache_misses"];
	        this.total_errors = source["total_errors"];
	        this.avg_latency_ms = source["avg_latency_ms"];
	    }
	}

}

export namespace upstream {
	
	export class ServerLatency {
	    server_url: string;
	    latency_ms: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerLatency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.latency_ms = source["latency_ms"];
	        this.status = source["status"];
	    }
	}

}

