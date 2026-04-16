import { fetchConfig, fetchDisabledRoutes, fetchIPLists, fetchRouteIPListBindings } from "../api";
import { RequireCaddy, useCaddyStatus } from "../contexts/CaddyContext";
import { usePolledData } from "../hooks/usePolledData";
import { useRouteForm } from "../hooks/useRouteForm";
import type { ParsedRoute } from "../types/api";
import type { CaddyConfig } from "../types/caddy";
import { parseDisabledRoute, parseRoute } from "../utils/parseRoutes";
import { ErrorAlert } from "./ErrorAlert";
import LoadingState from "./LoadingState";
import RouteRow from "./RouteRow";
import { SectionHeader } from "./SectionHeader";
import ToggleGrid from "./ToggleGrid";

export default function Routes() {
	const { caddyRunning } = useCaddyStatus();
	const fetchRoutes = async (): Promise<ParsedRoute[]> => {
		const parsed: ParsedRoute[] = [];

		if (caddyRunning) {
			const [config, disabled, bindings, ipListsData] = await Promise.all([
				fetchConfig(),
				fetchDisabledRoutes(),
				fetchRouteIPListBindings(),
				fetchIPLists(),
			]);
			const servers = (config as CaddyConfig).apps?.http?.servers;
			if (servers) {
				for (const [name, server] of Object.entries(servers)) {
					const domainSinks = new Map<string, string>(
						Object.entries(server.logs?.logger_names ?? {}),
					);
					for (const route of server.routes ?? []) {
						parsed.push(parseRoute(route, name, domainSinks));
					}
				}
			}
			for (const dr of disabled) {
				parsed.push(parseDisabledRoute(dr));
			}
			for (const route of parsed) {
				const listID = bindings[route.id];
				if (listID) {
					const list = ipListsData.find((l) => l.id === listID);
					if (list) {
						route.toggles = {
							...route.toggles,
							ip_filtering: {
								enabled: true,
								list_id: listID,
								type: list.type,
							},
						};
					}
				}
			}
		} else {
			const disabled = await fetchDisabledRoutes();
			for (const dr of disabled) {
				parsed.push(parseDisabledRoute(dr));
			}
		}

		return parsed;
	};

	const {
		data: routes,
		loading,
		error,
		setError,
		reload: loadRoutes,
	} = usePolledData({
		fetcher: fetchRoutes,
		initialData: [] as ParsedRoute[],
		errorPrefix: "Failed to load routes",
	});

	const {
		globalToggles,
		routeSettings,
		domain,
		setDomain,
		upstream,
		setUpstream,
		formToggles,
		updateFormToggle,
		form,
		warning,
		setWarning,
		submitting,
		deleting,
		toggling,
		handleAdd,
		handleDelete,
		handleToggleEnabled,
		handleUpdateToggles,
	} = useRouteForm({ routes, loadRoutes, setError });

	if (loading) {
		return <LoadingState label="routes" />;
	}

	return (
		<div className="routes">
			<SectionHeader title="Routes">
				<button
					type="button"
					className="btn btn-primary add-route-btn"
					disabled={!caddyRunning}
					onClick={form.toggle}
				>
					{form.visible ? "Cancel" : "Add Route"}
				</button>
			</SectionHeader>

			<RequireCaddy message="Start it to manage routes." />

			<ErrorAlert message={error} onDismiss={() => setError("")} />
			{warning && (
				<div className="alert-warning" role="status">
					{warning}
					<button type="button" onClick={() => setWarning("")}>
						Dismiss
					</button>
				</div>
			)}

			{form.visible && (
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
								maxLength={253}
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
								maxLength={260}
								required
							/>
						</div>
					</div>
					<ToggleGrid
						toggles={formToggles}
						onUpdate={updateFormToggle}
						idPrefix="new-route"
						isNew
						domain={domain}
						globalAutoHttps={globalToggles?.auto_https}
					/>
					<button type="submit" className="btn btn-primary submit-btn" disabled={submitting}>
						{submitting ? "Adding..." : "Add"}
					</button>
				</form>
			)}

			{routes.length === 0 ? (
				<div className="empty-state routes-empty">
					No routes yet. Routes connect a domain to an upstream service, with options for HTTPS,
					headers, IP filtering, and more{caddyRunning ? "." : "."}
				</div>
			) : (
				<div className="route-list">
					{routes.map((route, i) => (
						<RouteRow
							key={route.id || `${route.domain}-${route.upstream}-${i}`}
							route={route}
							deleting={deleting === route.id}
							toggling={toggling === route.id}
							onDelete={handleDelete}
							onToggleEnabled={handleToggleEnabled}
							onUpdateToggles={handleUpdateToggles}
							globalAutoHttps={globalToggles?.auto_https}
							routeSettings={routeSettings}
						/>
					))}
				</div>
			)}
		</div>
	);
}
