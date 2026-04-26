import { useEffect, useState } from "react";
import { cn } from "../cn";
import type {
	CreatePathRequest,
	DomainToggles,
	Rule,
	RuleHandlerType,
	Subdomain,
	UpdatePathRequest,
	UpdateRuleRequest,
	UpdateSubdomainRequest,
} from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import PathCard from "./PathCard";
import PathForm from "./PathForm";
import RuleEditor from "./RuleEditor";
import RuleSummaryCard from "./RuleSummaryCard";
import { Toggle } from "./Toggle";

export interface SubdomainCardProps {
	domainName: string;
	subdomain: Subdomain;
	saving: boolean;
	onUpdateRule: (req: UpdateRuleRequest) => Promise<void>;
	onUpdate: (req: UpdateSubdomainRequest) => Promise<void>;
	onDelete: () => void;
	onToggle: (enabled: boolean) => void;
	onCreatePath: (req: CreatePathRequest) => Promise<void>;
	onUpdatePath: (pathId: string, req: UpdatePathRequest) => Promise<void>;
	onDeletePath: (pathId: string) => void;
	onTogglePath: (pathId: string, enabled: boolean) => void;
}

const handlerLabels: Record<RuleHandlerType, string> = {
	reverse_proxy: "Reverse Proxy",
	redirect: "Redirect",
	file_server: "File Server",
	static_response: "Static Response",
	none: "No Handler",
};

export default function SubdomainCard({
	domainName,
	subdomain,
	saving,
	onUpdateRule,
	onUpdate,
	onDelete,
	onToggle,
	onCreatePath,
	onUpdatePath,
	onDeletePath,
	onTogglePath,
}: SubdomainCardProps) {
	const fullHost = `${subdomain.name}.${domainName}`;
	const idPrefix = `subdomain-${subdomain.id}`;

	const [toggles, setToggles] = useState<DomainToggles>(subdomain.toggles);
	const [editingRule, setEditingRule] = useState(false);
	const [editedRule, setEditedRule] = useState<Rule>(subdomain.rule);
	const [savingRule, setSavingRule] = useState(false);
	const [addingPath, setAddingPath] = useState(false);

	useEffect(() => {
		setToggles(subdomain.toggles);
	}, [subdomain.toggles]);

	useEffect(() => {
		if (!editingRule) {
			setEditedRule(subdomain.rule);
		}
	}, [subdomain.rule, editingRule]);

	function handleToggleUpdate<K extends keyof DomainToggles>(key: K, value: DomainToggles[K]) {
		const next = { ...toggles, [key]: value };
		setToggles(next);
		void onUpdate({ name: subdomain.name, toggles: next });
	}

	async function handleSaveRule() {
		setSavingRule(true);
		try {
			await onUpdateRule({
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
		setEditedRule(subdomain.rule);
		setEditingRule(false);
	}

	const title = (
		<>
			<span className="rule-card-match">{fullHost}</span>
			<span className={cn("rule-card-handler-badge", `handler-${subdomain.rule.handler_type}`)}>
				{handlerLabels[subdomain.rule.handler_type]}
			</span>
		</>
	);

	const actions = (
		<>
			<Toggle
				value={subdomain.enabled}
				onChange={onToggle}
				disabled={saving}
				title={subdomain.enabled ? "Disable" : "Enable"}
				aria-label={subdomain.enabled ? "Disable subdomain" : "Enable subdomain"}
				stopPropagation
			/>
			<ConfirmDeleteButton onConfirm={onDelete} label={`Delete ${fullHost}`} disabled={saving} />
		</>
	);

	return (
		<CollapsibleCard
			title={title}
			actions={actions}
			disabled={!subdomain.enabled}
			ariaLabel={`${fullHost} subdomain`}
		>
			<div className="rule-card-body">
				<section className="domain-detail-section">
					<h4 className="subdomain-section-title">Subdomain Toggles</h4>
					<DomainToggleGrid
						toggles={toggles}
						onUpdate={handleToggleUpdate}
						idPrefix={idPrefix}
						domain={fullHost}
					/>
				</section>

				<section className="domain-detail-section">
					<h4 className="subdomain-section-title">Rule</h4>
					{editingRule ? (
						<div className="rule-editor-inline">
							<RuleEditor
								allowNone
								value={editedRule}
								onChange={setEditedRule}
								idPrefix={`${idPrefix}-rule`}
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
							rule={subdomain.rule}
							onEdit={() => {
								setEditedRule(subdomain.rule);
								setEditingRule(true);
							}}
							disabled={saving}
						/>
					)}
				</section>

				<section className="domain-detail-section">
					<h4 className="subdomain-section-title">Paths</h4>
					{subdomain.paths.length === 0 && !addingPath ? (
						<div className="empty-state">No paths yet.</div>
					) : (
						<div className="rule-list">
							{subdomain.paths.map((p) => (
								<PathCard
									key={p.id}
									path={p}
									domainName={fullHost}
									parentToggles={subdomain.toggles}
									onUpdate={(req) => onUpdatePath(p.id, req)}
									onDelete={() => onDeletePath(p.id)}
									onToggle={(enabled) => onTogglePath(p.id, enabled)}
									saving={saving}
								/>
							))}
						</div>
					)}

					{addingPath ? (
						<PathForm
							domainName={fullHost}
							parentToggles={subdomain.toggles}
							inline
							onSubmit={async (req) => {
								await onCreatePath(req as CreatePathRequest);
								setAddingPath(false);
							}}
							onCancel={() => setAddingPath(false)}
						/>
					) : (
						<button
							type="button"
							className="btn btn-primary"
							onClick={() => setAddingPath(true)}
							disabled={saving}
						>
							Add Path
						</button>
					)}
				</section>
			</div>
		</CollapsibleCard>
	);
}
