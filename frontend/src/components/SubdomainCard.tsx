import { cn } from "../cn";
import type { Subdomain } from "../types/domain";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { handlerSummary } from "./HandlerConfig";
import { Toggle } from "./Toggle";

interface Props {
	subdomain: Subdomain;
	domainName: string;
	onSelect: (subId: string) => void;
	onToggle: (subId: string, enabled: boolean) => void;
	onDelete: (subId: string) => void;
	saving: boolean;
}

const handlerLabels: Record<string, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
	none: "No Handler",
};

export default function SubdomainCard({
	subdomain,
	domainName,
	onSelect,
	onToggle,
	onDelete,
	saving,
}: Props) {
	const fullName = `${subdomain.name}.${domainName}`;
	const ruleCount = subdomain.rules.length;

	return (
		<div className={cn("card", !subdomain.enabled && "card-disabled")}>
			<div className="card-header">
				<button
					type="button"
					className="card-toggle"
					aria-label={`Open ${fullName}`}
					onClick={() => onSelect(subdomain.id)}
				>
					<div className="card-title">
						<span className="subdomain-card-name">{fullName}</span>
						<span
							className={cn(
								"rule-card-handler-badge",
								`handler-${subdomain.handler_type === "none" ? "none" : subdomain.handler_type}`,
							)}
						>
							{handlerLabels[subdomain.handler_type] ?? subdomain.handler_type}
						</span>
						{ruleCount > 0 && (
							<span className="subdomain-card-rule-count">
								{ruleCount} {ruleCount === 1 ? "rule" : "rules"}
							</span>
						)}
					</div>
				</button>
				<div className="card-actions">
					<Toggle
						value={subdomain.enabled}
						onChange={(enabled) => onToggle(subdomain.id, enabled)}
						disabled={saving}
						title={subdomain.enabled ? "Disable" : "Enable"}
						aria-label={subdomain.enabled ? "Disable subdomain" : "Enable subdomain"}
						stopPropagation
					/>
					<ConfirmDeleteButton
						onConfirm={() => onDelete(subdomain.id)}
						label={`Delete ${fullName}`}
						disabled={saving}
					/>
				</div>
			</div>
			{subdomain.handler_type !== "none" && (
				<div className="subdomain-card-summary">
					{handlerSummary(
						subdomain.handler_type as Parameters<typeof handlerSummary>[0],
						subdomain.handler_config,
					)}
				</div>
			)}
		</div>
	);
}
