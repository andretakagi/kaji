import type {
	AdaptCaddyfileResponse,
	AuthStatus,
	CaddyfileResponse,
	CaddyStatus,
	ChangePasswordRequest,
	DNSProviderSettings,
	GlobalToggles,
	ImportResponse,
	IPList,
	IPListUsage,
	LoginRequest,
	SetupImportFullResponse,
	SetupRequest,
	SetupResponse,
	SetupStatus,
	UpstreamStatus,
} from "./types/api";
import type { CaddyConfig } from "./types/caddy";
import type { CertInfo } from "./types/certs";
import type {
	CreateDomainRequest,
	CreateRuleRequest,
	Domain,
	Rule,
	UpdateDomainRequest,
	UpdateRuleRequest,
} from "./types/domain";
import type {
	CaddyLoggingConfig,
	CaddyLogSink,
	LogQueryParams,
	LogQueryResponse,
	LokiConfig,
	LokiStatus,
	LokiTestResult,
} from "./types/logs";
import type { Snapshot, SnapshotIndex, SnapshotSettings } from "./types/snapshots";
import {
	validateACMEEmail,
	validateAccessDomains,
	validateAdaptCaddyfileResponse,
	validateAdvancedSettings,
	validateAdvancedUpdateResponse,
	validateAPIKeyStatus,
	validateAuthStatus,
	validateCaddyConfig,
	validateCaddyDataDir,
	validateCaddyfileResponse,
	validateCaddyStatus,
	validateCertificates,
	validateDNSProvider,
	validateExportCaddyfile,
	validateGenerateAPIKey,
	validateGlobalToggles,
	validateImportResponse,
	validateIPListSingle,
	validateIPLists,
	validateIPListUsage,
	validateLoggingConfig,
	validateLogs,
	validateLokiConfig,
	validateLokiStatus,
	validateLokiTestResult,
	validateRouteIPListBindings,
	validateSetupImportFullResponse,
	validateSetupResponse,
	validateSetupStatus,
	validateSnapshot,
	validateSnapshotIndex,
	validateStatusResponse,
	validateUpstreams,
} from "./validate";

export const POLL_INTERVAL = 5000;

const REQUEST_TIMEOUT_MS = 15000;

async function sendRequest(
	path: string,
	options?: RequestInit & {
		parseError?: (status: number, text: string, statusText: string) => Error;
	},
): Promise<Response> {
	const { parseError, ...fetchInit } = options ?? {};
	const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
	const signal = fetchInit.signal
		? AbortSignal.any([timeoutSignal, fetchInit.signal])
		: timeoutSignal;
	let res: Response;
	try {
		res = await fetch(path, {
			...fetchInit,
			credentials: "include",
			signal,
		});
	} catch (err) {
		if (err instanceof DOMException && err.name === "TimeoutError") {
			throw new Error("Request timed out - the server may be busy or unreachable");
		}
		if (err instanceof DOMException && err.name === "AbortError") {
			throw err;
		}
		throw new Error("Could not reach the server - check that Kaji is running");
	}
	if (!res.ok) {
		const text = await res.text();
		if (parseError) {
			throw parseError(res.status, text, res.statusText);
		}
		let message = text || res.statusText;
		try {
			const json = JSON.parse(text);
			if (json.error) message = json.error;
		} catch {
			// not JSON, use raw text
		}
		throw new Error(message);
	}
	return res;
}

async function request<T>(
	path: string,
	options?: RequestInit,
	validate?: (data: unknown) => T,
): Promise<T> {
	const res = await sendRequest(path, options);
	let data: unknown;
	try {
		data = await res.json();
	} catch {
		throw new Error(
			`unexpected non-JSON response from ${options?.method ?? "GET"} ${path} (${res.status})`,
		);
	}
	return validate ? validate(data) : (data as T);
}

const jsonBody = (body: unknown): RequestInit => ({
	headers: { "Content-Type": "application/json" },
	body: JSON.stringify(body),
});

export function fetchAuthStatus(): Promise<AuthStatus> {
	return request("/api/auth/status", undefined, validateAuthStatus);
}

