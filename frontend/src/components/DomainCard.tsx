import { useEffect, useRef, useState } from "react";
import { useDomain } from "../hooks/useDomain";
import type {
	CreatePathRequest,
	DomainToggles,
	Path,
	Rule,
	UpdatePathRequest,
} from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import PathCard from "./PathCard";
import PathForm from "./PathForm";
import RuleEditor from "./RuleEditor";
import RuleSummaryCard from "./RuleSummaryCard";
import { SectionHeader } from "./SectionHeader";
import SubdomainCard from "./SubdomainCard";
import SubdomainForm from "./SubdomainForm";
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
		handleUpdateDomainRule,
		handleCreateDomainPath,
		handleUpdateDomainPath,
		handleDeleteDomainPath,
		handleToggleDomainPath,
		handleCreateSubdomain,
		handleUpdateSubdomain,
		handleDeleteSubdomain,
		handleToggleSubdomain,
		handleUpdateSubdomainRule,
		handleCreateSubdomainPath,
		handleUpdateSubdomainPath,
		handleDeleteSubdomainPath,
		handleToggleSubdomainPath,
	} = useDomain(domainSummary.id);

	const [localToggles, setLocalToggles] = useState<DomainToggles | null>(null);
	const lastSyncedToggles = useRef<string>("");

	const [editingRule, setEditingRule] = useState(false);
	const [editedRule, setEditedRule] = useState<Rule>(domain.rule);
	const [savingRule, setSavingRule] = useState(false);
	const [addingSubdomain, setAddingSubdomain] = useState(false);

	useEffect(() => {
		if (!domain.id) return;
		const serialized = JSON.stringify(domain.toggles);
		if (serialized !== lastSyncedToggles.current) {
			lastSyncedToggles.current = serialized;
			setLocalToggles(domain.toggles);
		}
	}, [domain.id, domain.toggles]);

	useEffect(() => {
		if (!editingRule) {
			setEditedRule(domain.rule);
		}
	}, [domain.rule, editingRule]);

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

	async function handleSaveRule() {
		setSavingRule(true);
		try {
			await handleUpdateDomainRule({
				handler_type: editedRule.handler_type,
				handler_config: editedRule.handler_config,
				advanced_headers: editedRule.advanced_headers,
			});
			setEditingRule(false);
		} finally {
			setSavingRule(false);
		}
	}

	function handleCancelRule() {
		setEditedRule(domain.rule);
		setEditingRule(false);
	}

	const title = <span className="domain-card-name">{domainSummary.name}</span>;

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
				<SectionHeader title="Domain Rule" />
				{editingRule ? (
					<div className="rule-editor-inline">
						<RuleEditor
							allowNone
							value={editedRule}
							onChange={setEditedRule}
							idPrefix={`domain-${domainSummary.id}-rule`}
						/>
						<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
							<button
								type="button"
								className="btn btn-ghost"
								onClick={handleCancelRule}
								disabled={savingRule}
							>
								Cancel
							</button>
							<button
								type="button"
								className="btn btn-primary"
								onClick={handleSaveRule}
								disabled={savingRule}
							>
								{savingRule ? "Saving..." : "Save Rule"}
							</button>
						</div>
					</div>
				) : (
					<RuleSummaryCard
						title="Root"
						rule={domain.rule}
						onEdit={() => {
							setEditedRule(domain.rule);
							setEditingRule(true);
						}}
						disabled={isSaving}
					/>
				)}
			</section>

			<section className="domain-detail-section">
				<SectionHeader title="Subdomains" />
				{domain.subdomains.length === 0 ? (
					<div className="empty-state">No subdomains yet.</div>
				) : (
					<div className="rule-list">
						{domain.subdomains.map((sub) => (
							<SubdomainCard
								key={sub.id}
								domainName={domain.name}
								subdomain={sub}
								saving={isSaving}
								onUpdateRule={(req) => handleUpdateSubdomainRule(sub.id, req)}
								onUpdate={(req) => handleUpdateSubdomain(sub.id, req)}
								onDelete={() => handleDeleteSubdomain(sub.id)}
								onToggle={(enabled) => handleToggleSubdomain(sub.id, enabled)}
								onCreatePath={(req) => handleCreateSubdomainPath(sub.id, req)}
								onUpdatePath={(pathId, req) => handleUpdateSubdomainPath(sub.id, pathId, req)}
								onDeletePath={(pathId) => handleDeleteSubdomainPath(sub.id, pathId)}
								onTogglePath={(pathId, enabled) =>
									handleToggleSubdomainPath(sub.id, pathId, enabled)
								}
							/>
						))}
					</div>
				)}
				{addingSubdomain ? (
					<SubdomainForm
						onSubmit={async (req) => {
							await handleCreateSubdomain(req);
							setAddingSubdomain(false);
						}}
						onCancel={() => setAddingSubdomain(false)}
					/>
				) : (
					<button
						type="button"
						className="btn btn-primary"
						onClick={() => setAddingSubdomain(true)}
						disabled={isSaving}
					>
						Add Subdomain
					</button>
				)}
			</section>

			<section className="domain-detail-section">
				<SectionHeader title="Paths" />
				<PathGroup
					header={domain.name}
					domainName={domain.name}
					paths={domain.paths}
					parentToggles={domain.toggles}
					onCreate={handleCreateDomainPath}
					onUpdate={handleUpdateDomainPath}
					onDelete={handleDeleteDomainPath}
					onToggle={handleToggleDomainPath}
					saving={isSaving}
				/>
				{domain.subdomains.map((sub) => {
					const fullHost = `${sub.name}.${domain.name}`;
					return (
						<PathGroup
							key={sub.id}
							header={fullHost}
							domainName={fullHost}
							paths={sub.paths}
							parentToggles={sub.toggles}
							onCreate={(req) => handleCreateSubdomainPath(sub.id, req)}
							onUpdate={(pid, req) => handleUpdateSubdomainPath(sub.id, pid, req)}
							onDelete={(pid) => handleDeleteSubdomainPath(sub.id, pid)}
							onToggle={(pid, enabled) => handleToggleSubdomainPath(sub.id, pid, enabled)}
							saving={isSaving}
						/>
					);
				})}
			</section>
		</CollapsibleCard>
	);
}

