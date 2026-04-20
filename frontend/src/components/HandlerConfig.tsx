import { useId, useRef, useState } from "react";
import { cn } from "../cn";
import type { RequestHeaders } from "../types/api";
import type {
	FileServerConfig,
	HandlerConfigValue,
	HandlerType,
	RedirectConfig,
	ReverseProxyConfig,
	StaticResponseConfig,
} from "../types/domain";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	type: HandlerType;
	config: HandlerConfigValue;
	onChange: (config: HandlerConfigValue) => void;
	disabled?: boolean;
	domain?: string;
}

const strategyLabels: Record<ReverseProxyConfig["load_balancing"]["strategy"], string> = {
	round_robin: "Round Robin",
	first: "Failover (Primary/Backup)",
	least_conn: "Least Connections",
	random: "Random",
	ip_hash: "IP Hash",
};

export default function HandlerConfig({ type, config, onChange, disabled, domain }: Props) {
	const idPrefix = useId();

	switch (type) {
		case "reverse_proxy": {
			const rpConfig = config as ReverseProxyConfig;
			const update = (patch: Partial<ReverseProxyConfig>) => {
				onChange({ ...rpConfig, ...patch });
			};
			return (
				<div className="handler-config">
					<div className="form-field">
						<label htmlFor={`${idPrefix}-upstream`}>Upstream</label>
						<input
							id={`${idPrefix}-upstream`}
							type="text"
							placeholder="localhost:3000"
							value={rpConfig.upstream}
							onChange={(e) => update({ upstream: e.target.value })}
							maxLength={260}
							required
							disabled={disabled}
						/>
					</div>
					<ToggleItem
						label="TLS Skip Verify"
						description="Skip TLS certificate verification for upstream"
						checked={rpConfig.tls_skip_verify}
						onChange={(v) => update({ tls_skip_verify: v })}
						disabled={disabled}
					/>
					<ToggleItem
						label="WebSocket Passthrough"
						description="Enable WebSocket passthrough to upstream"
						checked={rpConfig.websocket_passthrough}
						onChange={(v) => update({ websocket_passthrough: v })}
						disabled={disabled}
					/>
					<LoadBalancingSection config={rpConfig} onChange={update} disabled={disabled} />
					<RequestHeadersSection
						config={rpConfig.request_headers}
						onChange={(rh) => update({ request_headers: rh })}
						disabled={disabled}
					/>
				</div>
			);
		}
		case "static_response":
			return (
				<StaticResponseSection
					config={config as StaticResponseConfig}
					onChange={onChange}
					disabled={disabled}
				/>
			);
		case "redirect":
			return (
				<RedirectSection
					config={config as RedirectConfig}
					onChange={onChange}
					disabled={disabled}
					domain={domain}
				/>
			);
		case "file_server":
			return (
				<FileServerSection
					config={config as FileServerConfig}
					onChange={onChange}
					disabled={disabled}
				/>
			);
		case "" as HandlerType:
			return null;
		default:
			return (
				<div className="alert-warning" role="status">
					This handler type is not yet supported.
				</div>
			);
	}
}

interface LBEntry {
	id: number;
	value: string;
}