export function submitLogin(req: LoginRequest): Promise<{ status: string }> {
	return request("/api/auth/login", { method: "POST", ...jsonBody(req) }, validateStatusResponse);
}

export function fetchSetupStatus(): Promise<SetupStatus> {
	return request("/api/setup/status", undefined, validateSetupStatus);
}

export function submitSetup(req: SetupRequest): Promise<SetupResponse> {
	return request("/api/setup", { method: "POST", ...jsonBody(req) }, validateSetupResponse);
}

export function adaptCaddyfile(caddyfile: string): Promise<AdaptCaddyfileResponse> {
	return request(
		"/api/setup/import/caddyfile",
		{ method: "POST", ...jsonBody({ caddyfile }) },
		validateAdaptCaddyfileResponse,
	);
}

export function fetchDefaultCaddyfile(): Promise<CaddyfileResponse> {
	return request("/api/setup/default-caddyfile", undefined, validateCaddyfileResponse);
}

export function fetchStatus(): Promise<CaddyStatus> {
	return request("/api/caddy/status", undefined, validateCaddyStatus);
}

export function startCaddy(): Promise<{ status: string }> {
	return request("/api/caddy/start", { method: "POST" }, validateStatusResponse);
}

export function stopCaddy(): Promise<{ status: string }> {
	return request("/api/caddy/stop", { method: "POST" }, validateStatusResponse);
}

export function restartCaddy(): Promise<{ status: string }> {
	return request("/api/caddy/restart", { method: "POST" }, validateStatusResponse);
}

export function fetchConfig(): Promise<CaddyConfig> {
	return request("/api/caddy/config", undefined, validateCaddyConfig);
}

export function fetchUpstreams(): Promise<UpstreamStatus[]> {
	return request("/api/caddy/upstreams", undefined, validateUpstreams);
}

export function fetchGlobalToggles(): Promise<GlobalToggles> {
	return request("/api/settings/global-toggles", undefined, validateGlobalToggles);
}

export function updateGlobalToggles(toggles: GlobalToggles): Promise<{ status: string }> {
	return request(
		"/api/settings/global-toggles",
		{ method: "PUT", ...jsonBody(toggles) },
		validateStatusResponse,
	);
}

export function fetchACMEEmail(): Promise<{ email: string }> {
	return request("/api/settings/acme-email", undefined, validateACMEEmail);
}

export function updateACMEEmail(email: string): Promise<{ status: string }> {
	return request(
		"/api/settings/acme-email",
		{ method: "PUT", ...jsonBody({ email }) },
		validateStatusResponse,
	);
}

export function fetchDNSProvider(): Promise<DNSProviderSettings> {
	return request("/api/settings/dns-provider", undefined, validateDNSProvider);
}

export function updateDNSProvider(settings: {
	enabled: boolean;
	api_token?: string;
}): Promise<{ status: string }> {
	return request(
		"/api/settings/dns-provider",
		{ method: "PUT", ...jsonBody(settings) },
		validateStatusResponse,
	);
}

export function updateAuthEnabled(
	enabled: boolean,
	password?: string,
): Promise<{ status: string }> {
	const body: Record<string, unknown> = { auth_enabled: enabled };
	if (password) body.password = password;
	return request(
		"/api/settings/auth",
		{ method: "PUT", ...jsonBody(body) },
		validateStatusResponse,
	);
}

export function changePassword(req: ChangePasswordRequest): Promise<{ status: string }> {
	return request("/api/auth/password", { method: "PUT", ...jsonBody(req) }, validateStatusResponse);
}

export function logout(): Promise<{ status: string }> {
	return request("/api/auth/logout", { method: "POST", ...jsonBody({}) }, validateStatusResponse);
}

export function fetchAPIKeyStatus(): Promise<{ has_api_key: boolean }> {
	return request("/api/settings/api-key", undefined, validateAPIKeyStatus);
}

export function generateAPIKey(): Promise<{ api_key: string }> {
	return request("/api/settings/api-key", { method: "POST" }, validateGenerateAPIKey);
}

