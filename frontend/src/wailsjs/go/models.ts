export namespace config {
	
	export class AdvancedConfig {
	    proxyPort: number;
	    probeInterval: number;
	    healthCheckInterval: number;
	    dohProviders: string[];
	    maxIPsPerDomain: number;
	    preferredMode: string;
	
	    static createFrom(source: any = {}) {
	        return new AdvancedConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.proxyPort = source["proxyPort"];
	        this.probeInterval = source["probeInterval"];
	        this.healthCheckInterval = source["healthCheckInterval"];
	        this.dohProviders = source["dohProviders"];
	        this.maxIPsPerDomain = source["maxIPsPerDomain"];
	        this.preferredMode = source["preferredMode"];
	    }
	}
	export class SiteConfig {
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SiteConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	    }
	}
	export class AppConfig {
	    version: string;
	    sites: Record<string, SiteConfig>;
	    advanced: AdvancedConfig;
	    customSites: string[];
	    autostart: boolean;
	    minimizeToTray: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.sites = this.convertValues(source["sites"], SiteConfig, true);
	        this.advanced = this.convertValues(source["advanced"], AdvancedConfig);
	        this.customSites = source["customSites"];
	        this.autostart = source["autostart"];
	        this.minimizeToTray = source["minimizeToTray"];
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

}

export namespace intercept {
	
	export class SiteStatus {
	    name: string;
	    icon: string;
	    enabled: boolean;
	    bestIP: string;
	    latency: number;
	    domains: number;
	    connected: number;
	
	    static createFrom(source: any = {}) {
	        return new SiteStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.enabled = source["enabled"];
	        this.bestIP = source["bestIP"];
	        this.latency = source["latency"];
	        this.domains = source["domains"];
	        this.connected = source["connected"];
	    }
	}
	export class Status {
	    running: boolean;
	    mode: string;
	    sites: Record<string, SiteStatus>;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.mode = source["mode"];
	        this.sites = this.convertValues(source["sites"], SiteStatus, true);
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

}

export namespace logger {
	
	export class LogEntry {
	    // Go type: time
	    time: any;
	    level: string;
	    msg: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
	        this.msg = source["msg"];
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

}

