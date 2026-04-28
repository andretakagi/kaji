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
	const details: { label: string; value: string }[] = [
		{ label: "Upstream", value: config.upstream || "..." },
	];
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
	return <DetailList details={details} />;
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
