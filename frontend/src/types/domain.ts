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

export interface ErrorConfig {
	status_code: string;
	message: string;
}

export type HandlerConfigValue =
	| ReverseProxyConfig
	| StaticResponseConfig
	| RedirectConfig
	| FileServerConfig
	| ErrorConfig
	| Record<string, never>;

export type HandlerType =
	| "reverse_proxy"
	| "redirect"
	| "file_server"
	| "static_response"
	| "error";
export type RuleHandlerType = "none" | HandlerType;
export type PathMatch = "exact" | "prefix" | "regex";

export interface Rule {
	handler_type: RuleHandlerType;
	handler_config: HandlerConfigValue;
	advanced_headers: boolean;
	enabled: boolean;
}

export interface Path {
	id: string;
	label?: string;
	enabled: boolean;
	path_match: PathMatch;
	match_value: string;
	rule: Rule;
	toggle_overrides?: DomainToggles | null;
}

export interface Subdomain {
	id: string;
	name: string;
	enabled: boolean;
	toggles: DomainToggles;
	rule: Rule;
	paths: Path[];
}

export interface Domain {
	id: string;
	name: string;
	enabled: boolean;
	toggles: DomainToggles;
	rule: Rule;
	subdomains: Subdomain[];
	paths: Path[];
}

export interface UpdateRuleRequest {
	handler_type: RuleHandlerType;
	handler_config: HandlerConfigValue;
	advanced_headers: boolean;
}

export interface CreatePathRequest {
	label?: string;
	path_match: PathMatch;
	match_value: string;
	rule: UpdateRuleRequest;
	toggle_overrides?: DomainToggles | null;
}

export type UpdatePathRequest = CreatePathRequest;

export interface CreateSubdomainRequest {
	name: string;
	rule: UpdateRuleRequest;
	toggles?: DomainToggles;
	paths?: CreatePathRequest[];
}

export interface UpdateSubdomainRequest {
	name: string;
	toggles: DomainToggles;
}

export interface UpdateDomainRequest {
	name: string;
	toggles: DomainToggles;
}

export interface CreateDomainFullRequest {
	name: string;
	toggles: DomainToggles;
	rule: UpdateRuleRequest;
	subdomains?: CreateSubdomainRequest[];
	paths?: CreatePathRequest[];
}

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
	status_code: "",
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
	hide: [],
};

export const defaultErrorConfig: ErrorConfig = {
	status_code: "404",
	message: "",
};

export const pathMatchOptions: { value: PathMatch; label: string }[] = [
	{ value: "prefix", label: "Prefix" },
	{ value: "exact", label: "Exact" },
	{ value: "regex", label: "Regex" },
];

export const pathMatchLabels: Record<PathMatch, string> = {
	prefix: "Prefix",
	exact: "Exact",
	regex: "Regex",
};

export const handlerLabels: Record<RuleHandlerType, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
	error: "Error",
	none: "None",
};

export const handlerOptions: { value: HandlerType; label: string }[] = [
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
	{ value: "error", label: "Error" },
];

export const handlerOptionsWithNone: { value: RuleHandlerType; label: string }[] = [
	{ value: "none", label: "None" },
	...handlerOptions,
];
