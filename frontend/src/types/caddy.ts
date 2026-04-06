export interface CaddyRoute {
	"@id"?: string;
	match?: CaddyMatch[];
	handle: CaddyHandler[];
	terminal?: boolean;
}

export interface CaddyMatch {
	host?: string[];
	path?: string[];
}

export interface CaddyHandler {
	handler:
		| "reverse_proxy"
		| "static_response"
		| "file_server"
		| "encode"
		| "headers"
		| "authentication"
		| "subroute";
	routes?: CaddyRoute[];
	upstreams?: CaddyUpstream[];
	headers?: Record<string, string[]>;
	response?: {
		set?: Record<string, string[]>;
	};
	load_balancing?: {
		selection_policy: {
			policy: "round_robin" | "first" | "least_conn" | "random" | "ip_hash";
		};
	};
	health_checks?: {
		passive?: {
			fail_duration: string;
			max_fails: number;
		};
	};
	flush_interval?: number;
	transport?: {
		protocol: string;
		tls?: { insecure_skip_verify: boolean };
	};
	providers?: {
		http_basic?: {
			accounts?: { username: string; password: string }[];
		};
	};
}

export interface CaddyUpstream {
	dial: string;
}

export interface CaddyServer {
	listen: string[];
	routes: CaddyRoute[];
	logs?: {
		logger_names?: Record<string, string>;
	};
}

export interface CaddyConfig {
	apps?: {
		http?: {
			servers?: Record<string, CaddyServer>;
		};
		tls?: {
			automation?: {
				policies?: CaddyTLSPolicy[];
			};
		};
	};
}

export interface CaddyTLSPolicy {
	subjects?: string[];
	issuers?: CaddyIssuer[];
}

export interface CaddyIssuer {
	module: "acme" | "internal";
	email?: string;
	challenges?: {
		dns?: {
			provider: {
				name: string;
				api_token?: string;
			};
		};
	};
}