interface PathGroupProps {
	header: string;
	domainName: string;
	paths: Path[];
	parentToggles: DomainToggles;
	onCreate: (req: CreatePathRequest) => Promise<void>;
	onUpdate: (pathId: string, req: UpdatePathRequest) => Promise<void>;
	onDelete: (pathId: string) => void;
	onToggle: (pathId: string, enabled: boolean) => void;
	saving: boolean;
}

function PathGroup({
	header,
	domainName,
	paths,
	parentToggles,
	onCreate,
	onUpdate,
	onDelete,
	onToggle,
	saving,
}: PathGroupProps) {
	const [adding, setAdding] = useState(false);

	return (
		<div className="path-group">
			<h4 className="path-group-header">{header}</h4>
			{paths.length === 0 && !adding ? (
				<div className="empty-state">No paths.</div>
			) : (
				<div className="rule-list">
					{paths.map((p) => (
						<PathCard
							key={p.id}
							path={p}
							domainName={domainName}
							parentToggles={parentToggles}
							onUpdate={(req) => onUpdate(p.id, req)}
							onDelete={() => onDelete(p.id)}
							onToggle={(enabled) => onToggle(p.id, enabled)}
							saving={saving}
						/>
					))}
				</div>
			)}
			{adding ? (
				<PathForm
					domainName={domainName}
					parentToggles={parentToggles}
					inline
					onSubmit={async (req) => {
						await onCreate(req as CreatePathRequest);
						setAdding(false);
					}}
					onCancel={() => setAdding(false)}
				/>
			) : (
				<button
					type="button"
					className="btn btn-primary"
					onClick={() => setAdding(true)}
					disabled={saving}
				>
					Add Path
				</button>
			)}
		</div>
	);
}
