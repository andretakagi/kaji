import { useEffect, useRef, useState } from "react";
import { useFormToggle } from "../hooks/useFormToggle";
import type {
	CreateRuleRequest,
	DomainToggles,
	HandlerType,
	Subdomain,
	UpdateRuleRequest,
	UpdateSubdomainRequest,
} from "../types/domain";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { handlerSummary } from "./HandlerConfig";
import RuleCard from "./RuleCard";
import RuleForm from "./RuleForm";
import { SectionHeader } from "./SectionHeader";
import { Toggle } from "./Toggle";

interface Props {
	subdomain: Subdomain;
	domainName: string;
	onBack: () => void;
	onUpdate: (subId: string, req: UpdateSubdomainRequest) => Promise<void>;
	onDelete: (subId: string) => void;
	onToggle: (subId: string, enabled: boolean) => void;
	onCreateRule: (subId: string, req: CreateRuleRequest) => Promise<void>;
	onUpdateRule: (subId: string, ruleId: string, req: UpdateRuleRequest) => Promise<void>;
	onDeleteRule: (subId: string, ruleId: string) => void;
	onToggleRule: (subId: string, ruleId: string, enabled: boolean) => void;
	saving: boolean;
}

export default function SubdomainDetail({
	subdomain,
	domainName,
	onBack,
	onUpdate,
	onDelete,
	onToggle,
	onCreateRule,
	onUpdateRule,
	onDeleteRule,
	onToggleRule,
	saving,
}: Props) {
	const fullName = `${subdomain.name}.${domainName}`;
	const ruleForm = useFormToggle();

	const [localToggles, setLocalToggles] = useState<DomainToggles | null>(null);
	const lastSyncedToggles = useRef<string>("");

	useEffect(() => {
		const serialized = JSON.stringify(subdomain.toggles);
		if (serialized !== lastSyncedToggles.current) {
			lastSyncedToggles.current = serialized;
			setLocalToggles(subdomain.toggles);
		}
	}, [subdomain.toggles]);

	const togglesDirty =
		localToggles !== null && JSON.stringify(localToggles) !== lastSyncedToggles.current;

	async function handleSaveToggles() {
		if (!localToggles) return;
		await onUpdate(subdomain.id, {
			name: subdomain.name,
			handler_type: subdomain.handler_type,
			handler_config: subdomain.handler_config,
			toggles: localToggles,
		});
	}

	function handleDiscardToggles() {
		setLocalToggles(subdomain.toggles);
		lastSyncedToggles.current = JSON.stringify(subdomain.toggles);
	}

	const ruleCount = subdomain.rules.length;

	return (
		<div className="domain-detail">
			<div className="domain-detail-header">
				<button type="button" className="btn btn-ghost domain-detail-back" onClick={onBack}>
					&larr; Back
				</button>
				<div className="domain-detail-title-row">
					<h1 className="domain-detail-name">{fullName}</h1>
					<div className="domain-detail-actions">
						<Toggle
							value={subdomain.enabled}
							onChange={(enabled) => onToggle(subdomain.id, enabled)}
							disabled={saving}
							title={subdomain.enabled ? "Disable subdomain" : "Enable subdomain"}
							aria-label={subdomain.enabled ? "Disable subdomain" : "Enable subdomain"}
						/>
						<ConfirmDeleteButton
							onConfirm={() => onDelete(subdomain.id)}
							label={`Delete ${fullName} and all ${ruleCount} ${ruleCount === 1 ? "rule" : "rules"}`}
							disabled={saving}
						/>
					</div>
				</div>
			</div>

			{subdomain.handler_type !== "none" && (
				<section className="domain-detail-section">
					<SectionHeader title="Handler" />
					<div className="subdomain-handler-info">
						<span className="subdomain-handler-label">
							{subdomain.handler_type.replace(/_/g, " ")}
						</span>
						<span className="subdomain-handler-value">
							{handlerSummary(subdomain.handler_type as HandlerType, subdomain.handler_config)}
						</span>
					</div>
				</section>
			)}

			{localToggles && (
				<section className="domain-detail-section">
					<SectionHeader title="Toggles">
						{togglesDirty && (
							<div className="domain-detail-toggle-actions">
								<button
									type="button"
									className="btn btn-ghost"
									onClick={handleDiscardToggles}
									disabled={saving}
								>
									Discard
								</button>
								<button
									type="button"
									className="btn btn-primary"
									onClick={handleSaveToggles}
									disabled={saving}
								>
									{saving ? "Saving..." : "Save"}
								</button>
							</div>
						)}
					</SectionHeader>
					<DomainToggleGrid
						toggles={localToggles}
						onUpdate={(key, value) =>
							setLocalToggles((prev) => (prev ? { ...prev, [key]: value } : prev))
						}
						idPrefix={`subdomain-${subdomain.id}`}
						domain={fullName}
					/>
				</section>
			)}

			<section className="domain-detail-section">
				<SectionHeader title="Rules">
					<button
						type="button"
						className="btn btn-primary"
						onClick={ruleForm.toggle}
						disabled={saving}
					>
						{ruleForm.visible ? "Cancel" : "Add Rule"}
					</button>
				</SectionHeader>

				{ruleForm.visible && (
					<RuleForm
						domainName={fullName}
						hasRootRule
						onSubmit={async (req) => {
							await onCreateRule(subdomain.id, req as CreateRuleRequest);
							ruleForm.close();
						}}
						onCancel={ruleForm.close}
					/>
				)}

				{subdomain.rules.length === 0 ? (
					<div className="empty-state">
						No rules yet. Rules define how requests to this subdomain are handled.
					</div>
				) : (
					<div className="rule-list">
						{subdomain.rules.map((rule) => (
							<RuleCard
								key={rule.id}
								rule={rule}
								domainName={fullName}
								onToggle={(ruleId, enabled) => onToggleRule(subdomain.id, ruleId, enabled)}
								onDelete={(ruleId) => onDeleteRule(subdomain.id, ruleId)}
								onUpdate={(ruleId, req) =>
									onUpdateRule(subdomain.id, ruleId, req as UpdateRuleRequest)
								}
								saving={saving}
							/>
						))}
					</div>
				)}
			</section>
		</div>
	);
}
