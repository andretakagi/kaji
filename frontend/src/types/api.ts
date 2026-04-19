import type { CaddyConfig } from "./caddy";

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
	caddyfile_json?: CaddyConfig;
	dns_challenge_token?: string;
	auto_snapshot_enabled?: boolean;
	auto_snapshot_limit?: number;
	backup_data?: Record<string, unknown>;
}

export interface SetupResponse {
	status: string;
	warnings?: string[];
	dns_error?: string;
}

export interface ReviewRoute {
	domain: string;
	upstream: string;
	enabled: boolean;
}

export interface ReviewLogging {
	log_file: string;
	log_dir: string;
	loki_enabled: boolean;
	loki_endpoint: string;
}

export interface ReviewIPList {
	name: string;
	type: string;
	entry_count: number;
}

export interface ReviewSnapshot {
	name: string;
	type: string;
	created_at: string;
}

export interface ImportReview {
	routes: ReviewRoute[];
	logging?: ReviewLogging;
	ip_lists?: ReviewIPList[];
	snapshots?: ReviewSnapshot[];
}

export interface AdaptCaddyfileResponse {
	acme_email: string;
	admin_listen?: string;
	global_toggles: GlobalToggles;
	route_count: number;
	adapted_config: CaddyConfig;
	routes?: ReviewRoute[];
}

export interface LoginRequest {
	password: string;
}

export interface AuthStatus {
	authenticated: boolean;
	auth_enabled: boolean;
	has_password: boolean;
}

export interface HeaderEntry {
	key: string;
	value: string;
	enabled: boolean;
}

export interface ResponseHeaders {
	enabled: boolean;
	security: boolean;
	cors: boolean;
	cors_origins: string[];
	cache_control: boolean;
	x_robots_tag: boolean;
	builtin: HeaderEntry[];
	custom: HeaderEntry[];
}

export interface RequestHeaders {
	enabled: boolean;
	host_override: boolean;
	host_value: string;
	authorization: boolean;
	auth_value: string;
	builtin: HeaderEntry[];
	custom: HeaderEntry[];
}

export interface HeadersConfig {
	response: ResponseHeaders;
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

export interface ImportResponse {
	status: string;
	route_count?: number;
	snapshot_count?: number;
	migrated_from?: string;
	migration_log?: string[];
	warnings?: string[];
}

export interface SetupImportFullResponse {
	status: string;
	backup_data: Record<string, unknown>;
	acme_email?: string;
	global_toggles?: GlobalToggles;
	route_count?: number;
	summary: {
		auth_enabled: boolean;
		has_api_key: boolean;
		caddy_admin_url: string;
		loki_enabled: boolean;
		ip_lists: number;
		disabled_routes: number;
		snapshot_count: number;
	};
	migrated_from?: string;
	migration_log?: string[];
	review?: ImportReview;
}