export function revokeAPIKey(): Promise<{ status: string }> {
	return request("/api/settings/api-key", { method: "DELETE" }, validateStatusResponse);
}

export function fetchAdvancedSettings(): Promise<{
	caddy_admin_url: string;
}> {
	return request("/api/settings/advanced", undefined, validateAdvancedSettings);
}

export function updateAdvancedSettings(settings: {
	caddy_admin_url: string;
}): Promise<{ status: string }> {
	return request(
		"/api/settings/advanced",
		{ method: "PUT", ...jsonBody(settings) },
		validateAdvancedUpdateResponse,
	);
}

export function exportCaddyfile(): Promise<string> {
	return request("/api/export/caddyfile", undefined, validateExportCaddyfile);
}

export async function exportFull(): Promise<Blob> {
	const res = await sendRequest("/api/export/full");
	return res.blob();
}

export function importCaddyfile(caddyfile: string): Promise<ImportResponse> {
	return request(
		"/api/import/caddyfile",
		{ method: "POST", ...jsonBody({ caddyfile }) },
		validateImportResponse,
	);
}

export function importFull(file: File): Promise<ImportResponse> {
	return request(
		"/api/import/full",
		{ method: "POST", headers: { "Content-Type": "application/zip" }, body: file },
		validateImportResponse,
	);
}

export function setupImportFull(file: File): Promise<SetupImportFullResponse> {
	return request(
		"/api/setup/import/full",
		{ method: "POST", headers: { "Content-Type": "application/zip" }, body: file },
		validateSetupImportFullResponse,
	);
}

export async function fetchLogConfig(): Promise<CaddyLoggingConfig> {
	const config = await request("/api/logs/config", undefined, validateLoggingConfig);
	const logDir = config.log_dir ?? "/var/log/caddy/";
	for (const sink of Object.values(config.logs ?? {})) {
		if (sink.writer && sink.writer.output === "file") {
			if (sink.writer.filename?.startsWith(logDir)) {
				sink.writer.filename = sink.writer.filename.slice(logDir.length);
			}
		}
	}
	return config;
}

export function updateLogConfig(config: CaddyLoggingConfig): Promise<{ status: string }> {
	const logDir = config.log_dir ?? "/var/log/caddy/";
	const logs: Record<string, CaddyLogSink> = {};
	for (const [name, sink] of Object.entries(config.logs ?? {})) {
		const writer = sink.writer ? { ...sink.writer } : undefined;
		if (writer) {
			if (writer.output === "file") {
				if (writer.filename && !writer.filename.startsWith("/")) {
					writer.filename = logDir + writer.filename;
				}
				const hasRollSettings =
					(writer.roll_size_mb != null && writer.roll_size_mb > 0) ||
					(writer.roll_keep != null && writer.roll_keep > 0) ||
					(writer.roll_keep_for != null && writer.roll_keep_for > 0);
				if (!hasRollSettings) {
					delete writer.roll_size_mb;
					delete writer.roll_keep;
					delete writer.roll_keep_for;
				}
			}
		}
		logs[name] = { ...sink, writer };
	}
	const { log_dir: _, ...rest } = config;
	const sanitized: CaddyLoggingConfig = { ...rest, logs };
	return request(
		"/api/logs/config",
		{ method: "PUT", ...jsonBody(sanitized) },
		validateStatusResponse,
	);
}

export function fetchAccessDomains(): Promise<Record<string, Record<string, string>>> {
	return request("/api/logs/access-domains", undefined, validateAccessDomains);
}

export function fetchLogs(params: LogQueryParams): Promise<LogQueryResponse> {
	const qs = new URLSearchParams();
	if (params.limit != null) qs.set("limit", String(params.limit));
	if (params.offset != null) qs.set("offset", String(params.offset));
	if (params.level) qs.set("level", params.level);
	if (params.host) qs.set("host", params.host);
	if (params.status_min) qs.set("status_min", String(params.status_min));
	if (params.status_max) qs.set("status_max", String(params.status_max));
	if (params.since) qs.set("since", params.since);
	if (params.until) qs.set("until", params.until);
	const query = qs.toString();
	return request(`/api/logs${query ? `?${query}` : ""}`, undefined, validateLogs);
}

