import { useId, useRef, useState } from "react";
import type { HandlerType, ReverseProxyConfig } from "../types/domain";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	type: HandlerType;
	config: ReverseProxyConfig | Record<string, unknown>;
	onChange: (config: ReverseProxyConfig | Record<string, unknown>) => void;
	disabled?: boolean;
}

const strategyLabels: Record<ReverseProxyConfig["load_balancing"]["strategy"], string> = {
	round_robin: "Round Robin",
	first: "Failover (Primary/Backup)",
	least_conn: "Least Connections",
	random: "Random",
	ip_hash: "IP Hash",
};

export default function HandlerConfig({ type, config, onChange, disabled }: Props) {
	if (type !== "reverse_proxy") {
		return (
			<div className="alert-warning" role="status">
				This handler type is not yet supported. Only reverse proxy is available for now.
			</div>
		);
	}

	const rpConfig = config as ReverseProxyConfig;

	function update(patch: Partial<ReverseProxyConfig>) {
		onChange({ ...rpConfig, ...patch });
	}

	return (
		<div className="handler-config">
			<div className="form-field">
				<label htmlFor="handler-upstream">Upstream</label>
				<input
					id="handler-upstream"
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
		</div>
	);
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

export function handlerSummary(
	type: HandlerType,
	config: ReverseProxyConfig | Record<string, unknown>,
): string {
	if (type !== "reverse_proxy") return "Not yet supported";
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
