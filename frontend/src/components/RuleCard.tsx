import { useState } from "react";
import { cn } from "../cn";
import type {
	DomainToggles,
	FileServerConfig,
	ReverseProxyConfig,
	Rule,
	StaticResponseConfig,
	UpdateRuleRequest,
} from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { handlerSummary } from "./HandlerConfig";
import RuleForm from "./RuleForm";
import { Toggle } from "./Toggle";

interface Props {
	rule: Rule;
	domainName: string;
	domainToggles?: DomainToggles;
	hasRootRule?: boolean;
	onToggle: (ruleId: string, enabled: boolean) => void;
	onDelete: (ruleId: string) => void;
	onUpdate: (ruleId: string, req: UpdateRuleRequest) => Promise<void>;
	saving: boolean;
}

const handlerLabels: Record<string, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
	"": "None",
};

function formatMatch(rule: Rule): string {
	if (rule.match_type === "") return "Root";
	if (rule.match_type === "subdomain") return `Subdomain: ${rule.match_value}`;
	if (rule.match_type === "path") {
		if (rule.path_match === "exact") return `Path: ${rule.match_value} (exact)`;
		if (rule.path_match === "prefix") return `Path: ${rule.match_value}*`;
		if (rule.path_match === "regex") return `Path: ~${rule.match_value}`;
	}
	return rule.match_value || "Unknown";
}

function overrideSummary(overrides: DomainToggles | null): string | null {
	if (!overrides) return null;
	const parts: string[] = [];
	if (overrides.force_https) parts.push("HTTPS");
	if (overrides.compression) parts.push("Compression");
	if (overrides.basic_auth.enabled) parts.push("Basic Auth");
	if (overrides.access_log) parts.push("Access Log");
	if (overrides.ip_filtering.enabled) parts.push("IP Filtering");
	if (overrides.headers.response.enabled) parts.push("Response Headers");
	return parts.length > 0 ? parts.join(", ") : null;
}

export default function RuleCard({
	rule,
	domainName,
	domainToggles,
	hasRootRule,
	onToggle,
	onDelete,
	onUpdate,
	saving,
}: Props) {
	const [editing, setEditing] = useState(false);

	const title = (
		<>
			<span className="rule-card-match">{formatMatch(rule)}</span>
			<span className={cn("rule-card-handler-badge", `handler-${rule.handler_type}`)}>
				{handlerLabels[rule.handler_type]}
			</span>
		</>
	);

	const actions = (
		<>
			<Toggle
				value={rule.enabled}
				onChange={(enabled) => onToggle(rule.id, enabled)}
				disabled={saving}
				title={rule.enabled ? "Disable" : "Enable"}
				aria-label={rule.enabled ? "Disable rule" : "Enable rule"}
				stopPropagation
			/>
			<ConfirmDeleteButton
				onConfirm={() => onDelete(rule.id)}
				label="Delete rule"
				disabled={saving}
			/>
		</>
	);

	return (
		<CollapsibleCard
			title={title}
			actions={actions}
			disabled={!rule.enabled && !editing}
			forceExpanded={editing}
			ariaLabel={`${formatMatch(rule)} rule`}
		>
			{editing ? (
				<RuleForm
					domainName={domainName}
					domainToggles={domainToggles}
					initial={rule}
					hasRootRule={hasRootRule}
					inline
					onSubmit={async (req) => {
						await onUpdate(rule.id, req as UpdateRuleRequest);
						setEditing(false);
					}}
					onCancel={() => setEditing(false)}
				/>
			) : (
				<RuleCardBody rule={rule} onEdit={() => setEditing(true)} />
			)}
		</CollapsibleCard>
	);
}

function RuleCardBody({ rule, onEdit }: { rule: Rule; onEdit: () => void }) {
	const summary = handlerSummary(rule.handler_type, rule.handler_config);
	const overrides = overrideSummary(rule.toggle_overrides);

	return (
		<div className="rule-card-body">
			<div className="rule-card-detail">
				<span className="rule-card-detail-label">Handler</span>
				<span className="rule-card-detail-value">{summary}</span>
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
			{overrides && (
				<div className="rule-card-detail">
					<span className="rule-card-detail-label">Toggle overrides</span>
					<span className="rule-card-detail-value">{overrides}</span>
				</div>
			)}
			<button
				type="button"
				className="btn btn-primary"
				style={{ alignSelf: "flex-end" }}
				onClick={onEdit}
			>
				Edit
			</button>
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
