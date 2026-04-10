import { useEffect, useId, useRef, useState } from "react";
import { fetchIPLists } from "../api";
import { cn } from "../cn";
import type { IPList, RouteToggles } from "../types/api";
import { Toggle } from "./Toggle";

interface ToggleGridProps {
	toggles: RouteToggles;
	onUpdate: <K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) => void;
	idPrefix: string;
	isNew?: boolean;
	domain?: string;
	globalAutoHttps?: "on" | "off" | "disable_redirects";
}

export default function ToggleGrid({
	toggles,
	onUpdate,
	idPrefix,
	isNew,
	domain,
	globalAutoHttps,
}: ToggleGridProps) {
	const [ipLists, setIpLists] = useState<IPList[]>([]);

	useEffect(() => {
		if (toggles.ip_filtering.enabled) {
			fetchIPLists()
				.then(setIpLists)
				.catch(() => {});
		}
	}, [toggles.ip_filtering.enabled]);

	return (
		<div className="toggle-grid">
			{globalAutoHttps && globalAutoHttps !== "off" ? (
				<ToggleItem
					label="Force HTTPS"
					description="Managed by global HTTPS setting"
					checked={globalAutoHttps === "on"}
					onChange={() => {}}
					disabled
				/>
			) : (
				<ToggleItem
					label="Force HTTPS"
					description="Redirect HTTP requests to HTTPS"
					checked={toggles.force_https}
					onChange={(v) => onUpdate("force_https", v)}
				/>
			)}
			<ToggleItem
				label="Compression"
				description="gzip + zstd encoding"
				checked={toggles.compression}
				onChange={(v) => onUpdate("compression", v)}
			/>
			<ToggleItem
				label="Security Headers"
				description="X-Content-Type-Options, X-Frame-Options, Referrer-Policy"
				checked={toggles.security_headers}
				onChange={(v) => onUpdate("security_headers", v)}
			/>
			<CorsGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} />
			<ToggleItem
				label="TLS Skip Verify"
				description="Skip TLS certificate verification for upstream"
				checked={toggles.tls_skip_verify}
				onChange={(v) => onUpdate("tls_skip_verify", v)}
			/>
			<BasicAuthGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} isNew={isNew} />
			<AccessLogGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} domain={domain} />
			<ToggleItem
				label="WebSocket"
				description="Enable WebSocket passthrough"
				checked={toggles.websocket_passthrough}
				onChange={(v) => onUpdate("websocket_passthrough", v)}
			/>
			<LoadBalancingGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} />
			<IPFilteringGroup toggles={toggles} onUpdate={onUpdate} ipLists={ipLists} />
		</div>
	);
}

interface GroupProps {
	toggles: RouteToggles;
	onUpdate: <K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) => void;
	idPrefix: string;
}

function CorsGroup({ toggles, onUpdate, idPrefix }: GroupProps) {
	return (
		<div className={cn("toggle-group", toggles.cors.enabled && "toggle-group-open")}>
			<ToggleItem
				label="CORS"
				description="Cross-origin resource sharing headers"
				checked={toggles.cors.enabled}
				onChange={(v) => onUpdate("cors", { ...toggles.cors, enabled: v })}
			/>
			{toggles.cors.enabled && (
				<div className="toggle-detail">
					<label htmlFor={`cors-origins-${idPrefix}`}>Allowed Origins</label>
					<input
						id={`cors-origins-${idPrefix}`}
						type="text"
						placeholder="* (all origins)"
						maxLength={2000}
						value={toggles.cors.allowed_origins.join(", ")}
						onChange={(e) => {
							const origins = e.target.value
								.split(",")
								.map((s) => s.trim())
								.filter(Boolean);
							onUpdate("cors", { ...toggles.cors, allowed_origins: origins });
						}}
					/>
				</div>
			)}
		</div>
	);
}

function BasicAuthGroup({ toggles, onUpdate, idPrefix, isNew }: GroupProps & { isNew?: boolean }) {
	return (
		<div className={cn("toggle-group", toggles.basic_auth.enabled && "toggle-group-open")}>
			<ToggleItem
				label="Basic Auth"
				description="HTTP basic authentication"
				checked={toggles.basic_auth.enabled}
				onChange={(v) => onUpdate("basic_auth", { ...toggles.basic_auth, enabled: v })}
			/>
			{toggles.basic_auth.enabled && (
				<div className="toggle-detail">
					<label htmlFor={`auth-user-${idPrefix}`}>Username</label>
					<input
						id={`auth-user-${idPrefix}`}
						type="text"
						placeholder="admin"
						maxLength={255}
						value={toggles.basic_auth.username}
						onChange={(e) =>
							onUpdate("basic_auth", {
								...toggles.basic_auth,
								username: e.target.value,
							})
						}
					/>
					<label htmlFor={`auth-pass-${idPrefix}`}>Password</label>
					<input
						id={`auth-pass-${idPrefix}`}
						type="password"
						placeholder={isNew ? "required" : "(unchanged)"}
						maxLength={512}
						value={toggles.basic_auth.password}
						onChange={(e) =>
							onUpdate("basic_auth", {
								...toggles.basic_auth,
								password: e.target.value,
							})
						}
					/>
				</div>
			)}
		</div>
	);
}