export function fetchSnapshots(): Promise<SnapshotIndex> {
	return request("/api/snapshots", undefined, validateSnapshotIndex);
}

export function createSnapshot(name: string, description: string): Promise<Snapshot> {
	return request(
		"/api/snapshots",
		{ method: "POST", ...jsonBody({ name, description }) },
		validateSnapshot,
	);
}

export function restoreSnapshot(
	id: string,
): Promise<{ status: string; legacy?: boolean; warnings?: string[]; migration_log?: string[] }> {
	return request(
		`/api/snapshots/${encodeURIComponent(id)}/restore`,
		{ method: "POST" },
		validateStatusResponse,
	);
}

export function updateSnapshot(
	id: string,
	name: string,
	description: string,
): Promise<{ status: string }> {
	return request(
		`/api/snapshots/${encodeURIComponent(id)}`,
		{ method: "PUT", ...jsonBody({ name, description }) },
		validateStatusResponse,
	);
}

export function deleteSnapshot(id: string): Promise<{ status: string }> {
	return request(
		`/api/snapshots/${encodeURIComponent(id)}`,
		{ method: "DELETE" },
		validateStatusResponse,
	);
}

export function updateSnapshotSettings(settings: SnapshotSettings): Promise<{ status: string }> {
	return request(
		"/api/snapshots/settings",
		{ method: "PUT", ...jsonBody(settings) },
		validateStatusResponse,
	);
}

export function fetchIPLists(): Promise<IPList[]> {
	return request("/api/ip-lists", undefined, validateIPLists);
}

export function createIPList(list: {
	name: string;
	description: string;
	type: "whitelist" | "blacklist";
	ips: string[];
	children: string[];
}): Promise<IPList> {
	return request("/api/ip-lists", { method: "POST", ...jsonBody(list) }, validateIPListSingle);
}

export function updateIPList(
	id: string,
	list: { name: string; description: string; ips: string[]; children: string[] },
): Promise<IPList> {
	return request(
		`/api/ip-lists/${encodeURIComponent(id)}`,
		{ method: "PUT", ...jsonBody(list) },
		validateIPListSingle,
	);
}

export function deleteIPList(id: string): Promise<{ status: string }> {
	return request(
		`/api/ip-lists/${encodeURIComponent(id)}`,
		{ method: "DELETE" },
		validateStatusResponse,
	);
}

export function fetchIPListUsage(id: string): Promise<IPListUsage> {
	return request(`/api/ip-lists/${encodeURIComponent(id)}/usage`, undefined, validateIPListUsage);
}

export function fetchRouteIPListBindings(): Promise<Record<string, string>> {
	return request("/api/ip-lists/bindings", undefined, validateRouteIPListBindings);
}

export function fetchCertificates(): Promise<CertInfo[]> {
	return request("/api/certificates", undefined, validateCertificates);
}

export function renewCertificate(issuerKey: string, domain: string): Promise<{ status: string }> {
	return request(
		"/api/certificates/renew",
		{ method: "POST", ...jsonBody({ issuer_key: issuerKey, domain }) },
		validateStatusResponse,
	);
}

export class CertInUseError extends Error {
	affectedRoutes: string[];
	constructor(message: string, affectedRoutes: string[]) {
		super(message);
		this.affectedRoutes = affectedRoutes;
	}
}

export async function deleteCertificate(
	issuerKey: string,
	domain: string,
	force = false,
): Promise<{ status: string }> {
	const query = force ? "?force=true" : "";
	const path = `/api/certificates/${encodeURIComponent(issuerKey)}/${encodeURIComponent(domain)}${query}`;
	const res = await sendRequest(path, {
		method: "DELETE",
		parseError: (_status, text, statusText) => {
			let message = text || statusText;
			let affectedRoutes: string[] | undefined;
			try {
				const json = JSON.parse(text);
				if (json.error) message = json.error;
				if (Array.isArray(json.affected_routes)) affectedRoutes = json.affected_routes;
			} catch {
				// not JSON
			}
			if (affectedRoutes && affectedRoutes.length > 0) {
				return new CertInUseError(message, affectedRoutes);
			}
			return new Error(message);
		},
	});
	return res.json();
}

