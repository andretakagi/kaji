import { useUpstreamStatus } from "../contexts/UpstreamStatusContext";
import type {
	ErrorConfig,
	FileServerConfig,
	RedirectConfig,
	ReverseProxyConfig,
	Rule,
	StaticResponseConfig,
} from "../types/domain";

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
					{rule.handler_type === "reverse_proxy" && (
						<ReverseProxyDetails config={rule.handler_config as ReverseProxyConfig} />
					)}
					{rule.handler_type === "static_response" && (
						<StaticResponseDetails config={rule.handler_config as StaticResponseConfig} />
					)}
					{rule.handler_type === "redirect" && (
						<RedirectDetails config={rule.handler_config as RedirectConfig} />
					)}
					{rule.handler_type === "file_server" && (
						<FileServerDetails config={rule.handler_config as FileServerConfig} />
					)}
					{rule.handler_type === "error" && (
						<ErrorDetails config={rule.handler_config as ErrorConfig} />
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

function DetailList({ details }: { details: { label: string; value: string }[] }) {
	if (details.length === 0) return null;
	return (
		<dl className="rule-card-detail-list">
			{details.map((d) => (
				<div key={d.label} className="rule-card-detail">
					<dt className="rule-card-detail-label">{d.label}</dt>
					<dd className="rule-card-detail-value">{d.value}</dd>
				</div>
			))}
		</dl>
	);
}

function ReverseProxyDetails({ config }: { config: ReverseProxyConfig }) {
	const { getUpstreamState } = useUpstreamStatus();
	const details: { label: string; value: string }[] = [];

	if (config.load_balancing.enabled) {
		const strategyLabel = config.load_balancing.strategy.replace(/_/g, " ");
		details.push({ label: "Strategy", value: strategyLabel });
	}

	if (config.tls_skip_verify) details.push({ label: "TLS", value: "Skip verify" });
	if (config.websocket_passthrough) details.push({ label: "WebSocket", value: "Enabled" });
	if (config.strip_path_prefix) {
		details.push({ label: "Strip prefix", value: config.strip_path_prefix });
	}
	if (config.prepend_path_prefix) {
		details.push({ label: "Prepend prefix", value: config.prepend_path_prefix });
	}
	if (config.header_up.enabled) details.push({ label: "Header up", value: "Enabled" });
	if (config.header_down.enabled) details.push({ label: "Header down", value: "Enabled" });

	const allUpstreams = config.load_balancing.enabled
		? [config.upstream, ...config.load_balancing.upstreams].filter(Boolean)
		: [config.upstream].filter(Boolean);

	return (
		<>
			<div className="upstream-status-list">
				<dt className="rule-card-detail-label">
					{allUpstreams.length === 1 ? "Upstream" : "Upstreams"}
				</dt>
				<dd className="upstream-status-items">
					{allUpstreams.length === 0 ? (
						<span className="rule-card-detail-value">...</span>
					) : (
						allUpstreams.map((addr) => (
							<div key={addr} className="upstream-status-item">
								<span
									className={`upstream-dot upstream-dot-${getUpstreamState(addr)}`}
									aria-hidden="true"
								/>
								<span className="rule-card-detail-value">{addr}</span>
							</div>
						))
					)}
				</dd>
			</div>
			<DetailList details={details} />
		</>
	);
}

function StaticResponseDetails({ config }: { config: StaticResponseConfig }) {
	const details: { label: string; value: string }[] = [];
	if (config.close) {
		details.push({ label: "Mode", value: "Close connection" });
		return <DetailList details={details} />;
	}
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
	return <DetailList details={details} />;
}

function RedirectDetails({ config }: { config: RedirectConfig }) {
	const details: { label: string; value: string }[] = [
		{ label: "Target", value: config.target_url || "..." },
		{ label: "Status", value: String(config.status_code) },
	];
	if (config.preserve_path) details.push({ label: "Preserve path", value: "Enabled" });
	return <DetailList details={details} />;
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
	return <DetailList details={details} />;
}

function ErrorDetails({ config }: { config: ErrorConfig }) {
	const details: { label: string; value: string }[] = [
		{ label: "Status", value: config.status_code || "..." },
	];
	if (config.message) details.push({ label: "Message", value: config.message });
	return <DetailList details={details} />;
}
