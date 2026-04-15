import type { DisabledRoute, ParsedRoute, RouteToggles } from "../types/api";
import type { CaddyHandler, CaddyRoute, ReverseProxyHandler } from "../types/caddy";

export const defaultToggles: RouteToggles = {
	enabled: true,
	force_https: true,
	compression: false,
	security_headers: false,
	cors: { enabled: false, allowed_origins: [] },
	tls_skip_verify: false,
	basic_auth: { enabled: false, username: "", password_hash: "", password: "" },
	access_log: "",
	websocket_passthrough: false,
	load_balancing: { enabled: false, strategy: "round_robin", upstreams: [] },
	ip_filtering: { enabled: false, list_id: "", type: "" },
};

// Caddyfile-adapted routes wrap all handlers in a top-level subroute.
// This flattens that wrapper so parseRoute sees handlers the same way
// regardless of whether the route came from the admin API or a Caddyfile.
function flattenHandlers(topLevel: CaddyHandler[]): {
	handlers: CaddyHandler[];
	forceHTTPS: boolean;
} {
	const handlers: CaddyHandler[] = [];
	let forceHTTPS = false;

	for (const h of topLevel) {
		if (h.handler !== "subroute" || !h.routes) {
			handlers.push(h);
			continue;
		}

		// Kaji's ForceHTTPS subroute has a nested route with a protocol:"http" match
		const isForceHTTPS = h.routes.some((r) =>
			r.match?.some((m) => (m as Record<string, unknown>).protocol === "http"),
		);
		if (isForceHTTPS) {
			forceHTTPS = true;
			// Caddyfile-adapted routes may wrap all handlers (including
			// reverse_proxy) in the same subroute. Extract handlers from
			// nested routes that aren't the HTTP redirect.
			for (const nested of h.routes) {
				const isRedirect = nested.match?.some(
					(m) => (m as Record<string, unknown>).protocol === "http",
				);
				if (!isRedirect) {
					for (const nh of nested.handle ?? []) {
						handlers.push(nh);
					}
				}
			}
			continue;
		}

		// Caddyfile wrapper subroute - extract handlers from nested routes
		for (const nested of h.routes) {
			for (const nh of nested.handle ?? []) {
				handlers.push(nh);
			}
		}
	}

	return { handlers, forceHTTPS };
}

export function parseRoute(
	route: CaddyRoute,
	server: string,
	domainSinks?: Map<string, string>,
): ParsedRoute {
	const domain = route.match?.[0]?.host?.[0] ?? "";
	const { handlers, forceHTTPS } = flattenHandlers(route.handle ?? []);
	const rpHandler = handlers.find((h): h is ReverseProxyHandler => h.handler === "reverse_proxy");
	const upstream = rpHandler?.upstreams?.[0]?.dial ?? "";

	const toggles: RouteToggles = { ...defaultToggles, enabled: false, force_https: forceHTTPS };
	for (const h of handlers) {
		switch (h.handler) {
			case "subroute":
				toggles.force_https = true;
				break;
			case "encode":
				toggles.compression = true;
				break;
			case "headers": {
				const sets = h.response?.set;
				if (sets && "X-Content-Type-Options" in sets) {
					toggles.security_headers = true;
				}
				if (sets && "Access-Control-Allow-Origin" in sets) {
					toggles.cors = {
						enabled: true,
						allowed_origins:
							sets["Access-Control-Allow-Origin"]?.[0] === "*"
								? []
								: (sets["Access-Control-Allow-Origin"] ?? []),
					};
				}
				break;
			}
			case "authentication": {
				const acct = h.providers?.http_basic?.accounts?.[0];
				toggles.basic_auth = {
					enabled: true,
					username: acct?.username ?? "",
					password_hash: "",
					password: "",
				};
				break;
			}
		}
	}

	if (rpHandler?.transport?.tls?.insecure_skip_verify) {
		toggles.tls_skip_verify = true;
	}
	if (rpHandler?.flush_interval === -1) {
		toggles.websocket_passthrough = true;
	}
	if (rpHandler?.load_balancing?.selection_policy?.policy) {
		const additionalUpstreams = (rpHandler.upstreams ?? []).slice(1).map((u) => u.dial);
		toggles.load_balancing = {
			enabled: true,
			strategy: rpHandler.load_balancing.selection_policy.policy,
			upstreams: additionalUpstreams,
		};
	}
	const sinkName = domainSinks?.get(domain);
	if (sinkName) {
		toggles.access_log = sinkName;
	}
	toggles.enabled = true;

	return {
		id: route["@id"] ?? "",
		domain,
		upstream,
		disabled: false,
		server,
		toggles,
	};
}

export function parseDisabledRoute(dr: DisabledRoute): ParsedRoute {
	const parsed = parseRoute(dr.route, dr.server);
	parsed.disabled = true;
	parsed.toggles = { ...parsed.toggles, enabled: false };
	if (!parsed.id) parsed.id = dr.id;
	return parsed;
}
