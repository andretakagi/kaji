import type {
	FileServerConfig,
	ReverseProxyConfig,
	Rule,
	StaticResponseConfig,
} from "../types/domain";
import { handlerSummary } from "./HandlerConfig";

export function RuleDetails({
	rule,
	onEdit,
	disabled,
}: {
	rule: Rule;
	onEdit?: () => void;
	disabled?: boolean;
}) {
	return (
		<div className="rule-card-body">
			{rule.handler_type === "none" ? (
				<div className="rule-card-detail-empty">No handler set. Edit to configure.</div>
			) : (
				<>
					<div className="rule-card-detail">
						<span className="rule-card-detail-label">Handler</span>
						<span className="rule-card-detail-value">
							{handlerSummary(rule.handler_type, rule.handler_config)}
						</span>
					</div>
					{rule.handler_type === "reverse_proxy" && (
						<ReverseProxyDetails config={rule.handler_config as ReverseProxyConfig} />
					)}
					{rule.handler_type === "static_response" && (
						<StaticResponseDetails config={rule.handler_config as StaticResponseConfig} />
					)}
					{rule.handler_type === "file_server" && (
						<FileServerDetails config={rule.handler_config as FileServerConfig} />
					)}
				</>
			)}
			{onEdit && (
				<button
					type="button"
					className="btn btn-primary"
					style={{ alignSelf: "flex-end" }}
					onClick={onEdit}
					disabled={disabled}
				>
					Edit
				</button>
			)}
		</div>
	);
}

function ReverseProxyDetails({ config }: { config: ReverseProxyConfig }) {
	const details: { label: string; value: string }[] = [];
	if (config.tls_skip_verify) details.push({ label: "TLS", value: "Skip verify" });
	if (config.websocket_passthrough) details.push({ label: "WebSocket", value: "Enabled" });
	if (config.load_balancing.enabled) {
		const extra = config.load_balancing.upstreams.length;
		const strategyLabel = config.load_balancing.strategy.replace(/_/g, " ");
		details.push({
			label: "Load balancing",
			value: `${strategyLabel}${extra > 0 ? ` (+${extra} upstreams)` : ""}`,
		});
	}

	if (details.length === 0) return null;

	return (
		<>
			{details.map((d) => (
				<div key={d.label} className="rule-card-detail">
					<span className="rule-card-detail-label">{d.label}</span>
					<span className="rule-card-detail-value">{d.value}</span>
				</div>
			))}
		</>
	);
}

function StaticResponseDetails({ config }: { config: StaticResponseConfig }) {
	if (config.close) return null;

	const details: { label: string; value: string }[] = [];
	if (config.status_code) details.push({ label: "Status", value: config.status_code });
	if (config.body) {
		const preview = config.body.length > 60 ? `${config.body.slice(0, 60)}...` : config.body;
		details.push({ label: "Body", value: preview });
	}
	const headerKeys = Object.keys(config.headers || {});
	if (headerKeys.length > 0) {
		details.push({
			label: "Headers",
			value: headerKeys.map((k) => `${k}: ${(config.headers[k] || []).join(", ")}`).join("; "),
		});
	}

	if (details.length === 0) return null;

	return (
		<>
			{details.map((d) => (
				<div key={d.label} className="rule-card-detail">
					<span className="rule-card-detail-label">{d.label}</span>
					<span className="rule-card-detail-value">{d.value}</span>
				</div>
			))}
		</>
	);
}

function FileServerDetails({ config }: { config: FileServerConfig }) {
	const details: { label: string; value: string }[] = [{ label: "Root", value: config.root }];
	if (config.browse) details.push({ label: "Browse", value: "Enabled" });
	if (config.index_names?.length > 0) {
		details.push({ label: "Index files", value: config.index_names.join(", ") });
	}
	if (config.hide?.length > 0) {
		details.push({ label: "Hidden", value: config.hide.join(", ") });
	}

	return (
		<>
			{details.map((d) => (
				<div key={d.label} className="rule-card-detail">
					<span className="rule-card-detail-label">{d.label}</span>
					<span className="rule-card-detail-value">{d.value}</span>
				</div>
			))}
		</>
	);
}