function AccessLogGroup({ toggles, onUpdate, idPrefix, domain }: GroupProps & { domain?: string }) {
	return (
		<div className={cn("toggle-group", toggles.access_log && "toggle-group-open")}>
			<ToggleItem
				label="Access Log"
				description="Log requests to this route"
				checked={toggles.access_log !== ""}
				onChange={(v) => onUpdate("access_log", v ? "kaji_access" : "")}
			/>
			{toggles.access_log !== "" && (
				<div className="toggle-detail">
					<label className="toggle-radio-option">
						<input
							type="radio"
							name={`${idPrefix}-access-log-type`}
							checked={toggles.access_log === "kaji_access"}
							onChange={() => onUpdate("access_log", "kaji_access")}
						/>
						<span>Shared (kaji_access)</span>
					</label>
					<label className="toggle-radio-option">
						<input
							type="radio"
							name={`${idPrefix}-access-log-type`}
							checked={toggles.access_log !== "" && toggles.access_log !== "kaji_access"}
							onChange={() => {
								const defaultName = (domain ?? "").replace(/[^a-zA-Z0-9_-]/g, "_") || "custom";
								onUpdate("access_log", defaultName);
							}}
						/>
						<span>Dedicated</span>
					</label>
					{toggles.access_log !== "" && toggles.access_log !== "kaji_access" && (
						<input
							id={`access-log-name-${idPrefix}`}
							type="text"
							placeholder="sink name"
							maxLength={255}
							value={toggles.access_log}
							onChange={(e) => {
								const sanitized = e.target.value.replace(/\s+/g, "_");
								onUpdate("access_log", sanitized);
							}}
						/>
					)}
				</div>
			)}
		</div>
	);
}

function LoadBalancingGroup({ toggles, onUpdate, idPrefix }: GroupProps) {
	return (
		<div className={cn("toggle-group", toggles.load_balancing.enabled && "toggle-group-open")}>
			<ToggleItem
				label="Load Balancing"
				description="Multiple upstream strategy"
				checked={toggles.load_balancing.enabled}
				onChange={(v) => onUpdate("load_balancing", { ...toggles.load_balancing, enabled: v })}
			/>
			{toggles.load_balancing.enabled && (
				<div className="toggle-detail">
					<label htmlFor={`lb-strategy-${idPrefix}`}>Strategy</label>
					<select
						id={`lb-strategy-${idPrefix}`}
						value={toggles.load_balancing.strategy}
						onChange={(e) =>
							onUpdate("load_balancing", {
								...toggles.load_balancing,
								strategy: e.target.value as RouteToggles["load_balancing"]["strategy"],
							})
						}
					>
						<option value="round_robin">Round Robin</option>
						<option value="first">Failover (Primary/Backup)</option>
						<option value="least_conn">Least Connections</option>
						<option value="random">Random</option>
						<option value="ip_hash">IP Hash</option>
					</select>
					{toggles.load_balancing.strategy === "first" ? (
						<div className="lb-failover-info">
							<span className="lb-primary-badge">Primary</span>
							<span>The upstream above is the primary server</span>
						</div>
					) : (
						<span className="lb-pool-hint">The upstream above is included in the pool</span>
					)}
					<span className="toggle-detail-heading">
						{toggles.load_balancing.strategy === "first"
							? "Fallback Servers"
							: "Additional Upstreams"}
					</span>
					<UpstreamList
						upstreams={toggles.load_balancing.upstreams}
						idPrefix={idPrefix}
						isFailover={toggles.load_balancing.strategy === "first"}
						onChange={(upstreams) =>
							onUpdate("load_balancing", { ...toggles.load_balancing, upstreams })
						}
					/>
				</div>
			)}
		</div>
	);
}

