import type {
	DomainToggles,
	ErrorConfig,
	FileServerConfig,
	HandlerConfigValue,
	RedirectConfig,
	ReverseProxyConfig,
	StaticResponseConfig,
} from "../types/domain";
import { handlerLabels, pathMatchLabels } from "../types/domain";
import type { WizardData, WizardPath, WizardSubdomain } from "./DomainWizard";

interface Props {
	data: WizardData;
	onEditStep: (step: number) => void;
}

function activeToggles(toggles: DomainToggles): string[] {
	const active: string[] = [];
	if (toggles.force_https) active.push("Force HTTPS");
	if (toggles.compression) active.push("Compression");
	if (toggles.headers.response.enabled) active.push("Response Headers");
	if (toggles.basic_auth.enabled) active.push("Basic Auth");
	if (toggles.access_log) active.push("Access Log");
	if (toggles.ip_filtering.enabled) active.push("IP Filtering");
	if (toggles.error_pages.length > 0)
		active.push(
			`${toggles.error_pages.length} Error Page${toggles.error_pages.length !== 1 ? "s" : ""}`,
		);
	return active;
}

function TogglesList({ toggles }: { toggles: DomainToggles }) {
	const tags = activeToggles(toggles);
	if (tags.length === 0) return <DetailRow label="Toggles" value="None" />;
	return <DetailRow label="Toggles" value={tags.join(", ")} />;
}

function DetailRow({ label, value }: { label: string; value: string }) {
	return (
		<div className="wizard-review-detail-row">
			<span className="wizard-review-detail-label">{label}</span>
			<span className="wizard-review-detail-value">{value}</span>
		</div>
	);
}

function ReverseProxySummary({ config }: { config: ReverseProxyConfig }) {
	const rows: { label: string; value: string }[] = [{ label: "Upstream", value: config.upstream }];
	if (config.tls_skip_verify) rows.push({ label: "TLS", value: "Skip verify" });
	if (config.websocket_passthrough) rows.push({ label: "WebSocket", value: "Enabled" });
	if (config.load_balancing.enabled) {
		const extra = config.load_balancing.upstreams.length;
		const strategyLabel = config.load_balancing.strategy.replace(/_/g, " ");
		rows.push({
			label: "Load balancing",
			value: `${strategyLabel}${extra > 0 ? ` (+${extra} upstreams)` : ""}`,
		});
	}
	if (config.header_up.enabled) {
		const parts: string[] = [];
		if (config.header_up.host_override) parts.push(`Host: ${config.header_up.host_value}`);
		if (config.header_up.authorization) parts.push("Authorization");
		const custom = [...config.header_up.builtin, ...config.header_up.custom].filter((h) => h.key);
		if (custom.length > 0) parts.push(`${custom.length} custom`);
		if (parts.length > 0) rows.push({ label: "Req headers", value: parts.join(", ") });
	}

	return (
		<>
			{rows.map((d) => (
				<DetailRow key={d.label} label={d.label} value={d.value} />
			))}
		</>
	);
}

function StaticResponseSummary({ config }: { config: StaticResponseConfig }) {
	if (config.close) return <DetailRow label="Action" value="Close connection" />;

	const headerKeys = Object.keys(config.headers || {});

	return (
		<>
			{config.status_code && <DetailRow label="Status" value={config.status_code} />}
			{config.body && (
				<DetailRow
					label="Body"
					value={config.body.length > 60 ? `${config.body.slice(0, 60)}...` : config.body}
				/>
			)}
			{headerKeys.length > 0 && (
				<DetailRow
					label="Headers"
					value={headerKeys.map((k) => `${k}: ${(config.headers[k] || []).join(", ")}`).join("; ")}
				/>
			)}
		</>
	);
}

function RedirectSummary({ config }: { config: RedirectConfig }) {
	return (
		<>
			{config.target_url && <DetailRow label="Target" value={config.target_url} />}
			{config.status_code && <DetailRow label="Status" value={config.status_code} />}
			<DetailRow label="Preserve path" value={config.preserve_path ? "Yes" : "No"} />
		</>
	);
}

function FileServerSummary({ config }: { config: FileServerConfig }) {
	return (
		<>
			<DetailRow label="Root" value={config.root} />
			<DetailRow label="Browse" value={config.browse ? "Yes" : "No"} />
			{config.index_names?.length > 0 && (
				<DetailRow label="Index files" value={config.index_names.join(", ")} />
			)}
			{config.hide?.length > 0 && <DetailRow label="Hidden" value={config.hide.join(", ")} />}
		</>
	);
}

