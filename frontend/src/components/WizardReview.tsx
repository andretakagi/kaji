import type {
	DomainToggles,
	FileServerConfig,
	RedirectConfig,
	ReverseProxyConfig,
	StaticResponseConfig,
} from "../types/domain";
import type { WizardData, WizardRule } from "./DomainWizard";

interface Props {
	data: WizardData;
	onEditStep: (step: number) => void;
}

function toggleSummary(toggles: DomainToggles): string[] {
	const active: string[] = [];
	if (toggles.force_https) active.push("Force HTTPS");
	if (toggles.compression) active.push("Compression");
	if (toggles.headers.response.enabled) active.push("Response Headers");
	if (toggles.basic_auth.enabled) active.push("Basic Auth");
	if (toggles.access_log) active.push("Access Log");
	if (toggles.ip_filtering.enabled) active.push("IP Filtering");
	return active;
}

function ruleMatchLabel(rule: WizardRule, domainName: string): string {
	return `${domainName}${rule.matchValue}`;
}

function ReverseProxySummary({ config }: { config: ReverseProxyConfig }) {
	const details: { label: string; value: string }[] = [
		{ label: "Upstream", value: config.upstream },
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
	if (config.request_headers.enabled) {
		const parts: string[] = [];
		if (config.request_headers.host_override)
			parts.push(`Host: ${config.request_headers.host_value}`);
		if (config.request_headers.authorization) parts.push("Authorization");
		const custom = [...config.request_headers.builtin, ...config.request_headers.custom].filter(
			(h) => h.key,
		);
		if (custom.length > 0) parts.push(`${custom.length} custom`);
		if (parts.length > 0) details.push({ label: "Req headers", value: parts.join(", ") });
	}

	return (
		<div className="wizard-review-static-details">
			{details.map((d) => (
				<div key={d.label} className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">{d.label}</span>
					<span className="wizard-review-detail-value">{d.value}</span>
				</div>
			))}
		</div>
	);
}

function StaticResponseSummary({ config }: { config: StaticResponseConfig }) {
	if (config.close) return <span className="wizard-review-upstream">Close connection</span>;

	const headerKeys = Object.keys(config.headers || {});

	return (
		<div className="wizard-review-static-details">
			{config.status_code && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Status</span>
					<span className="wizard-review-detail-value">{config.status_code}</span>
				</div>
			)}
			{config.body && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Body</span>
					<span className="wizard-review-detail-value">
						{config.body.length > 60 ? `${config.body.slice(0, 60)}...` : config.body}
					</span>
				</div>
			)}
			{headerKeys.length > 0 && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Headers</span>
					<span className="wizard-review-detail-value">
						{headerKeys.map((k) => `${k}: ${(config.headers[k] || []).join(", ")}`).join("; ")}
					</span>
				</div>
			)}
		</div>
	);
}

function RedirectSummary({ config }: { config: RedirectConfig }) {
	return (
		<div className="wizard-review-static-details">
			{config.target_url && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Target</span>
					<span className="wizard-review-detail-value">{config.target_url}</span>
				</div>
			)}
			{config.status_code && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Status</span>
					<span className="wizard-review-detail-value">{config.status_code}</span>
				</div>
			)}
			<div className="wizard-review-detail-row">
				<span className="wizard-review-detail-label">Preserve path</span>
				<span className="wizard-review-detail-value">{config.preserve_path ? "Yes" : "No"}</span>
			</div>
		</div>
	);
}

function FileServerSummary({ config }: { config: FileServerConfig }) {
	return (
		<div className="wizard-review-static-details">
			<div className="wizard-review-detail-row">
				<span className="wizard-review-detail-label">Root</span>
				<span className="wizard-review-detail-value">{config.root}</span>
			</div>
			<div className="wizard-review-detail-row">
				<span className="wizard-review-detail-label">Browse</span>
				<span className="wizard-review-detail-value">{config.browse ? "Yes" : "No"}</span>
			</div>
			{config.index_names?.length > 0 && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Index files</span>
					<span className="wizard-review-detail-value">{config.index_names.join(", ")}</span>
				</div>
			)}
			{config.hide?.length > 0 && (
				<div className="wizard-review-detail-row">
					<span className="wizard-review-detail-label">Hidden</span>
					<span className="wizard-review-detail-value">{config.hide.join(", ")}</span>
				</div>
			)}
		</div>
	);
}

export default function WizardReview({ data, onEditStep }: Props) {
	const activeTags = toggleSummary(data.toggles);

	const rootHandlerLabel =
		data.rootRule.handlerType !== "none" ? data.rootRule.handlerType.replace("_", " ") : null;

	const rootIsProxy = data.rootRule.handlerType === "reverse_proxy";
	const rootIsStatic = data.rootRule.handlerType === "static_response";
	const rootIsRedirect = data.rootRule.handlerType === "redirect";
	const rootIsFileServer = data.rootRule.handlerType === "file_server";

	return (
		<div className="wizard-review">
			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Domain</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(0)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">{data.name}</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Toggles</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(1)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{activeTags.length > 0 ? (
						<div className="wizard-review-tags">
							{activeTags.map((tag) => (
								<span key={tag} className="wizard-review-tag">
									{tag}
								</span>
							))}
						</div>
					) : (
						<span className="text-muted">None enabled</span>
					)}
				</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Root Rule</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(2)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{rootHandlerLabel ? (
						<div className="wizard-review-rule-detail">
							<span className={`rule-card-handler-badge handler-${data.rootRule.handlerType}`}>
								{rootHandlerLabel}
							</span>
							{rootIsProxy && (
								<ReverseProxySummary config={data.rootRule.handlerConfig as ReverseProxyConfig} />
							)}
							{rootIsStatic && (
								<StaticResponseSummary
									config={data.rootRule.handlerConfig as StaticResponseConfig}
								/>
							)}
							{rootIsRedirect && (
								<RedirectSummary config={data.rootRule.handlerConfig as RedirectConfig} />
							)}
							{rootIsFileServer && (
								<FileServerSummary config={data.rootRule.handlerConfig as FileServerConfig} />
							)}
						</div>
					) : (
						<span className="text-muted">None</span>
					)}
				</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Rules</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(3)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{data.rules.length > 0 ? (
						<div className="wizard-review-rule-list">
							{data.rules.map((rule) => (
								<div key={rule.key} className="wizard-review-rule-item">
									<span className="wizard-review-rule-match">
										{ruleMatchLabel(rule, data.name)}
									</span>
									<span className={`rule-card-handler-badge handler-${rule.handlerType}`}>
										{rule.handlerType.replace("_", " ")}
									</span>
									{rule.handlerType === "reverse_proxy" && (
										<ReverseProxySummary config={rule.handlerConfig as ReverseProxyConfig} />
									)}
									{rule.handlerType === "static_response" && (
										<StaticResponseSummary config={rule.handlerConfig as StaticResponseConfig} />
									)}
									{rule.handlerType === "redirect" && (
										<RedirectSummary config={rule.handlerConfig as RedirectConfig} />
									)}
									{rule.handlerType === "file_server" && (
										<FileServerSummary config={rule.handlerConfig as FileServerConfig} />
									)}
									{rule.toggleOverrides && <span className="wizard-review-tag">overrides</span>}
								</div>
							))}
						</div>
					) : (
						<span className="text-muted">No additional rules</span>
					)}
				</div>
			</div>
		</div>
	);
}