function IPFilteringGroup({
	toggles,
	onUpdate,
	ipLists,
}: Omit<GroupProps, "idPrefix"> & { ipLists: IPList[] }) {
	return (
		<div className={cn("toggle-group", toggles.ip_filtering.enabled && "toggle-group-open")}>
			<ToggleItem
				label="IP Filtering"
				description="Restrict access by IP whitelist or blacklist"
				checked={toggles.ip_filtering.enabled}
				onChange={(v) =>
					onUpdate(
						"ip_filtering",
						v
							? { enabled: true, list_id: "", type: "blacklist" }
							: { enabled: false, list_id: "", type: "" },
					)
				}
			/>
			{toggles.ip_filtering.enabled && (
				<div className="toggle-detail">
					<div className="ip-list-type-toggle">
						<button
							type="button"
							className={cn(
								"type-toggle-btn",
								toggles.ip_filtering.type === "blacklist" && "active",
							)}
							onClick={() =>
								onUpdate("ip_filtering", { enabled: true, list_id: "", type: "blacklist" })
							}
						>
							Blacklist
						</button>
						<button
							type="button"
							className={cn(
								"type-toggle-btn",
								toggles.ip_filtering.type === "whitelist" && "active",
							)}
							onClick={() =>
								onUpdate("ip_filtering", { enabled: true, list_id: "", type: "whitelist" })
							}
						>
							Whitelist
						</button>
					</div>
					{toggles.ip_filtering.type && (
						<select
							value={toggles.ip_filtering.list_id}
							onChange={(e) =>
								onUpdate("ip_filtering", {
									...toggles.ip_filtering,
									list_id: e.target.value,
								})
							}
						>
							<option value="">Select a {toggles.ip_filtering.type}...</option>
							{ipLists
								.filter((l) => l.type === toggles.ip_filtering.type)
								.map((l) => (
									<option key={l.id} value={l.id}>
										{l.name} ({l.resolved_count} IPs)
									</option>
								))}
						</select>
					)}
				</div>
			)}
		</div>
	);
}

function ToggleItem({
	label,
	description,
	checked,
	onChange,
	disabled,
}: {
	label: string;
	description: string;
	checked: boolean;
	onChange: (v: boolean) => void;
	disabled?: boolean;
}) {
	const id = useId();
	return (
		<label className={cn("toggle-item", disabled && "toggle-item-disabled")} htmlFor={id}>
			<div className="toggle-item-text">
				<span className="toggle-item-label">{label}</span>
				<span className="toggle-item-desc">{description}</span>
			</div>
			<Toggle inline small id={id} checked={checked} onChange={onChange} disabled={disabled} />
		</label>
	);
}

interface UpstreamEntry {
	id: number;
	value: string;
}

function UpstreamList({
	upstreams,
	idPrefix,
	isFailover,
	onChange,
}: {
	upstreams: string[];
	idPrefix: string;
	isFailover?: boolean;
	onChange: (upstreams: string[]) => void;
}) {
	const nextId = useRef(upstreams.length);
	const [entries, setEntries] = useState<UpstreamEntry[]>(() =>
		upstreams.map((v, i) => ({ id: i, value: v })),
	);

	function sync(next: UpstreamEntry[]) {
		setEntries(next);
		onChange(next.map((e) => e.value));
	}

	// Keep entries in sync when upstreams change externally (e.g. reset)
	const prevUpstreams = useRef(upstreams);
	if (
		upstreams.length !== prevUpstreams.current.length ||
		upstreams.some((v, i) => v !== prevUpstreams.current[i])
	) {
		const currentValues = entries.map((e) => e.value);
		if (
			upstreams.length !== currentValues.length ||
			upstreams.some((v, i) => v !== currentValues[i])
		) {
			const newEntries = upstreams.map((v, i) => ({ id: nextId.current + i, value: v }));
			nextId.current += upstreams.length;
			setEntries(newEntries);
		}
		prevUpstreams.current = upstreams;
	}

	return (
		<>
			{entries.map((entry, i) => (
				<div
					className={cn("lb-upstream-row", isFailover && "lb-upstream-fallback")}
					key={`${idPrefix}-lbu-${entry.id}`}
				>
					{isFailover && <span className="lb-fallback-badge">#{i + 1}</span>}
					<input
						type="text"
						placeholder="localhost:3001"
						maxLength={260}
						value={entry.value}
						onChange={(e) => {
							const next = [...entries];
							next[i] = { ...entry, value: e.target.value };
							sync(next);
						}}
					/>
					<button
						type="button"
						className="btn btn-ghost lb-upstream-remove"
						onClick={() => sync(entries.filter((_, j) => j !== i))}
						aria-label="Remove upstream"
					>
						&#x2715;
					</button>
				</div>
			))}
			<button
				type="button"
				className="btn btn-ghost lb-add-upstream"
				onClick={() => {
					nextId.current += 1;
					sync([...entries, { id: nextId.current, value: "" }]);
				}}
			>
				+ Add Upstream
			</button>
		</>
	);
}
