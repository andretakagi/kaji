import { useEffect, useRef, useState } from "react";
import { useDomain } from "../hooks/useDomain";
import { useFormToggle } from "../hooks/useFormToggle";
import type { DomainToggles, UpdateRuleRequest } from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import RuleCard from "./RuleCard";
import RuleForm from "./RuleForm";
import { SectionHeader } from "./SectionHeader";
import { Toggle } from "./Toggle";

interface Props {
	domain: { id: string; name: string; enabled: boolean };
	onToggleEnabled: (id: string, enabled: boolean) => void;
	onDelete: (id: string) => void;
	saving: boolean;
}

export default function DomainCard({
	domain: domainSummary,
	onToggleEnabled,
	onDelete,
	saving,
}: Props) {
	const {
		domain,
		error,
		setError,
		saving: detailSaving,
		feedback,
		handleUpdateDomain,
		handleCreateRule,
		handleUpdateRule,
		handleDeleteRule,
		handleToggleRule,
	} = useDomain(domainSummary.id);

	const [localToggles, setLocalToggles] = useState<DomainToggles | null>(null);
	const lastSyncedToggles = useRef<string>("");
	const ruleForm = useFormToggle();

	useEffect(() => {
		if (!domain.id) return;
		const serialized = JSON.stringify(domain.toggles);
		if (serialized !== lastSyncedToggles.current) {
			lastSyncedToggles.current = serialized;
			setLocalToggles(domain.toggles);
		}
	}, [domain.id, domain.toggles]);

	const togglesDirty =
		localToggles !== null && JSON.stringify(localToggles) !== lastSyncedToggles.current;

	async function handleSaveToggles() {
		if (!localToggles) return;
		await handleUpdateDomain({ name: domain.name, toggles: localToggles });
	}

	function handleDiscardToggles() {
		setLocalToggles(domain.toggles);
		lastSyncedToggles.current = JSON.stringify(domain.toggles);
	}

	const hasRootRule = domain.rules.some((r) => r.match_type === "");

	const title = (
		<>
			<span className="domain-card-name">{domainSummary.name}</span>
			<span className="domain-card-meta">
				{domain.rules.length > 0 && (
					<span className="domain-card-rule-count">
						{domain.rules.length} {domain.rules.length === 1 ? "rule" : "rules"}
					</span>
				)}
			</span>
		</>
	);

	const actions = (
		<>
			<Toggle
				value={domainSummary.enabled}
				onChange={(enabled) => onToggleEnabled(domainSummary.id, enabled)}
				disabled={saving}
				title={domainSummary.enabled ? "Disable" : "Enable"}
				aria-label={domainSummary.enabled ? "Disable domain" : "Enable domain"}
				stopPropagation
			/>
			<ConfirmDeleteButton
				onConfirm={() => onDelete(domainSummary.id)}
				label="Delete domain"
				disabled={saving}
			/>
		</>
	);

	const isSaving = saving || detailSaving;

	return (
		<CollapsibleCard title={title} actions={actions} ariaLabel={domainSummary.name}>
			<ErrorAlert message={error} onDismiss={() => setError("")} />
			<Feedback msg={feedback.msg} type={feedback.type} />

			{localToggles && (
				<section className="domain-detail-section">
					<SectionHeader title="Domain Toggles">
						{togglesDirty && (
							<div className="domain-detail-toggle-actions">
								<button
									type="button"
									className="btn btn-ghost"
									onClick={handleDiscardToggles}
									disabled={isSaving}
								>
									Discard
								</button>
								<button
									type="button"
									className="btn btn-primary"
									onClick={handleSaveToggles}
									disabled={isSaving}
								>
									{isSaving ? "Saving..." : "Save"}
								</button>
							</div>
						)}
					</SectionHeader>
					<DomainToggleGrid
						toggles={localToggles}
						onUpdate={(key, value) =>
							setLocalToggles((prev) => (prev ? { ...prev, [key]: value } : prev))
						}
						idPrefix={`domain-${domainSummary.id}`}
						domain={domainSummary.name}
					/>
				</section>
			)}

			<section className="domain-detail-section">
				<SectionHeader title="Rules" />

				{domain.rules.length === 0 && !ruleForm.visible ? (
					<div className="empty-state">
						No rules yet. Rules define how requests to this domain are handled.
					</div>
				) : (
					<div className="rule-list">
						{domain.rules.map((rule) => (
							<RuleCard
								key={rule.id}
								rule={rule}
								domainName={domain.name}
								domainToggles={domain.toggles}
								hasRootRule={hasRootRule}
								onToggle={handleToggleRule}
								onDelete={handleDeleteRule}
								onUpdate={(ruleId, req) => handleUpdateRule(ruleId, req as UpdateRuleRequest)}
								saving={isSaving}
							/>
						))}
					</div>
				)}

				{ruleForm.visible ? (
					<RuleForm
						domainName={domain.name}
						domainToggles={domain.toggles}
						hasRootRule={hasRootRule}
						onSubmit={async (req) => {
							await handleCreateRule(req);
							ruleForm.close();
						}}
						onCancel={ruleForm.close}
					/>
				) : (
					<button
						type="button"
						className="btn btn-primary"
						onClick={ruleForm.open}
						disabled={isSaving}
					>
						Add Rule
					</button>
				)}
			</section>
		</CollapsibleCard>
	);
}
