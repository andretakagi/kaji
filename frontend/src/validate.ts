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
	DNSProviderSettings,
	GlobalToggles,
	ImportResponse,
	IPList,
	IPListUsage,
	SetupImportFullResponse,
	SetupResponse,
	SetupStatus,
	UpstreamStatus,
} from "./types/api";
import type { CaddyConfig } from "./types/caddy";
import type { CertInfo } from "./types/certs";
import type { Domain, Path, Subdomain } from "./types/domain";
import type {
	CaddyLogEntry,
	CaddyLoggingConfig,
	LokiConfig,
	LokiStatus,
	LokiTestResult,
} from "./types/logs";
import type { Snapshot, SnapshotIndex } from "./types/snapshots";

export function validateAuthStatus(data: unknown): AuthStatus {
	return assertValid("AuthStatus", data, (d) =>
		hasFields(d, { authenticated: is.boolean, auth_enabled: is.boolean, has_password: is.boolean }),
	);
}

export function validateSetupStatus(data: unknown): SetupStatus {
	return assertValid("SetupStatus", data, (d) =>
		hasFields(d, { is_first_run: is.boolean, caddy_running: is.boolean }),
	);
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

function isUpstreamStatus(d: unknown): boolean {
	return hasFields(d, {
		address: is.string,
	});
}

export function validateUpstreams(data: unknown): UpstreamStatus[] {
	return assertValid("UpstreamStatus[]", data, (d) => is.array(d) && d.every(isUpstreamStatus));
}

export function validateCaddyConfig(data: unknown): CaddyConfig {
	return assertValid("CaddyConfig", data, (d) => is.object(d));
}

export function validateGlobalToggles(data: unknown): GlobalToggles {
	return assertValid("GlobalToggles", data, (d) =>
		hasFields(d, {
			auto_https: is.string,
			prometheus_metrics: is.boolean,
			per_host_metrics: is.boolean,
		}),
	);
}

export function validateACMEEmail(data: unknown): { email: string } {
	return assertValid("ACMEEmail", data, (d) => hasFields(d, { email: is.string }));
}

export function validateDNSProvider(data: unknown): DNSProviderSettings {
	return assertValid("DNSProvider", data, (d) => hasFields(d, { enabled: is.boolean }));
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

export function validateExportCaddyfile(data: unknown): string {
	const result = assertValid<{ content: string }>("ExportCaddyfile", data, (d) =>
		hasFields(d, { content: is.string }),
	);
	return result.content;
}

export function validateAdaptCaddyfileResponse(data: unknown): AdaptCaddyfileResponse {
	return assertValid("AdaptCaddyfileResponse", data, (d) =>
		hasFields(d, {
			acme_email: is.string,
			global_toggles: is.object,
			domain_count: is.number,
		}),
	);
}

export function validateAccessDomains(data: unknown): Record<string, Record<string, string>> {
	return assertValid("AccessDomains", data, (d) => {
		if (!is.object(d)) return false;
		for (const v of Object.values(d)) {
			if (!is.object(v)) return false;
			for (const sink of Object.values(v)) {
				if (!is.string(sink)) return false;
			}
		}
		return true;
	});
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

export function validateSnapshot(data: unknown): Snapshot {
	return assertValid("Snapshot", data, (d) =>
		hasFields(d, { id: is.string, name: is.string, type: is.string, created_at: is.string }),
	);
}

export function validateSnapshotIndex(data: unknown): SnapshotIndex {
	return assertValid("SnapshotIndex", data, (d) =>
		hasFields(d, {
			current_id: is.string,
			auto_snapshot_enabled: is.boolean,
			auto_snapshot_limit: is.number,
			snapshots: is.array,
		}),
	);
}

function isIPList(d: unknown): boolean {
	return hasFields(d, {
		id: is.string,
		name: is.string,
		type: is.string,
	});
}

export function validateIPLists(data: unknown): IPList[] {
	return assertValid("IPList[]", data, (d) => is.array(d) && d.every(isIPList));
}

export function validateIPListSingle(data: unknown): IPList {
	return assertValid("IPList", data, isIPList);
}

export function validateIPListUsage(data: unknown): IPListUsage {
	return assertValid("IPListUsage", data, (d) =>
		hasFields(d, { domains: is.array, composite_lists: is.array }),
	);
}

export function validateDomainIPListBindings(data: unknown): Record<string, string> {
	return assertValid("DomainIPListBindings", data, (d) => {
		if (!is.object(d)) return false;
		for (const v of Object.values(d)) {
			if (!is.string(v)) return false;
		}
		return true;
	});
}

function isCertInfo(d: unknown): boolean {
	return hasFields(d, {
		domain: is.string,
		sans: is.array,
		issuer: is.string,
		not_before: is.string,
		not_after: is.string,
		days_left: is.number,
		status: is.string,
		managed: is.boolean,
		issuer_key: is.string,
		fingerprint: is.string,
	});
}

export function validateCertificates(data: unknown): CertInfo[] {
	return assertValid("CertInfo[]", data, (d) => is.array(d) && d.every(isCertInfo));
}

export function validateCaddyDataDir(data: unknown): {
	caddy_data_dir: string;
	is_override: string;
} {
	return assertValid("CaddyDataDir", data, (d) =>
		hasFields(d, { caddy_data_dir: is.string, is_override: is.string }),
	);
}

export function validateLokiStatus(data: unknown): LokiStatus {
	return assertValid("LokiStatus", data, (d) =>
		hasFields(d, { running: is.boolean, sinks: is.object }),
	);
}

export function validateLokiConfig(data: unknown): LokiConfig {
	return assertValid("LokiConfig", data, (d) =>
		hasFields(d, {
			enabled: is.boolean,
			endpoint: is.string,
			batch_size: is.number,
			flush_interval_seconds: is.number,
		}),
	);
}

export function validateImportResponse(data: unknown): ImportResponse {
	return assertValid("ImportResponse", data, (d) => hasFields(d, { status: is.string }));
}

export function validateSetupImportFullResponse(data: unknown): SetupImportFullResponse {
	return assertValid("SetupImportFullResponse", data, (d) =>
		hasFields(d, { status: is.string, backup_data: is.object, summary: is.object }),
	);
}

export function validateLokiTestResult(data: unknown): LokiTestResult {
	return assertValid("LokiTestResult", data, (d) =>
		hasFields(d, { success: is.boolean, message: is.string }),
	);
}

function isRule(d: unknown): boolean {
	return hasFields(d, {
		handler_type: is.string,
		handler_config: is.object,
		enabled: is.boolean,
	});
}

function isPath(d: unknown): boolean {
	return hasFields(d, {
		id: is.string,
		enabled: is.boolean,
		path_match: is.string,
		match_value: is.string,
		rule: isRule,
	});
}

function isSubdomain(d: unknown): boolean {
	return hasFields(d, {
		id: is.string,
		name: is.string,
		enabled: is.boolean,
		toggles: is.object,
		rule: isRule,
		paths: (v) => is.array(v) && v.every(isPath),
	});
}

function isDomain(d: unknown): boolean {
	return hasFields(d, {
		id: is.string,
		name: is.string,
		enabled: is.boolean,
		toggles: is.object,
		rule: isRule,
		subdomains: (v) => is.array(v) && v.every(isSubdomain),
		paths: (v) => is.array(v) && v.every(isPath),
	});
}

export function validateDomain(data: unknown): Domain {
	return assertValid("Domain", data, isDomain);
}

export function validateDomainArray(data: unknown): Domain[] {
	return assertValid("Domain[]", data, (d) => is.array(d) && d.every(isDomain));
}

export function validatePath(data: unknown): Path {
	return assertValid("Path", data, isPath);
}

export function validateSubdomain(data: unknown): Subdomain {
	return assertValid("Subdomain", data, isSubdomain);
}
