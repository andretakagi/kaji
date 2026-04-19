import type { HeaderEntry, RequestHeaders, ResponseHeaders } from "../types/api";

export const defaultResponseBuiltins: HeaderEntry[] = [
	{
		key: "Strict-Transport-Security",
		value: "max-age=31536000; includeSubDomains; preload",
		enabled: false,
	},
	{ key: "X-Content-Type-Options", value: "nosniff", enabled: false },
	{ key: "X-Frame-Options", value: "DENY", enabled: false },
	{ key: "Referrer-Policy", value: "strict-origin-when-cross-origin", enabled: false },
	{
		key: "Permissions-Policy",
		value:
			"accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()",
		enabled: false,
	},
	{ key: "Cache-Control", value: "no-store", enabled: false },
	{ key: "X-Robots-Tag", value: "noindex, nofollow", enabled: false },
	{ key: "Access-Control-Allow-Origin", value: "*", enabled: false },
	{ key: "Access-Control-Allow-Methods", value: "GET, POST, PUT, DELETE, OPTIONS", enabled: false },
	{ key: "Access-Control-Allow-Headers", value: "Content-Type, Authorization", enabled: false },
	{ key: "Access-Control-Allow-Credentials", value: "true", enabled: false },
	{ key: "Content-Security-Policy", value: "", enabled: false },
];

export const defaultRequestBuiltins: HeaderEntry[] = [
	{ key: "Host", value: "", enabled: false },
	{ key: "Authorization", value: "", enabled: false },
	{ key: "X-Real-IP", value: "{http.request.remote.host}", enabled: false },
];

export const builtinResponseKeys = new Set(defaultResponseBuiltins.map((e) => e.key));
export const builtinRequestKeys = new Set(defaultRequestBuiltins.map((e) => e.key));

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
		return { ...entry, value, enabled };
	});
}

export function expandBasicRequestToAdvanced(toggles: RequestHeaders): HeaderEntry[] {
	return defaultRequestBuiltins.map((entry) => {
		if (entry.key === "Host") {
			return {
				...entry,
				enabled: toggles.host_override,
				value: toggles.host_value || entry.value,
			};
		}
		if (entry.key === "Authorization") {
			return {
				...entry,
				enabled: toggles.authorization,
				value: toggles.auth_value || entry.value,
			};
		}
		return { ...entry };
	});
}
