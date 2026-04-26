import { useState } from "react";
import { cn } from "../cn";
import type {
	DomainToggles,
	FileServerConfig,
	Path,
	ReverseProxyConfig,
	RuleHandlerType,
	StaticResponseConfig,
	UpdatePathRequest,
} from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { handlerSummary } from "./HandlerConfig";
import PathForm from "./PathForm";
import { FileServerDetails, ReverseProxyDetails, StaticResponseDetails } from "./RuleSummaryCard";
import { Toggle } from "./Toggle";

interface PathCardProps {
	path: Path;
	domainName: string;
	parentToggles: DomainToggles;
	onUpdate: (req: UpdatePathRequest) => Promise<void>;
	onDelete: () => void;
	onToggle: (enabled: boolean) => void;
	saving: boolean;
}

const handlerLabels: Record<RuleHandlerType, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
	none: "No Handler",
};

function formatPath(p: Path): string {
	if (p.path_match === "exact") return `Path: ${p.match_value} (exact)`;
	if (p.path_match === "prefix") return `Path: ${p.match_value}*`;
	if (p.path_match === "regex") return `Path: ~${p.match_value}`;
	return p.match_value;
}

function overrideSummary(overrides: DomainToggles | null | undefined): string | null {
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

export default function PathCard({
	path,
	domainName,
	parentToggles,
	onUpdate,
	onDelete,
	onToggle,
	saving,
}: PathCardProps) {
	const [editing, setEditing] = useState(false);

	const title = (
		<>
			<span className="rule-card-match">{formatPath(path)}</span>
			<span className={cn("rule-card-handler-badge", `handler-${path.rule.handler_type}`)}>
				{handlerLabels[path.rule.handler_type]}
			</span>
		</>
	);

	const actions = (
		<>
			<Toggle
				value={path.enabled}
				onChange={onToggle}
				disabled={saving}
				title={path.enabled ? "Disable" : "Enable"}
				aria-label={path.enabled ? "Disable path" : "Enable path"}
				stopPropagation
			/>
			<ConfirmDeleteButton onConfirm={onDelete} label="Delete path" disabled={saving} />
		</>
	);

	return (
		<CollapsibleCard
			title={title}
			actions={actions}
			disabled={!path.enabled && !editing}
			forceExpanded={editing}
			ariaLabel={`${formatPath(path)} path`}
		>
			{editing ? (
				<PathForm
					domainName={domainName}
					parentToggles={parentToggles}
					initial={path}
					inline
					onSubmit={async (req) => {
						await onUpdate(req as UpdatePathRequest);
						setEditing(false);
					}}
					onCancel={() => setEditing(false)}
				/>
			) : (
				<PathCardBody path={path} onEdit={() => setEditing(true)} />
			)}
		</CollapsibleCard>
	);
}

function PathCardBody({ path, onEdit }: { path: Path; onEdit: () => void }) {
	const summary = handlerSummary(path.rule.handler_type, path.rule.handler_config);
	const overrides = overrideSummary(path.toggle_overrides);

	return (
		<div className="rule-card-body">
			{path.rule.handler_type === "none" ? (
				<div className="rule-card-detail-empty">No handler set. Edit to configure.</div>
			) : (
				<>
					<div className="rule-card-detail">
						<span className="rule-card-detail-label">Handler</span>
						<span className="rule-card-detail-value">{summary}</span>
					</div>
					{path.rule.handler_type === "reverse_proxy" && (
						<ReverseProxyDetails config={path.rule.handler_config as ReverseProxyConfig} />
					)}
					{path.rule.handler_type === "static_response" && (
						<StaticResponseDetails config={path.rule.handler_config as StaticResponseConfig} />
					)}
					{path.rule.handler_type === "file_server" && (
						<FileServerDetails config={path.rule.handler_config as FileServerConfig} />
					)}
				</>
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
