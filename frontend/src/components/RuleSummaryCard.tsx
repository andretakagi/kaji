import type { ReactNode } from "react";
import { cn } from "../cn";
import type { Rule, RuleHandlerType } from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { handlerSummary } from "./HandlerConfig";

interface Props {
	title: ReactNode;
	rule: Rule;
	onEdit: () => void;
	disabled?: boolean;
	actions?: ReactNode;
}

const labels: Record<RuleHandlerType, string> = {
	none: "No Handler",
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
};

export default function RuleSummaryCard({ title, rule, onEdit, disabled, actions }: Props) {
	const titleNode = (
		<>
			<span className="rule-card-match">{title}</span>
			<span className={cn("rule-card-handler-badge", `handler-${rule.handler_type}`)}>
				{labels[rule.handler_type]}
			</span>
		</>
	);
	return (
		<CollapsibleCard title={titleNode} actions={actions}>
			<div className="rule-card-body">
				{rule.handler_type === "none" ? (
					<div className="rule-card-detail-empty">No handler set. Edit to configure.</div>
				) : (
					<div className="rule-card-detail">
						<span className="rule-card-detail-label">Handler</span>
						<span className="rule-card-detail-value">
							{handlerSummary(rule.handler_type, rule.handler_config)}
						</span>
					</div>
				)}
				<button
					type="button"
					className="btn btn-primary"
					style={{ alignSelf: "flex-end" }}
					onClick={onEdit}
					disabled={disabled}
				>
					Edit
				</button>
			</div>
		</CollapsibleCard>
	);
}