function LoadBalancingSection({
	config,
	onChange,
	disabled,
}: {
	config: ReverseProxyConfig;
	onChange: (patch: Partial<ReverseProxyConfig>) => void;
	disabled?: boolean;
}) {
	const idPrefix = useId();
	const lb = config.load_balancing;
	const nextId = useRef(lb.upstreams.length);
	const [entries, setEntries] = useState<LBEntry[]>(() =>
		lb.upstreams.map((v, i) => ({ id: i, value: v })),
	);

	const prevUpstreams = useRef(lb.upstreams);
	if (
		lb.upstreams.length !== prevUpstreams.current.length ||
		lb.upstreams.some((v, i) => v !== prevUpstreams.current[i])
	) {
		const currentValues = entries.map((e) => e.value);
		if (
			lb.upstreams.length !== currentValues.length ||
			lb.upstreams.some((v, i) => v !== currentValues[i])
		) {
			const newEntries = lb.upstreams.map((v, i) => ({ id: nextId.current + i, value: v }));
			nextId.current += lb.upstreams.length;
			setEntries(newEntries);
		}
		prevUpstreams.current = lb.upstreams;
	}

	function updateLB(patch: Partial<ReverseProxyConfig["load_balancing"]>) {
		onChange({ load_balancing: { ...lb, ...patch } });
	}

	function syncEntries(next: LBEntry[]) {
		setEntries(next);
		updateLB({ upstreams: next.map((e) => e.value) });
	}

	const isFailover = lb.strategy === "first";

	return (
		<div className="handler-lb-section">
			<ToggleItem
				label="Load Balancing"
				description="Multiple upstream strategy"
				checked={lb.enabled}
				onChange={(v) => updateLB({ enabled: v })}
				disabled={disabled}
			/>
			{lb.enabled && (
				<div className="toggle-detail">
					<label htmlFor={`lb-strategy-${idPrefix}`}>Strategy</label>
					<select
						id={`lb-strategy-${idPrefix}`}
						value={lb.strategy}
						onChange={(e) =>
							updateLB({
								strategy: e.target.value as ReverseProxyConfig["load_balancing"]["strategy"],
							})
						}
						disabled={disabled}
					>
						{(Object.keys(strategyLabels) as Array<keyof typeof strategyLabels>).map((key) => (
							<option key={key} value={key}>
								{strategyLabels[key]}
							</option>
						))}
					</select>
					{isFailover ? (
						<div className="lb-failover-info">
							<span className="lb-primary-badge">Primary</span>
							<span>The upstream above is the primary server</span>
						</div>
					) : (
						<span className="lb-pool-hint">The upstream above is included in the pool</span>
					)}
					<span className="toggle-detail-heading">
						{isFailover ? "Fallback Servers" : "Additional Upstreams"}
					</span>
					{entries.map((entry, i) => (
						<div
							className={isFailover ? "lb-upstream-row lb-upstream-fallback" : "lb-upstream-row"}
							key={`${idPrefix}-lbu-${entry.id}`}
						>
							{isFailover && <span className="lb-fallback-badge">#{i + 1}</span>}
							<input
								type="text"
								placeholder="localhost:3001"
								maxLength={260}
								value={entry.value}
								disabled={disabled}
								onChange={(e) => {
									const next = [...entries];
									next[i] = { ...entry, value: e.target.value };
									syncEntries(next);
								}}
							/>
							<button
								type="button"
								className="btn btn-ghost lb-upstream-remove"
								onClick={() => syncEntries(entries.filter((_, j) => j !== i))}
								aria-label="Remove upstream"
								disabled={disabled}
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
							syncEntries([...entries, { id: nextId.current, value: "" }]);
						}}
						disabled={disabled}
					>
						+ Add Upstream
					</button>
				</div>
			)}
		</div>
	);
}

interface HeaderEntry {
	id: number;
	key: string;
	value: string;
}

function StaticResponseSection({
	config,
	onChange,
	disabled,
}: {
	config: StaticResponseConfig;
	onChange: (config: StaticResponseConfig) => void;
	disabled?: boolean;
}) {
	const idPrefix = useId();
	const nextId = useRef(Object.keys(config.headers).length);
	const [headerEntries, setHeaderEntries] = useState<HeaderEntry[]>(() =>
		Object.entries(config.headers).map(([k, v], i) => ({
			id: i,
			key: k,
			value: v[0] ?? "",
		})),
	);

	function update(patch: Partial<StaticResponseConfig>) {
		onChange({ ...config, ...patch });
	}

	function syncHeaders(next: HeaderEntry[]) {
		setHeaderEntries(next);
		const headers: Record<string, string[]> = {};
		for (const entry of next) {
			if (entry.key) {
				headers[entry.key] = [entry.value];
			}
		}
		update({ headers });
	}

	return (
		<div className="handler-config">
			<ToggleItem
				label="Close Connection"
				description="Immediately close without sending a response"
				checked={config.close}
				onChange={(v) => update({ close: v })}
				disabled={disabled}
			/>
			{!config.close && (
				<>
					<div className="form-field">
						<label htmlFor={`${idPrefix}-status`}>Status Code</label>
						<input
							id={`${idPrefix}-status`}
							type="text"
							placeholder="200"
							value={config.status_code}
							onChange={(e) =>
								update({ status_code: e.target.value.replace(/\D/g, "").slice(0, 3) })
							}
							maxLength={3}
							disabled={disabled}
						/>
					</div>
					<div className="form-field">
						<label htmlFor={`${idPrefix}-body`}>Body</label>
						<textarea
							id={`${idPrefix}-body`}
							placeholder="Response body"
							rows={4}
							value={config.body}
							onChange={(e) => update({ body: e.target.value })}
							disabled={disabled}
						/>
					</div>
					<div className="handler-static-headers">
						<span className="toggle-detail-heading">Response Headers</span>
						{headerEntries.map((entry, i) => (
							<div className="lb-upstream-row" key={`${idPrefix}-sh-${entry.id}`}>
								<input
									type="text"
									placeholder="Header name"
									value={entry.key}
									maxLength={256}
									disabled={disabled}
									onChange={(e) => {
										const next = [...headerEntries];
										next[i] = { ...entry, key: e.target.value };
										syncHeaders(next);
									}}
								/>
								<input
									type="text"
									placeholder="Value"
									value={entry.value}
									maxLength={1024}
									disabled={disabled}
									onChange={(e) => {
										const next = [...headerEntries];
										next[i] = { ...entry, value: e.target.value };
										syncHeaders(next);
									}}
								/>
								<button
									type="button"
									className="btn btn-ghost lb-upstream-remove"
									onClick={() => syncHeaders(headerEntries.filter((_, j) => j !== i))}
									aria-label="Remove header"
									disabled={disabled}
								>
									&#x2715;
								</button>
							</div>
						))}
						<button
							type="button"
							className="btn btn-primary"
							style={{ alignSelf: "flex-start" }}
							onClick={() => {
								nextId.current += 1;
								syncHeaders([...headerEntries, { id: nextId.current, key: "", value: "" }]);
							}}
							disabled={disabled}
						>
							+ Add Header
						</button>
					</div>
				</>
			)}
		</div>
	);
}

const redirectStatusOptions = [
	{ value: "301", label: "301 - Permanent Redirect" },
	{ value: "302", label: "302 - Found (Temporary)" },
	{ value: "303", label: "303 - See Other" },
	{ value: "307", label: "307 - Temporary Redirect" },
	{ value: "308", label: "308 - Permanent Redirect (Strict)" },
	{ value: "custom", label: "Custom..." },
];

function redirectPlaceholder(domain: string | undefined): string {
	if (!domain) return "https://example.com or /new-path";
	const parts = domain.split(".");
	parts[0] = `new-${parts[0]}`;
	return `https://${parts.join(".")}`;
}

function RedirectSection({
	config,
	onChange,
	disabled,
	domain,
}: {
	config: RedirectConfig;
	onChange: (config: RedirectConfig) => void;
	disabled?: boolean;
	domain?: string;
}) {
	const idPrefix = useId();
	const [customMode, setCustomMode] = useState(
		() => !redirectStatusOptions.some((o) => o.value === config.status_code),
	);

	function update(patch: Partial<RedirectConfig>) {
		onChange({ ...config, ...patch });
	}

	const statusCode = Number.parseInt(config.status_code, 10);
	const showWarning =
		config.status_code !== "" &&
		!Number.isNaN(statusCode) &&
		(statusCode < 300 || statusCode > 399);

	return (
		<div className="handler-config">
			<div className="form-field">
				<label htmlFor={`${idPrefix}-target`}>Target URL</label>
				<input
					id={`${idPrefix}-target`}
					type="text"
					placeholder={redirectPlaceholder(domain)}
					value={config.target_url}
					onChange={(e) => update({ target_url: e.target.value })}
					maxLength={2048}
					required
					disabled={disabled}
				/>
			</div>
			<div className="form-field">
				<label htmlFor={`${idPrefix}-status`}>Status Code</label>
				{customMode ? (
					<div className="redirect-status-custom">
						<input
							id={`${idPrefix}-status`}
							type="text"
							placeholder="301"
							value={config.status_code}
							onChange={(e) =>
								update({ status_code: e.target.value.replace(/\D/g, "").slice(0, 3) })
							}
							maxLength={3}
							disabled={disabled}
						/>
						<button
							type="button"
							className="btn btn-ghost"
							onClick={() => {
								setCustomMode(false);
								update({ status_code: "301" });
							}}
							disabled={disabled}
						>
							Presets
						</button>
					</div>
				) : (
					<select
						id={`${idPrefix}-status`}
						value={
							redirectStatusOptions.some((o) => o.value === config.status_code)
								? config.status_code
								: "custom"
						}
						onChange={(e) => {
							if (e.target.value === "custom") {
								setCustomMode(true);
								update({ status_code: "" });
							} else {
								update({ status_code: e.target.value });
							}
						}}
						disabled={disabled}
					>
						{redirectStatusOptions.map((o) => (
							<option key={o.value} value={o.value}>
								{o.label}
							</option>
						))}
					</select>
				)}
				{showWarning && (
					<span className="field-hint field-hint-warn">
						Non-3xx status codes may not trigger a browser redirect.
					</span>
				)}
			</div>
			<ToggleItem
				label="Preserve Path"
				description="Appends the original request path and query string to the target URL"
				checked={config.preserve_path}
				onChange={(v) => update({ preserve_path: v })}
				disabled={disabled}
			/>
		</div>
	);
}

function FileServerSection({
	config,
	onChange,
	disabled,
}: {
	config: FileServerConfig;
	onChange: (config: FileServerConfig) => void;
	disabled?: boolean;
}) {
	const idPrefix = useId();
	const [newIndex, setNewIndex] = useState("");
	const [newHide, setNewHide] = useState("");

	function update(patch: Partial<FileServerConfig>) {
		onChange({ ...config, ...patch });
	}

	return (
		<div className="handler-config">
			<div className="form-field">
				<label htmlFor={`${idPrefix}-root`}>Root Directory</label>
				<input
					id={`${idPrefix}-root`}
					type="text"
					placeholder="/var/www/html"
					value={config.root}
					onChange={(e) => update({ root: e.target.value })}
					maxLength={4096}
					required
					disabled={disabled}
				/>
			</div>
			<ToggleItem
				label="Directory Browsing"
				description="Show a file listing when no index file exists"
				checked={config.browse}
				onChange={(v) => update({ browse: v })}
				disabled={disabled}
			/>
			<div className="handler-static-headers">
				<span className="toggle-detail-heading">Index Files</span>
				{config.index_names.map((name, i) => (
					<div className="lb-upstream-row" key={`${idPrefix}-idx-${name}`}>
						<input type="text" value={name} disabled readOnly />
						<button
							type="button"
							className="btn btn-ghost lb-upstream-remove"
							onClick={() =>
								update({
									index_names: config.index_names.filter((_, idx) => idx !== i),
								})
							}
							aria-label="Remove index file"
							disabled={disabled}
						>
							&#x2715;
						</button>
					</div>
				))}
				<div className="lb-upstream-row">
					<input
						type="text"
						placeholder="index.html"
						value={newIndex}
						onChange={(e) => setNewIndex(e.target.value)}
						maxLength={256}
						disabled={disabled}
					/>
					<button
						type="button"
						className="btn btn-primary"
						onClick={() => {
							if (newIndex.trim()) {
								update({
									index_names: [...config.index_names, newIndex.trim()],
								});
								setNewIndex("");
							}
						}}
						disabled={disabled || !newIndex.trim()}
					>
						Add
					</button>
				</div>
			</div>
			<div className="handler-static-headers">
				<span className="toggle-detail-heading">Hidden Files</span>
				{config.hide.map((pattern, i) => (
					<div className="lb-upstream-row" key={`${idPrefix}-hide-${pattern}`}>
						<input type="text" value={pattern} disabled readOnly />
						<button
							type="button"
							className="btn btn-ghost lb-upstream-remove"
							onClick={() =>
								update({
									hide: config.hide.filter((_, idx) => idx !== i),
								})
							}
							aria-label="Remove hidden pattern"
							disabled={disabled}
						>
							&#x2715;
						</button>
					</div>
				))}
				<div className="lb-upstream-row">
					<input
						type="text"
						placeholder=".*"
						value={newHide}
						onChange={(e) => setNewHide(e.target.value)}
						maxLength={256}
						disabled={disabled}
					/>
					<button
						type="button"
						className="btn btn-primary"
						onClick={() => {
							if (newHide.trim()) {
								update({
									hide: [...config.hide, newHide.trim()],
								});
								setNewHide("");
							}
						}}
						disabled={disabled || !newHide.trim()}
					>
						Add
					</button>
				</div>
			</div>
		</div>
	);
}

function RequestHeadersSection({
	config,
	onChange,
	disabled,
}: {
	config: RequestHeaders;
	onChange: (config: RequestHeaders) => void;
	disabled?: boolean;
}) {
	const idPrefix = useId();

	function update(patch: Partial<RequestHeaders>) {
		onChange({ ...config, ...patch });
	}

	return (
		<div className={cn("toggle-group", config.enabled && "toggle-group-open")}>
			<ToggleItem
				label="Request Headers"
				description="Modify headers sent to upstream"
				checked={config.enabled}
				onChange={(v) => update({ enabled: v })}
				disabled={disabled}
			/>
			{config.enabled && (
				<div className="toggle-detail">
					<div className={cn("toggle-group", config.host_override && "toggle-group-open")}>
						<ToggleItem
							label="Host Override"
							description="Set the Host header sent to upstream"
							checked={config.host_override}
							onChange={(v) => update({ host_override: v })}
							disabled={disabled}
						/>
						{config.host_override && (
							<div className="toggle-detail">
								<label htmlFor={`${idPrefix}-host-value`}>Host Value</label>
								<input
									id={`${idPrefix}-host-value`}
									type="text"
									placeholder="example.com"
									value={config.host_value}
									onChange={(e) => update({ host_value: e.target.value })}
									maxLength={260}
									disabled={disabled}
								/>
							</div>
						)}
					</div>
					<div className={cn("toggle-group", config.authorization && "toggle-group-open")}>
						<ToggleItem
							label="Authorization"
							description="Set the Authorization header sent to upstream"
							checked={config.authorization}
							onChange={(v) => update({ authorization: v })}
							disabled={disabled}
						/>
						{config.authorization && (
							<div className="toggle-detail">
								<label htmlFor={`${idPrefix}-auth-value`}>Authorization Value</label>
								<input
									id={`${idPrefix}-auth-value`}
									type="text"
									placeholder="Bearer token123"
									value={config.auth_value}
									onChange={(e) => update({ auth_value: e.target.value })}
									maxLength={1024}
									disabled={disabled}
								/>
							</div>
						)}
					</div>
				</div>
			)}
		</div>
	);
}

export function handlerSummary(type: HandlerType, config: HandlerConfigValue): string {
	switch (type) {
		case "reverse_proxy": {
			const rp = config as ReverseProxyConfig;
			const parts: string[] = [rp.upstream];
			if (rp.tls_skip_verify) parts.push("TLS skip");
			if (rp.websocket_passthrough) parts.push("WS");
			if (rp.load_balancing.enabled) {
				const extra = rp.load_balancing.upstreams.length;
				parts.push(`LB: ${rp.load_balancing.strategy}${extra > 0 ? ` (+${extra})` : ""}`);
			}
			return parts.join(" / ");
		}
		case "static_response": {
			const sr = config as StaticResponseConfig;
			if (sr.close) return "Close connection";
			return "Static response";
		}
		case "redirect": {
			const rd = config as RedirectConfig;
			const target = rd.target_url || "...";
			const suffix = rd.preserve_path ? " (+ path)" : "";
			return `${rd.status_code} -> ${target}${suffix}`;
		}
		case "file_server": {
			const fs = config as FileServerConfig;
			const parts: string[] = [fs.root || "..."];
			if (fs.browse) parts.push("browse");
			return parts.join(" / ");
		}
		case "" as HandlerType:
			return "No handler";
		default:
			return "Not yet supported";
	}
}
