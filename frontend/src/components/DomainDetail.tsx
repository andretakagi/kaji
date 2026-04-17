import { useCallback, useEffect, useRef, useState } from "react";
import { deleteDomain, disableDomain, enableDomain } from "../api";
import { useDomain } from "../hooks/useDomain";
import { useFormToggle } from "../hooks/useFormToggle";
import type { DomainToggles, UpdateRuleRequest } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import LoadingState from "./LoadingState";
import RuleCard from "./RuleCard";
import RuleForm from "./RuleForm";
import { SectionHeader } from "./SectionHeader";
import { Toggle } from "./Toggle";

interface Props {
	id: string;
	onBack: () => void;
	onDelete?: () => void;
}

export default function DomainDetail({ id, onBack, onDelete }: Props) {
	const {
		domain,
		loading,
		error,
		setError,
		saving,
		feedback,
		reload,
		handleUpdateDomain,
		handleCreateRule,
		handleUpdateRule,
		handleDeleteRule,
		handleToggleRule,
	} = useDomain(id);

	const [deleting, setDeleting] = useState(false);
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

	const handleToggleEnabled = useCallback(
		async (enabled: boolean) => {
			try {
				if (enabled) {
					await enableDomain(id);
				} else {
					await disableDomain(id);
				}
				await reload();
			} catch (err) {
				setError(getErrorMessage(err, "Failed to toggle domain"));
			}
		},
		[id, setError, reload],
	);

	const handleDeleteDomain = useCallback(async () => {
		setDeleting(true);
		try {
			await deleteDomain(id);
			onDelete?.();
			onBack();
		} catch (err) {
			setError(getErrorMessage(err, "Failed to delete domain"));
			setDeleting(false);
		}
	}, [id, onBack, onDelete, setError]);

	async function handleSaveToggles() {
		if (!localToggles) return;
		await handleUpdateDomain({ name: domain.name, toggles: localToggles });
	}

	function handleDiscardToggles() {
		setLocalToggles(domain.toggles);
		lastSyncedToggles.current = JSON.stringify(domain.toggles);
	}

	if (loading) {
		return <LoadingState label="domain" />;
	}

	const ruleCount = domain.rules.length;

	return (
		<div className="domain-detail">
			<div className="domain-detail-header">
				<button type="button" className="btn btn-ghost domain-detail-back" onClick={onBack}>
					&larr; Domains
				</button>
				<div className="domain-detail-title-row">
					<h1 className="domain-detail-name">{domain.name}</h1>
					<div className="domain-detail-actions">
						<Toggle
							value={domain.enabled}
							onChange={handleToggleEnabled}
							disabled={saving || deleting}
							title={domain.enabled ? "Disable domain" : "Enable domain"}
							aria-label={domain.enabled ? "Disable domain" : "Enable domain"}
						/>
						<ConfirmDeleteButton
							onConfirm={handleDeleteDomain}
							label={`Delete ${domain.name} and all ${ruleCount} ${ruleCount === 1 ? "rule" : "rules"}`}
							disabled={saving}
							deleting={deleting}
							deletingLabel="Deleting..."
						/>
					</div>
				</div>
			</div>

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
						idPrefix={`domain-${id}`}
						domain={domain.name}
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
						domainName={domain.name}
						onSubmit={async (req) => {
							await handleCreateRule(req);
							ruleForm.close();
						}}
						onCancel={ruleForm.close}
					/>
				)}

				{domain.rules.length === 0 ? (
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
								onToggle={handleToggleRule}
								onDelete={handleDeleteRule}
								onUpdate={(ruleId, req) => handleUpdateRule(ruleId, req as UpdateRuleRequest)}
								saving={saving}
							/>
						))}
					</div>
				)}
			</section>
		</div>
	);
}
