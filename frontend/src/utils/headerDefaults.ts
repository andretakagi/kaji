import type {
	DomainRequestHeaders,
	HeaderDownConfig,
	HeaderEntry,
	HeaderUpConfig,
	ResponseHeaders,
} from "../types/api";

export const defaultResponseBuiltins: HeaderEntry[] = [
	{
		key: "Strict-Transport-Security",
		value: "max-age=31536000; includeSubDomains; preload",
		operation: "set",
		enabled: false,
	},
	{ key: "X-Content-Type-Options", value: "nosniff", operation: "set", enabled: false },
	{ key: "X-Frame-Options", value: "DENY", operation: "set", enabled: false },
	{
		key: "Referrer-Policy",
		value: "strict-origin-when-cross-origin",
		operation: "set",
		enabled: false,
	},
	{
		key: "Permissions-Policy",
		value:
			"accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
		operation: "set",
		enabled: false,
	},
	{ key: "Cache-Control", value: "no-store", operation: "set", enabled: false },
	{ key: "X-Robots-Tag", value: "noindex, nofollow", operation: "set", enabled: false },
	{ key: "Access-Control-Allow-Origin", value: "*", operation: "set", enabled: false },
	{
		key: "Access-Control-Allow-Methods",
		value: "GET, POST, PUT, DELETE, OPTIONS",
		operation: "set",
		enabled: false,
	},
	{
		key: "Access-Control-Allow-Headers",
		value: "Content-Type, Authorization",
		operation: "set",
		enabled: false,
	},
	{ key: "Access-Control-Allow-Credentials", value: "true", operation: "set", enabled: false },
	{ key: "Content-Security-Policy", value: "", operation: "set", enabled: false },
];

export const defaultDomainRequestBuiltins: HeaderEntry[] = [
	{
		key: "X-Forwarded-For",
		value: "{http.request.remote.host}",
		operation: "set",
		enabled: false,
	},
	{ key: "X-Real-IP", value: "{http.request.remote.host}", operation: "set", enabled: false },
	{ key: "X-Forwarded-Proto", value: "{http.request.scheme}", operation: "set", enabled: false },
	{ key: "X-Forwarded-Host", value: "{http.request.host}", operation: "set", enabled: false },
	{ key: "X-Request-ID", value: "{http.request.uuid}", operation: "set", enabled: false },
];

export const defaultHeaderUpBuiltins: HeaderEntry[] = [
	{ key: "Host", value: "", operation: "set", enabled: false },
	{ key: "Authorization", value: "", operation: "set", enabled: false },
];

export const defaultHeaderDownBuiltins: HeaderEntry[] = [
	{ key: "Server", value: "", operation: "delete", enabled: false },
	{ key: "X-Powered-By", value: "", operation: "delete", enabled: false },
];

export const builtinResponseKeys = new Set(defaultResponseBuiltins.map((e) => e.key));
export const builtinDomainRequestKeys = new Set(defaultDomainRequestBuiltins.map((e) => e.key));
export const builtinHeaderUpKeys = new Set(defaultHeaderUpBuiltins.map((e) => e.key));
export const builtinHeaderDownKeys = new Set(defaultHeaderDownBuiltins.map((e) => e.key));

const securityKeys = new Set([
	"Strict-Transport-Security",
	"X-Content-Type-Options",
	"X-Frame-Options",
	"Referrer-Policy",
	"Permissions-Policy",
]);

const corsKeys = new Set([
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Credentials",
]);

export function expandBasicToAdvanced(toggles: ResponseHeaders): HeaderEntry[] {
	return defaultResponseBuiltins.map((entry) => {
		let enabled = false;
		let value = entry.value;
		if (securityKeys.has(entry.key)) enabled = toggles.security;
		else if (corsKeys.has(entry.key)) {
			enabled = toggles.cors;
			if (entry.key === "Access-Control-Allow-Origin" && toggles.cors_origins.length > 0) {
				value = toggles.cors_origins.join(", ");
			}
		} else if (entry.key === "Cache-Control") enabled = toggles.cache_control;
		else if (entry.key === "X-Robots-Tag") enabled = toggles.x_robots_tag;
		return { ...entry, value, operation: "set" as const, enabled };
	});
}

export function expandBasicDomainRequestToAdvanced(cfg: DomainRequestHeaders): HeaderEntry[] {
	return defaultDomainRequestBuiltins.map((entry) => {
		let enabled = false;
		if (entry.key === "X-Forwarded-For") enabled = cfg.x_forwarded_for;
		else if (entry.key === "X-Real-IP") enabled = cfg.x_real_ip;
		else if (entry.key === "X-Forwarded-Proto") enabled = cfg.x_forwarded_proto;
		else if (entry.key === "X-Forwarded-Host") enabled = cfg.x_forwarded_host;
		else if (entry.key === "X-Request-ID") enabled = cfg.x_request_id;
		return { ...entry, enabled };
	});
}

export function expandBasicHeaderUpToAdvanced(cfg: HeaderUpConfig): HeaderEntry[] {
	return defaultHeaderUpBuiltins.map((entry) => {
		if (entry.key === "Host") {
			return { ...entry, enabled: cfg.host_override, value: cfg.host_value || entry.value };
		}
		if (entry.key === "Authorization") {
			return { ...entry, enabled: cfg.authorization, value: cfg.auth_value || entry.value };
		}
		return { ...entry };
	});
}

export function expandBasicHeaderDownToAdvanced(cfg: HeaderDownConfig): HeaderEntry[] {
	return defaultHeaderDownBuiltins.map((entry) => {
		let enabled = false;
		if (entry.key === "Server") enabled = cfg.strip_server;
		else if (entry.key === "X-Powered-By") enabled = cfg.strip_powered_by;
		return { ...entry, enabled };
	});
}