export function downloadCertificate(issuerKey: string, domain: string): void {
	const url = `/api/certificates/${encodeURIComponent(issuerKey)}/${encodeURIComponent(domain)}/download`;
	const a = document.createElement("a");
	a.href = url;
	a.download = `${domain}.crt`;
	a.click();
}

export function fetchCaddyDataDir(): Promise<{
	caddy_data_dir: string;
	is_override: string;
}> {
	return request("/api/settings/caddy-data-dir", undefined, validateCaddyDataDir);
}

export function updateCaddyDataDir(dir: string): Promise<{ status: string }> {
	return request(
		"/api/settings/caddy-data-dir",
		{ method: "PUT", ...jsonBody({ caddy_data_dir: dir }) },
		validateStatusResponse,
	);
}

export function fetchLokiStatus(): Promise<LokiStatus> {
	return request("/api/loki/status", undefined, validateLokiStatus);
}

export function fetchLokiConfig(): Promise<LokiConfig> {
	return request("/api/loki/config", undefined, validateLokiConfig);
}

export function updateLokiConfig(config: LokiConfig): Promise<{ status: string }> {
	return request(
		"/api/loki/config",
		{ method: "PUT", ...jsonBody(config) },
		validateStatusResponse,
	);
}

export function testLokiConnection(): Promise<LokiTestResult> {
	return request("/api/loki/test", { method: "POST" }, validateLokiTestResult);
}

export function fetchDomains(): Promise<Domain[]> {
	return request("/api/domains", undefined, (d) => d as Domain[]);
}

export function fetchDomain(id: string): Promise<Domain> {
	return request(`/api/domains/${encodeURIComponent(id)}`, undefined, (d) => d as Domain);
}

export function createDomain(req: CreateDomainRequest): Promise<Domain> {
	return request("/api/domains", { method: "POST", ...jsonBody(req) }, (d) => d as Domain);
}

export function updateDomain(id: string, req: UpdateDomainRequest): Promise<Domain> {
	return request(
		`/api/domains/${encodeURIComponent(id)}`,
		{ method: "PUT", ...jsonBody(req) },
		(d) => d as Domain,
	);
}

export function deleteDomain(id: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(id)}`,
		{ method: "DELETE" },
		validateStatusResponse,
	);
}

export function enableDomain(id: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(id)}/enable`,
		{ method: "POST" },
		validateStatusResponse,
	);
}

export function disableDomain(id: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(id)}/disable`,
		{ method: "POST" },
		validateStatusResponse,
	);
}

export function createRule(domainId: string, req: CreateRuleRequest): Promise<Rule> {
	return request(
		`/api/domains/${encodeURIComponent(domainId)}/rules`,
		{ method: "POST", ...jsonBody(req) },
		(d) => d as Rule,
	);
}

export function updateRule(
	domainId: string,
	ruleId: string,
	req: UpdateRuleRequest,
): Promise<Rule> {
	return request(
		`/api/domains/${encodeURIComponent(domainId)}/rules/${encodeURIComponent(ruleId)}`,
		{ method: "PUT", ...jsonBody(req) },
		(d) => d as Rule,
	);
}

export function deleteRule(domainId: string, ruleId: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(domainId)}/rules/${encodeURIComponent(ruleId)}`,
		{ method: "DELETE" },
		validateStatusResponse,
	);
}

export function enableRule(domainId: string, ruleId: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(domainId)}/rules/${encodeURIComponent(ruleId)}/enable`,
		{ method: "POST" },
		validateStatusResponse,
	);
}

export function disableRule(domainId: string, ruleId: string): Promise<{ status: string }> {
	return request(
		`/api/domains/${encodeURIComponent(domainId)}/rules/${encodeURIComponent(ruleId)}/disable`,
		{ method: "POST" },
		validateStatusResponse,
	);
}
