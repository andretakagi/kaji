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

export interface ReverseProxyHandler {
	handler: "reverse_proxy";
	upstreams?: CaddyUpstream[];
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
	headers?: {
		request?: {
			set?: Record<string, string[]>;
			add?: Record<string, string[]>;
			delete?: string[];
			replace?: Record<string, { search: string; replace: string }[]>;
		};
		response?: {
			set?: Record<string, string[]>;
			add?: Record<string, string[]>;
			delete?: string[];
			deferred?: boolean;
		};
	};
}

export interface HeadersHandler {
	handler: "headers";
	request?: {
		set?: Record<string, string[]>;
		add?: Record<string, string[]>;
		delete?: string[];
		replace?: Record<string, { search: string; replace: string }[]>;
	};
	response?: {
		set?: Record<string, string[]>;
		add?: Record<string, string[]>;
		delete?: string[];
		deferred?: boolean;
	};
}

export interface AuthenticationHandler {
	handler: "authentication";
	providers?: {
		http_basic?: {
			accounts?: { username: string; password: string }[];
		};
	};
}

export interface SubrouteHandler {
	handler: "subroute";
	routes?: CaddyRoute[];
}

export interface EncodeHandler {
	handler: "encode";
}

export interface StaticResponseHandler {
	handler: "static_response";
}

export interface FileServerHandler {
	handler: "file_server";
}

export interface CaddyHandler {
	handler: string;
	upstreams?: CaddyUpstream[];
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
	response?: {
		set?: Record<string, string[]>;
		add?: Record<string, string[]>;
		delete?: string[];
		deferred?: boolean;
	};
	headers?: {
		request?: {
			set?: Record<string, string[]>;
			add?: Record<string, string[]>;
			delete?: string[];
			replace?: Record<string, { search: string; replace: string }[]>;
		};
		response?: {
			set?: Record<string, string[]>;
			add?: Record<string, string[]>;
			delete?: string[];
			deferred?: boolean;
		};
	};
	providers?: {
		http_basic?: {
			accounts?: { username: string; password: string }[];
		};
	};
	routes?: CaddyRoute[];
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
