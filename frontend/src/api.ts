import type {
	AdaptCaddyfileResponse,
	AuthStatus,
	CaddyfileResponse,
	CaddyStatus,
	ChangePasswordRequest,
	CreateRouteRequest,
	DisabledRoute,
	DNSProviderSettings,
	GlobalToggles,
	IPList,
	IPListUsage,
	LoginRequest,
	SetupRequest,
	SetupResponse,
	SetupStatus,
	UpdateRouteRequest,
	UpstreamStatus,
} from "./types/api";
import type { CaddyConfig } from "./types/caddy";
import type {
	CaddyLoggingConfig,
	CaddyLogSink,
	LogQueryParams,
	LogQueryResponse,
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
	validateCaddyfileResponse,
	validateCaddyStatus,
	validateCreateRouteResponse,
	validateDisabledRoutes,
	validateDNSProvider,
	validateGenerateAPIKey,
	validateGlobalToggles,
	validateIPListSingle,
	validateIPLists,
	validateIPListUsage,
	validateLoggingConfig,
	validateLogs,
	validateRouteIPListBindings,
	validateSetupResponse,
	validateSetupStatus,
	validateSnapshot,
	validateSnapshotIndex,
	validateStatusResponse,
	validateUpstreams,
} from "./validate";

export const POLL_INTERVAL = 5000;

const REQUEST_TIMEOUT_MS = 15000;

async function request<T>(
	path: string,
	options?: RequestInit,
	validate?: (data: unknown) => T,
): Promise<T> {
	const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
	const signal = options?.signal ? AbortSignal.any([timeoutSignal, options.signal]) : timeoutSignal;
	let res: Response;
	try {
		res = await fetch(path, {
			...options,
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
		let message = text || res.statusText;
		try {
			const json = JSON.parse(text);
			if (json.error) message = json.error;
		} catch {
			// not JSON, use raw text
		}
		throw new Error(message);
	}
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
		"/api/setup/adapt-caddyfile",
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

export function createRoute(req: CreateRouteRequest): Promise<{ status: string; "@id": string }> {
	return request("/api/routes", { method: "POST", ...jsonBody(req) }, validateCreateRouteResponse);
}

export function deleteRoute(id: string): Promise<{ status: string }> {
	return request(
		`/api/routes/${encodeURIComponent(id)}`,
		{ method: "DELETE" },
		validateStatusResponse,
	);
}

export function updateRoute(req: UpdateRouteRequest): Promise<{ status: string }> {
	const { id, ...body } = req;
	return request(
		`/api/routes/${encodeURIComponent(id)}`,
		{ method: "PUT", ...jsonBody(body) },
		validateStatusResponse,
	);
}

export function disableRoute(id: string): Promise<{ status: string }> {
	return request(
		"/api/routes/disable",
		{ method: "POST", ...jsonBody({ "@id": id }) },
		validateStatusResponse,
	);
}

export function enableRoute(id: string): Promise<{ status: string }> {
	return request(
		"/api/routes/enable",
		{ method: "POST", ...jsonBody({ "@id": id }) },
		validateStatusResponse,
	);
}

export function fetchDisabledRoutes(): Promise<DisabledRoute[]> {
	return request("/api/routes/disabled", undefined, validateDisabledRoutes);
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

export async function exportCaddyfile(): Promise<string> {
	const res = await fetch("/api/caddyfile", { credentials: "include" });
	if (!res.ok) {
		const body = await res.text();
		throw new Error(body || `Export failed (${res.status})`);
	}
	const data = await res.json();
	return data.content;
}

const LOG_DIR = "/var/log/caddy/";

export async function fetchLogConfig(): Promise<CaddyLoggingConfig> {
	const config = await request("/api/logs/config", undefined, validateLoggingConfig);
	for (const sink of Object.values(config.logs ?? {})) {
		if (sink.writer && sink.writer.output === "file") {
			if (sink.writer.filename?.startsWith(LOG_DIR)) {
				sink.writer.filename = sink.writer.filename.slice(LOG_DIR.length);
			}
		}
	}
	return config;
}

export function updateLogConfig(config: CaddyLoggingConfig): Promise<{ status: string }> {
	const logs: Record<string, CaddyLogSink> = {};
	for (const [name, sink] of Object.entries(config.logs ?? {})) {
		const writer = sink.writer ? { ...sink.writer } : undefined;
		if (writer) {
			if (writer.output === "file") {
				if (writer.filename && !writer.filename.startsWith("/")) {
					writer.filename = LOG_DIR + writer.filename;
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
	const sanitized: CaddyLoggingConfig = { ...config, logs };
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

export function restoreSnapshot(id: string): Promise<{ status: string }> {
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
