type Checker = (value: unknown) => boolean;

const is = {
	string: (v: unknown): v is string => typeof v === "string",
	number: (v: unknown): v is number => typeof v === "number",
	boolean: (v: unknown): v is boolean => typeof v === "boolean",
	array: (v: unknown): v is unknown[] => Array.isArray(v),
	object: (v: unknown): v is Record<string, unknown> =>
		typeof v === "object" && v !== null && !Array.isArray(v),
};

function hasFields(obj: unknown, fields: Record<string, Checker>): obj is Record<string, unknown> {
	if (!is.object(obj)) return false;
	for (const [key, check] of Object.entries(fields)) {
		if (!(key in obj) || !check(obj[key])) return false;
	}
	return true;
}

class ValidationError extends Error {
	constructor(type: string, data: unknown) {
		const preview = typeof data === "object" ? JSON.stringify(data)?.slice(0, 200) : String(data);
		super(`Invalid ${type} response: ${preview}`);
		this.name = "ValidationError";
	}
}

function assertValid<T>(type: string, data: unknown, check: (d: unknown) => boolean): T {
	if (!check(data)) throw new ValidationError(type, data);
	return data as T;
}

// --- Response validators ---

import type {
	AdaptCaddyfileResponse,
	AuthStatus,
	CaddyfileResponse,
	CaddyStatus,
	DisabledRoute,
	GlobalToggles,
	SetupResponse,
	SetupStatus,
	UpstreamStatus,
} from "./types/api";
import type { CaddyConfig } from "./types/caddy";
import type { CaddyLogEntry, CaddyLoggingConfig } from "./types/logs";

export function validateAuthStatus(data: unknown): AuthStatus {
	return assertValid("AuthStatus", data, (d) =>
		hasFields(d, { authenticated: is.boolean, auth_enabled: is.boolean, has_password: is.boolean }),
	);
}

export function validateSetupStatus(data: unknown): SetupStatus {
	return assertValid("SetupStatus", data, (d) => hasFields(d, { is_first_run: is.boolean }));
}

export function validateSetupResponse(data: unknown): SetupResponse {
	return assertValid("SetupResponse", data, (d) => hasFields(d, { status: is.string }));
}

export function validateCaddyStatus(data: unknown): CaddyStatus {
	return assertValid("CaddyStatus", data, (d) => hasFields(d, { running: is.boolean }));
}

export function validateStatusResponse(data: unknown): { status: string } {
	return assertValid("StatusResponse", data, (d) => hasFields(d, { status: is.string }));
}

export function validateCreateRouteResponse(data: unknown): { status: string; "@id": string } {
	return assertValid("CreateRouteResponse", data, (d) =>
		hasFields(d, { status: is.string, "@id": is.string }),
	);
}

function isUpstreamStatus(d: unknown): boolean {
	return hasFields(d, {
		address: is.string,
		healthy: is.boolean,
		num_requests: is.number,
	});
}

export function validateUpstreams(data: unknown): UpstreamStatus[] {
	return assertValid("UpstreamStatus[]", data, (d) => is.array(d) && d.every(isUpstreamStatus));
}

function isDisabledRoute(d: unknown): boolean {
	return hasFields(d, {
		id: is.string,
		server: is.string,
		disabled_at: is.string,
	});
}

export function validateDisabledRoutes(data: unknown): DisabledRoute[] {
	return assertValid("DisabledRoute[]", data, (d) => is.array(d) && d.every(isDisabledRoute));
}

export function validateCaddyConfig(data: unknown): CaddyConfig {
	return assertValid("CaddyConfig", data, (d) => is.object(d));
}

export function validateGlobalToggles(data: unknown): GlobalToggles {
	return assertValid("GlobalToggles", data, (d) =>
		hasFields(d, {
			auto_https: is.string,
			http_to_https_redirect: is.boolean,
			prometheus_metrics: is.boolean,
			per_host_metrics: is.boolean,
			debug_logging: is.boolean,
		}),
	);
}

export function validateACMEEmail(data: unknown): { email: string } {
	return assertValid("ACMEEmail", data, (d) => hasFields(d, { email: is.string }));
}

export function validateAPIKeyStatus(data: unknown): { has_api_key: boolean } {
	return assertValid("APIKeyStatus", data, (d) => hasFields(d, { has_api_key: is.boolean }));
}

export function validateGenerateAPIKey(data: unknown): { api_key: string } {
	return assertValid("GenerateAPIKey", data, (d) => hasFields(d, { api_key: is.string }));
}

export function validateAdvancedSettings(data: unknown): {
	caddy_admin_url: string;
} {
	return assertValid("AdvancedSettings", data, (d) => hasFields(d, { caddy_admin_url: is.string }));
}

export function validateAdvancedUpdateResponse(data: unknown): {
	status: string;
} {
	return assertValid("AdvancedUpdateResponse", data, (d) => hasFields(d, { status: is.string }));
}

export function validateCaddyfileResponse(data: unknown): CaddyfileResponse {
	return assertValid("CaddyfileResponse", data, (d) =>
		hasFields(d, { content: is.string, path: is.string }),
	);
}

export function validateAdaptCaddyfileResponse(data: unknown): AdaptCaddyfileResponse {
	return assertValid("AdaptCaddyfileResponse", data, (d) =>
		hasFields(d, {
			acme_email: is.string,
			global_toggles: is.object,
			route_count: is.number,
		}),
	);
}

export function validateLoggingConfig(data: unknown): CaddyLoggingConfig {
	return assertValid("CaddyLoggingConfig", data, (d) => is.object(d));
}

function isLogEntry(d: unknown): boolean {
	return hasFields(d, {
		level: is.string,
		ts: is.number,
		logger: is.string,
		msg: is.string,
	});
}

export function validateLogs(data: unknown): { entries: CaddyLogEntry[]; has_more: boolean } {
	return assertValid("Logs", data, (d) =>
		hasFields(d, {
			entries: (v) => is.array(v) && v.every(isLogEntry),
			has_more: is.boolean,
		}),
	);
}
