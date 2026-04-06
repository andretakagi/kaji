import { useCallback, useEffect, useRef, useState } from "react";
import {
	createRoute,
	deleteRoute,
	disableRoute,
	enableRoute,
	fetchConfig,
	fetchDisabledRoutes,
	POLL_INTERVAL,
	updateRoute,
} from "../api";
import type { DisabledRoute, ParsedRoute, RouteToggles } from "../types/api";
import type { CaddyConfig, CaddyHandler, CaddyRoute } from "../types/caddy";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validateDomain, validateUpstream } from "../utils/validate";
import RouteRow from "./RouteRow";
import ToggleGrid from "./ToggleGrid";

const defaultToggles: RouteToggles = {
	enabled: true,
	force_https: true,
	compression: false,
	security_headers: false,
	cors: { enabled: false, allowed_origins: [] },
	tls_skip_verify: false,
	basic_auth: { enabled: false, username: "", password_hash: "", password: "" },
	access_log: false,
	websocket_passthrough: false,
	load_balancing: { enabled: false, strategy: "round_robin", upstreams: [] },
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

function parseRoute(route: CaddyRoute, server: string, loggedDomains?: Set<string>): ParsedRoute {
	const domain = route.match?.[0]?.host?.[0] ?? "";
	const { handlers, forceHTTPS } = flattenHandlers(route.handle ?? []);
	const rpHandler = handlers.find((h: CaddyHandler) => h.handler === "reverse_proxy");
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
	if (loggedDomains?.has(domain)) {
		toggles.access_log = true;
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

function parseDisabledRoute(dr: DisabledRoute): ParsedRoute {
	const parsed = parseRoute(dr.route, dr.server);
	parsed.disabled = true;
	parsed.toggles = { ...parsed.toggles, enabled: false };
	if (!parsed.id) parsed.id = dr.id;
	return parsed;
}

export default function Routes({ caddyRunning }: { caddyRunning: boolean }) {
	const [routes, setRoutes] = useState<ParsedRoute[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [showForm, setShowForm] = useState(false);
	const [domain, setDomain] = useState("");
	const [upstream, setUpstream] = useState("");
	const [formToggles, setFormToggles] = useState<RouteToggles>({ ...defaultToggles });
	const [submitting, setSubmitting] = useState(false);
	const [deleting, setDeleting] = useState<string | null>(null);
	const deletingRef = useRef(deleting);
	deletingRef.current = deleting;

	const loadRoutes = useCallback(async () => {
		try {
			const parsed: ParsedRoute[] = [];

			if (caddyRunning) {
				const [config, disabled] = await Promise.all([fetchConfig(), fetchDisabledRoutes()]);
				const servers = (config as CaddyConfig).apps?.http?.servers;
				if (servers) {
					for (const [name, server] of Object.entries(servers)) {
						const loggedDomains = new Set<string>(Object.keys(server.logs?.logger_names ?? {}));
						for (const route of server.routes ?? []) {
							parsed.push(parseRoute(route, name, loggedDomains));
						}
					}
				}
				for (const dr of disabled) {
					parsed.push(parseDisabledRoute(dr));
				}
			} else {
				const disabled = await fetchDisabledRoutes();
				for (const dr of disabled) {
					parsed.push(parseDisabledRoute(dr));
				}
			}

			setRoutes((prev) => {
				if (prev.length === parsed.length && JSON.stringify(prev) === JSON.stringify(parsed)) {
					return prev;
				}
				return parsed;
			});
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load routes"));
		} finally {
			setLoading(false);
		}
	}, [caddyRunning]);

	useEffect(() => {
		loadRoutes();
		const id = setInterval(loadRoutes, POLL_INTERVAL);
		return () => clearInterval(id);
	}, [loadRoutes]);

	function updateFormToggle<K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) {
		setFormToggles((prev) => ({ ...prev, [key]: value }));
	}

	async function handleAdd(e: React.FormEvent) {
		e.preventDefault();
		setError("");

		const domainErr = validateDomain(domain);
		if (domainErr) {
			setError(domainErr);
			return;
		}
		const upstreamErr = validateUpstream(upstream);
		if (upstreamErr) {
			setError(upstreamErr);
			return;
		}

		if (formToggles.load_balancing.enabled) {
			if (formToggles.load_balancing.upstreams.length === 0) {
				setError("Load balancing requires at least one additional upstream");
				return;
			}
			for (const u of formToggles.load_balancing.upstreams) {
				const err = validateUpstream(u);
				if (err) {
					setError(`Additional upstream: ${err}`);
					return;
				}
			}
		}

		if (formToggles.basic_auth.enabled) {
			if (!formToggles.basic_auth.username.trim()) {
				setError("Username is required for basic auth");
				return;
			}
			if (!formToggles.basic_auth.password) {
				setError("Password is required for basic auth");
				return;
			}
		}

		if (routes.some((r) => r.domain === domain.trim())) {
			setError("A route for this domain already exists");
			return;
		}

		setSubmitting(true);
		try {
			await createRoute({
				domain: domain.trim(),
				upstream: upstream.trim(),
				toggles: formToggles,
			});
			setDomain("");
			setUpstream("");
			setFormToggles({ ...defaultToggles });
			setShowForm(false);
			await loadRoutes().catch(() => {});
		} catch (err) {
			setError(getErrorMessage(err, "Failed to create route"));
		} finally {
			setSubmitting(false);
		}
	}

	const handleDelete = useCallback(
		async (id: string) => {
			if (deletingRef.current) return;
			setDeleting(id);
			try {
				await deleteRoute(id);
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to delete route"));
			} finally {
				setDeleting(null);
			}
		},
		[loadRoutes],
	);

	const handleToggleEnabled = useCallback(
		async (route: ParsedRoute) => {
			try {
				if (route.disabled) {
					await enableRoute(route.id);
				} else {
					await disableRoute(route.id);
				}
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to toggle route"));
			}
		},
		[loadRoutes],
	);

	const handleUpdateToggles = useCallback(
		async (route: ParsedRoute, toggles: RouteToggles) => {
			try {
				await updateRoute({
					id: route.id,
					domain: route.domain,
					upstream: route.upstream,
					toggles,
				});
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to update route"));
				throw err;
			}
		},
		[loadRoutes],
	);

	if (loading) {
		return <div className="empty-state">Loading routes...</div>;
	}

	return (
		<div className="routes">
			<div className="section-header">
				<h2>Routes</h2>
				<button
					type="button"
					className="btn btn-primary add-route-btn"
					disabled={!caddyRunning}
					onClick={() => {
						if (showForm) setFormToggles({ ...defaultToggles });
						setShowForm(!showForm);
					}}
				>
					{showForm ? "Cancel" : "Add Route"}
				</button>
			</div>

			{!caddyRunning && (
				<div className="caddy-offline" role="status">
					Caddy is not running. Start it to manage routes.
				</div>
			)}

			{error && (
				<div className="alert-error" role="alert">
					{error}
					<button type="button" onClick={() => setError("")}>
						Dismiss
					</button>
				</div>
			)}

			{showForm && (
				<form className="add-route-form" onSubmit={handleAdd}>
					<div className="form-row">
						<div className="form-field">
							<label htmlFor="route-domain">Domain</label>
							<input
								id="route-domain"
								type="text"
								placeholder="example.com"
								value={domain}
								onChange={(e) => setDomain(e.target.value)}
								required
							/>
						</div>
						<div className="form-field">
							<label htmlFor="route-upstream">Upstream</label>
							<input
								id="route-upstream"
								type="text"
								placeholder="localhost:3000"
								value={upstream}
								onChange={(e) => setUpstream(e.target.value)}
								required
							/>
						</div>
					</div>
					<ToggleGrid
						toggles={formToggles}
						onUpdate={updateFormToggle}
						idPrefix="new-route"
						isNew
					/>
					<button type="submit" className="btn btn-primary submit-btn" disabled={submitting}>
						{submitting ? "Adding..." : "Add"}
					</button>
				</form>
			)}

			{routes.length === 0 ? (
				<div className="empty-state routes-empty">
					No routes yet. Routes map domains to your services
					{caddyRunning ? " - add one to get started." : "."}
				</div>
			) : (
				<div className="route-list">
					{routes.map((route, i) => (
						<RouteRow
							key={route.id || `${route.domain}-${route.upstream}-${i}`}
							route={route}
							deleting={deleting === route.id}
							onDelete={handleDelete}
							onToggleEnabled={handleToggleEnabled}
							onUpdateToggles={handleUpdateToggles}
						/>
					))}
				</div>
			)}
		</div>
	);
}
