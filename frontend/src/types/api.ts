import type { CaddyRoute } from "./caddy";
import type { LokiConfig } from "./logs";

export interface CaddyStatus {
	running: boolean;
	uptime?: number;
}

export interface CaddyfileResponse {
	content: string;
	path: string;
}

export interface UpstreamStatus {
	address: string;
	healthy: boolean;
	num_requests: number;
}

export interface SetupStatus {
	is_first_run: boolean;
}

export interface SetupRequest {
	password?: string;
	caddy_admin_url?: string;
	acme_email?: string;
	global_toggles?: GlobalToggles;
	caddyfile_json?: unknown;
}

export interface SetupResponse {
	status: string;
	warnings?: string[];
}

export interface AdaptCaddyfileResponse {
	acme_email: string;
	global_toggles: GlobalToggles;
	route_count: number;
	adapted_config: unknown;
}

export interface LoginRequest {
	password: string;
}

export interface AuthStatus {
	authenticated: boolean;
	auth_enabled: boolean;
	has_password: boolean;
}

export interface AppConfig {
	auth_enabled: boolean;
	caddy_admin_url: string;
	caddy_config_path: string;
	log_file: string;
	loki: LokiConfig;
	disabled_routes: DisabledRoute[];
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
	access_log: boolean;
	websocket_passthrough: boolean;
	load_balancing: {
		enabled: boolean;
		strategy: "round_robin" | "first" | "least_conn" | "random" | "ip_hash";
		upstreams: string[];
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

export interface AuthToggleRequest {
	auth_enabled: boolean;
}

export interface GlobalToggles {
	auto_https: "on" | "off" | "disable_redirects";
	http_to_https_redirect: boolean;
	prometheus_metrics: boolean;
	per_host_metrics: boolean;
	debug_logging: boolean;
}

export const DEFAULT_GLOBAL_TOGGLES: GlobalToggles = {
	auto_https: "on",
	http_to_https_redirect: true,
	prometheus_metrics: false,
	per_host_metrics: false,
	debug_logging: false,
};
