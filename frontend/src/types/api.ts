import type { CaddyRoute } from "./caddy";

export interface CaddyStatus {
	running: boolean;
}

export interface CaddyfileResponse {
	content: string;
	path: string;
}

export interface UpstreamStatus {
	address: string;
}

export interface SetupStatus {
	is_first_run: boolean;
	caddy_running: boolean;
}

export interface SetupRequest {
	password?: string;
	caddy_admin_url?: string;
	acme_email?: string;
	global_toggles?: GlobalToggles;
	caddyfile_json?: Record<string, unknown>;
	dns_challenge_token?: string;
	auto_snapshot_enabled?: boolean;
	auto_snapshot_limit?: number;
}

export interface SetupResponse {
	status: string;
	warnings?: string[];
	dns_error?: string;
}

export interface AdaptCaddyfileResponse {
	acme_email: string;
	global_toggles: GlobalToggles;
	route_count: number;
	adapted_config: Record<string, unknown>;
}

export interface LoginRequest {
	password: string;
}

export interface AuthStatus {
	authenticated: boolean;
	auth_enabled: boolean;
	has_password: boolean;
}

export interface DisabledRoute {
	id: string;
	server: string;
	disabled_at: string;
	route: CaddyRoute;
}

export interface RouteToggles {
	enabled: boolean;
	force_https: boolean;
	compression: boolean;
	security_headers: boolean;
	cors: {
		enabled: boolean;
		allowed_origins: string[];
	};
	tls_skip_verify: boolean;
	basic_auth: {
		enabled: boolean;
		username: string;
		password_hash: string;
		password: string;
	};
	access_log: string;
	websocket_passthrough: boolean;
	load_balancing: {
		enabled: boolean;
		strategy: "round_robin" | "first" | "least_conn" | "random" | "ip_hash";
		upstreams: string[];
	};
	ip_filtering: {
		enabled: boolean;
		list_id: string;
		type: "whitelist" | "blacklist" | "";
	};
}

export interface CreateRouteRequest {
	server?: string;
	domain: string;
	upstream: string;
	toggles?: RouteToggles;
}

export interface UpdateRouteRequest {
	id: string;
	domain: string;
	upstream: string;
	toggles: RouteToggles;
}

export interface ParsedRoute {
	id: string;
	domain: string;
	upstream: string;
	disabled: boolean;
	server: string;
	toggles: RouteToggles;
}

export interface ChangePasswordRequest {
	new_password: string;
}

export interface GlobalToggles {
	auto_https: "on" | "off" | "disable_redirects";
	prometheus_metrics: boolean;
	per_host_metrics: boolean;
}

export const DEFAULT_GLOBAL_TOGGLES: GlobalToggles = {
	auto_https: "on",
	prometheus_metrics: false,
	per_host_metrics: false,
};

export interface DNSProviderSettings {
	enabled: boolean;
	provider?: string;
	api_token?: string;
}

export interface IPList {
	id: string;
	name: string;
	description: string;
	type: "whitelist" | "blacklist";
	ips: string[];
	children: string[];
	created_at: string;
	updated_at: string;
	resolved_count: number;
}

export interface IPListUsage {
	routes: { id: string; domain: string }[];
	composite_lists: { id: string; name: string }[];
}