function HandlerBadge({ type }: { type: string }) {
	return (
		<span className={`rule-card-handler-badge handler-${type}`}>
			{handlerLabels[type as keyof typeof handlerLabels] ?? type.replace(/_/g, " ")}
		</span>
	);
}

function ErrorSummary({ config }: { config: ErrorConfig }) {
	return (
		<>
			<DetailRow label="Status" value={config.status_code} />
			{config.message && <DetailRow label="Message" value={config.message} />}
		</>
	);
}

function HandlerDetails({ type, config }: { type: string; config: HandlerConfigValue }) {
	if (type === "reverse_proxy")
		return <ReverseProxySummary config={config as ReverseProxyConfig} />;
	if (type === "static_response")
		return <StaticResponseSummary config={config as StaticResponseConfig} />;
	if (type === "redirect") return <RedirectSummary config={config as RedirectConfig} />;
	if (type === "file_server") return <FileServerSummary config={config as FileServerConfig} />;
	if (type === "error") return <ErrorSummary config={config as ErrorConfig} />;
	return null;
}

function SubdomainReviewItem({ sub, domainName }: { sub: WizardSubdomain; domainName: string }) {
	return (
		<div className="wizard-review-item">
			<div className="wizard-review-item-title">
				<span>
					{sub.prefix}.{domainName}
				</span>
				<HandlerBadge type={sub.rule.handlerType} />
			</div>
			<div className="wizard-review-details">
				{sub.rule.handlerType !== "none" && (
					<HandlerDetails type={sub.rule.handlerType} config={sub.rule.handlerConfig} />
				)}
				<TogglesList toggles={sub.toggles} />
				{sub.paths.length > 0 && (
					<DetailRow
						label="Paths"
						value={`${sub.paths.length} path${sub.paths.length !== 1 ? "s" : ""}`}
					/>
				)}
			</div>
		</div>
	);
}

function formatPathLabel(target: string, path: WizardPath): string {
	return `${target}${path.matchValue}`;
}

function PathMatchBadge({ match }: { match: WizardPath["pathMatch"] }) {
	return <span className={`path-match-badge path-match-${match}`}>{pathMatchLabels[match]}</span>;
}

export default function WizardReview({ data, onEditStep }: Props) {
	const allPaths: { targetLabel: string; path: WizardPath }[] = [];
	for (const path of data.rootPaths) {
		allPaths.push({ targetLabel: data.name, path });
	}
	for (const sub of data.subdomains) {
		for (const path of sub.paths) {
			allPaths.push({ targetLabel: `${sub.prefix}.${data.name}`, path });
		}
	}

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
					<h4>Domain Rule</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(1)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-item">
					<div className="wizard-review-item-title">
						<span>{data.name}</span>
						<HandlerBadge type={data.rootRule.handlerType} />
					</div>
					<div className="wizard-review-details">
						{data.rootRule.handlerType !== "none" && (
							<HandlerDetails
								type={data.rootRule.handlerType}
								config={data.rootRule.handlerConfig}
							/>
						)}
						<TogglesList toggles={data.toggles} />
					</div>
				</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Subdomains</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(2)}>
						Edit
					</button>
				</div>
				{data.subdomains.length > 0 ? (
					<div className="wizard-review-item-list">
						{data.subdomains.map((sub) => (
							<SubdomainReviewItem key={sub.key} sub={sub} domainName={data.name} />
						))}
					</div>
				) : (
					<span className="text-muted">No subdomains</span>
				)}
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Paths</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(3)}>
						Edit
					</button>
				</div>
				{allPaths.length > 0 ? (
					<div className="wizard-review-item-list">
						{allPaths.map((entry) => (
							<div key={`${entry.targetLabel}-${entry.path.key}`} className="wizard-review-item">
								<div className="wizard-review-item-title">
									<span>{formatPathLabel(entry.targetLabel, entry.path)}</span>
									<div className="wizard-review-item-badges">
										<PathMatchBadge match={entry.path.pathMatch} />
										<HandlerBadge type={entry.path.handlerType} />
									</div>
								</div>
								<div className="wizard-review-details">
									<HandlerDetails type={entry.path.handlerType} config={entry.path.handlerConfig} />
									{entry.path.toggleOverrides && <DetailRow label="Overrides" value="Yes" />}
								</div>
							</div>
						))}
					</div>
				) : (
					<span className="text-muted">No paths</span>
				)}
			</div>
		</div>
	);
}
