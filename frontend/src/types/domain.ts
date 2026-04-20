import type { HeadersConfig, RequestHeaders } from "./api";

export interface DomainToggles {
	force_https: boolean;
	compression: boolean;
	headers: HeadersConfig;
	basic_auth: {
		enabled: boolean;
		username: string;
		password_hash: string;
		password: string;
	};
	access_log: string;
	ip_filtering: {
		enabled: boolean;
		list_id: string;
		type: "whitelist" | "blacklist" | "";
	};
}

export interface ReverseProxyConfig {
	upstream: string;
	tls_skip_verify: boolean;
	websocket_passthrough: boolean;
	load_balancing: {
		enabled: boolean;
		strategy: "round_robin" | "first" | "least_conn" | "random" | "ip_hash";
		upstreams: string[];
	};
	request_headers: RequestHeaders;
}

export interface StaticResponseConfig {
	status_code: string;
	body: string;
	headers: Record<string, string[]>;
	close: boolean;
}

export interface RedirectConfig {
	target_url: string;
	status_code: string;
	preserve_path: boolean;
}

export interface FileServerConfig {
	root: string;
	browse: boolean;
	index_names: string[];
	hide: string[];
}

export type HandlerConfigValue =
	| ReverseProxyConfig
	| StaticResponseConfig
	| RedirectConfig
	| FileServerConfig
	| Record<string, unknown>;

export interface Rule {
	id: string;
	label: string;
	enabled: boolean;
	match_type: "" | "subdomain" | "path";
	path_match: "" | "exact" | "prefix" | "regex";
	match_value: string;
	handler_type: "reverse_proxy" | "redirect" | "file_server" | "static_response";
	handler_config: HandlerConfigValue;
	toggle_overrides: DomainToggles | null;
	advanced_headers: boolean;
}

export interface Domain {
	id: string;
	name: string;
	enabled: boolean;
	toggles: DomainToggles;
	rules: Rule[];
}

export type HandlerType = "reverse_proxy" | "redirect" | "file_server" | "static_response";
export type MatchType = "" | "subdomain" | "path";
export type PathMatch = "exact" | "prefix" | "regex";

export interface CreateDomainFullRule {
	label?: string;
	match_type: MatchType;
	path_match?: PathMatch;
	match_value?: string;
	handler_type: HandlerType;
	handler_config: HandlerConfigValue;
	toggle_overrides?: DomainToggles | null;
	advanced_headers?: boolean;
}

export interface CreateDomainFullRequest {
	name: string;
	toggles: DomainToggles;
	rules: CreateDomainFullRule[];
}

export interface UpdateDomainRequest {
	name: string;
	toggles: DomainToggles;
}

export interface CreateRuleRequest {
	label?: string;
	match_type: MatchType;
	path_match?: PathMatch;
	match_value?: string;
	handler_type: HandlerType;
	handler_config: HandlerConfigValue;
	toggle_overrides?: DomainToggles | null;
	advanced_headers?: boolean;
}

export interface UpdateRuleRequest extends CreateRuleRequest {}

export const defaultDomainToggles: DomainToggles = {
	force_https: true,
	compression: false,
	headers: {
		response: {
			enabled: false,
			security: false,
			cors: false,
			cors_origins: [],
			cache_control: false,
			x_robots_tag: false,
			builtin: [],
			custom: [],
		},
	},
	basic_auth: { enabled: false, username: "", password_hash: "", password: "" },
	access_log: "",
	ip_filtering: { enabled: false, list_id: "", type: "" },
};

export const defaultReverseProxyConfig: ReverseProxyConfig = {
	upstream: "",
	tls_skip_verify: false,
	websocket_passthrough: false,
	load_balancing: { enabled: false, strategy: "round_robin", upstreams: [] },
	request_headers: {
		enabled: false,
		host_override: false,
		host_value: "",
		authorization: false,
		auth_value: "",
		builtin: [],
		custom: [],
	},
};

export const defaultStaticResponseConfig: StaticResponseConfig = {
	status_code: "200",
	body: "",
	headers: {},
	close: false,
};

export const defaultRedirectConfig: RedirectConfig = {
	target_url: "",
	status_code: "301",
	preserve_path: false,
};

export const defaultFileServerConfig: FileServerConfig = {
	root: "",
	browse: false,
	index_names: ["index.html"],
	hide: [".*"],
};
