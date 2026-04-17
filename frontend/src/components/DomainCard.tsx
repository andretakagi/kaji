import { cn } from "../cn";
import type { Domain, HandlerType } from "../types/domain";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { Toggle } from "./Toggle";

interface Props {
	domain: Domain;
	onNavigate: (id: string) => void;
	onToggleEnabled: (id: string, enabled: boolean) => void;
	onDelete: (id: string) => void;
	saving: boolean;
}

const handlerLabels: Record<HandlerType, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
};

function uniqueHandlerTypes(domain: Domain): HandlerType[] {
	const seen = new Set<HandlerType>();
	for (const rule of domain.rules) {
		seen.add(rule.handler_type);
	}
	return Array.from(seen);
}

export default function DomainCard({
	domain,
	onNavigate,
	onToggleEnabled,
	onDelete,
	saving,
}: Props) {
	const ruleCount = domain.rules.length;
	const handlers = uniqueHandlerTypes(domain);

	return (
		<div className={cn("domain-card", "card", !domain.enabled && "card-disabled")}>
			<div className="domain-card-header">
				<button
					type="button"
					className="domain-card-info"
					onClick={() => onNavigate(domain.id)}
					aria-label={domain.name}
				>
					<span className="domain-card-name">{domain.name}</span>
					<span className="domain-card-meta">
						<span className="domain-card-rule-count">
							{ruleCount} {ruleCount === 1 ? "rule" : "rules"}
						</span>
						{handlers.map((h) => (
							<span key={h} className="domain-card-handler-badge">
								{handlerLabels[h]}
							</span>
						))}
					</span>
				</button>
				<div className="domain-card-actions">
					<Toggle
						value={domain.enabled}
						onChange={(enabled) => onToggleEnabled(domain.id, enabled)}
						disabled={saving}
						title={domain.enabled ? "Disable" : "Enable"}
						aria-label={domain.enabled ? "Disable domain" : "Enable domain"}
						stopPropagation
					/>
					<ConfirmDeleteButton
						onConfirm={() => onDelete(domain.id)}
						label="Delete domain"
						disabled={saving}
					/>
				</div>
			</div>
		</div>
	);
}
