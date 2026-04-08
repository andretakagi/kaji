import { useRef, useState } from "react";
import type { RouteToggles } from "../types/api";

interface ToggleGridProps {
	toggles: RouteToggles;
	onUpdate: <K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) => void;
	idPrefix: string;
	isNew?: boolean;
}

export default function ToggleGrid({ toggles, onUpdate, idPrefix, isNew }: ToggleGridProps) {
	return (
		<div className="toggle-grid">
			<ToggleItem
				label="Force HTTPS"
				description="Redirect HTTP requests to HTTPS"
				checked={toggles.force_https}
				onChange={(v) => onUpdate("force_https", v)}
			/>
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
			<div className={`toggle-group${toggles.cors.enabled ? " toggle-group-open" : ""}`}>
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
			<ToggleItem
				label="TLS Skip Verify"
				description="Skip TLS certificate verification for upstream"
				checked={toggles.tls_skip_verify}
				onChange={(v) => onUpdate("tls_skip_verify", v)}
			/>
			<div className={`toggle-group${toggles.basic_auth.enabled ? " toggle-group-open" : ""}`}>
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
			<ToggleItem
				label="Access Log"
				description="Log requests to this route"
				checked={toggles.access_log}
				onChange={(v) => onUpdate("access_log", v)}
			/>
			<ToggleItem
				label="WebSocket"
				description="Enable WebSocket passthrough"
				checked={toggles.websocket_passthrough}
				onChange={(v) => onUpdate("websocket_passthrough", v)}
			/>
			<div className={`toggle-group${toggles.load_balancing.enabled ? " toggle-group-open" : ""}`}>
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
		</div>
	);
}

function ToggleItem({
	label,
	description,
	checked,
	onChange,
}: {
	label: string;
	description: string;
	checked: boolean;
	onChange: (v: boolean) => void;
}) {
	return (
		<label className="toggle-item">
			<div className="toggle-item-text">
				<span className="toggle-item-label">{label}</span>
				<span className="toggle-item-desc">{description}</span>
			</div>
			<div className="toggle-switch small">
				<input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
				<span className="toggle-slider" />
			</div>
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
		// Only reset entries if the values actually differ from what we have
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
					className={`lb-upstream-row${isFailover ? " lb-upstream-fallback" : ""}`}
					key={`${idPrefix}-lbu-${entry.id}`}
				>
					{isFailover && <span className="lb-fallback-badge">#{i + 1}</span>}
					<input
						type="text"
						placeholder="localhost:3001"
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
