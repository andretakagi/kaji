import { useId, useState } from "react";
import { useDomain } from "../hooks/useDomain";
import type {
	CreatePathRequest,
	DomainToggles,
	Path,
	Rule,
	Subdomain,
	UpdatePathRequest,
} from "../types/domain";
import { defaultReverseProxyConfig } from "../types/domain";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import Feedback from "./Feedback";
import RuleCard, { normalizeRule, normalizeToggles } from "./RuleCard";
import { SectionHeader } from "./SectionHeader";
import SubdomainForm from "./SubdomainForm";
import { Toggle } from "./Toggle";

const newPathRule: Rule = {
	handler_type: "reverse_proxy",
	handler_config: { ...defaultReverseProxyConfig },
	advanced_headers: false,
	enabled: true,
};

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

	const [addingSubdomain, setAddingSubdomain] = useState(false);

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
			<Feedback msg={feedback.msg} type={feedback.type} />

			<section className="domain-detail-section domain-detail-section--no-button">
				<SectionHeader title="Domain" />
				<RuleCard
					kind="domain"
					hostLabel={domain.name}
					rule={domain.rule}
					toggles={domain.toggles}
					saving={isSaving}
					ariaLabel={`${domain.name} settings`}
					idPrefix={`domain-${domainSummary.id}`}
					domainName={domain.name}
					onSave={async ({ rule, toggles }) => {
						const togglesChanged =
							JSON.stringify(toggles) !== JSON.stringify(normalizeToggles(domain.toggles));
						const ruleChanged = JSON.stringify(rule) !== JSON.stringify(normalizeRule(domain.rule));
						if (togglesChanged) {
							const ok = await handleUpdateDomain({ name: domain.name, toggles });
							if (!ok) throw new Error("Failed to update domain");
						}
						if (ruleChanged) {
							const ok = await handleUpdateDomainRule(rule);
							if (!ok) throw new Error("Failed to update domain rule");
						}
					}}
				/>
			</section>

			<section className="domain-detail-section">
				<SectionHeader title="Subdomains" />
				{domain.subdomains.length === 0 ? (
					<div className="empty-state">No subdomains yet.</div>
				) : (
					<div className="rule-list">
						{domain.subdomains.map((sub) => {
							const fullHost = `${sub.name}.${domain.name}`;
							return (
								<RuleCard
									key={sub.id}
									kind="subdomain"
									hostLabel={fullHost}
									rule={sub.rule}
									toggles={sub.toggles}
									enabled={sub.enabled}
									saving={isSaving}
									ariaLabel={`${fullHost} subdomain`}
									idPrefix={`subdomain-${sub.id}`}
									domainName={fullHost}
									onSave={async ({ rule, toggles }) => {
										const togglesChanged =
											JSON.stringify(toggles) !== JSON.stringify(normalizeToggles(sub.toggles));
										const ruleChanged =
											JSON.stringify(rule) !== JSON.stringify(normalizeRule(sub.rule));
										if (togglesChanged) {
											const ok = await handleUpdateSubdomain(sub.id, {
												name: sub.name,
												toggles,
											});
											if (!ok) throw new Error("Failed to update subdomain");
										}
										if (ruleChanged) {
											const ok = await handleUpdateSubdomainRule(sub.id, rule);
											if (!ok) throw new Error("Failed to update subdomain rule");
										}
									}}
									onToggleEnabled={(enabled) => handleToggleSubdomain(sub.id, enabled)}
									onDelete={() => handleDeleteSubdomain(sub.id)}
									deleteLabel={`Delete ${fullHost}`}
									enableLabel={`Enable ${fullHost}`}
									disableLabel={`Disable ${fullHost}`}
								/>
							);
						})}
					</div>
				)}
				{addingSubdomain ? (
					<SubdomainForm
						onSubmit={async (req) => {
							const ok = await handleCreateSubdomain(req);
							if (!ok) throw new Error("Failed to create subdomain");
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
					domainName={domain.name}
					rootPaths={domain.paths}
					rootToggles={domain.toggles}
					subdomains={domain.subdomains}
					onCreateRoot={handleCreateDomainPath}
					onUpdateRoot={handleUpdateDomainPath}
					onDeleteRoot={handleDeleteDomainPath}
					onToggleRoot={handleToggleDomainPath}
					onCreateSub={handleCreateSubdomainPath}
					onUpdateSub={handleUpdateSubdomainPath}
					onDeleteSub={handleDeleteSubdomainPath}
					onToggleSub={handleToggleSubdomainPath}
					saving={isSaving}
				/>
			</section>
		</CollapsibleCard>
	);
}

interface PathGroupProps {
	domainName: string;
	rootPaths: Path[];
	rootToggles: DomainToggles;
	subdomains: Subdomain[];
	onCreateRoot: (req: CreatePathRequest) => Promise<boolean>;
	onUpdateRoot: (pathId: string, req: UpdatePathRequest) => Promise<boolean>;
	onDeleteRoot: (pathId: string) => void;
	onToggleRoot: (pathId: string, enabled: boolean) => void;
	onCreateSub: (subId: string, req: CreatePathRequest) => Promise<boolean>;
	onUpdateSub: (subId: string, pathId: string, req: UpdatePathRequest) => Promise<boolean>;
	onDeleteSub: (subId: string, pathId: string) => void;
	onToggleSub: (subId: string, pathId: string, enabled: boolean) => void;
	saving: boolean;
}

interface PathEntry {
	key: string;
	path: Path;
	host: string;
	parentToggles: DomainToggles;
	onUpdate: (req: UpdatePathRequest) => Promise<boolean>;
	onDelete: () => void;
	onToggle: (enabled: boolean) => void;
}

function formatPathLabel(p: Path, host: string): string {
	return `${host}${p.match_value}`;
}

function PathGroup({
	domainName,
	rootPaths,
	rootToggles,
	subdomains,
	onCreateRoot,
	onUpdateRoot,
	onDeleteRoot,
	onToggleRoot,
	onCreateSub,
	onUpdateSub,
	onDeleteSub,
	onToggleSub,
	saving,
}: PathGroupProps) {
	const [adding, setAdding] = useState(false);
	const [addTarget, setAddTarget] = useState<string>("");
	const targetSelectId = useId();

	const sortedRoot = [...rootPaths].sort((a, b) => a.match_value.localeCompare(b.match_value));
	const sortedSubdomains = [...subdomains].sort((a, b) => a.name.localeCompare(b.name));

	const entries: PathEntry[] = [
		...sortedRoot.map<PathEntry>((p) => ({
			key: `root-${p.id}`,
			path: p,
			host: domainName,
			parentToggles: rootToggles,
			onUpdate: (req) => onUpdateRoot(p.id, req),
			onDelete: () => onDeleteRoot(p.id),
			onToggle: (enabled) => onToggleRoot(p.id, enabled),
		})),
		...sortedSubdomains.flatMap((sub) =>
			[...sub.paths]
				.sort((a, b) => a.match_value.localeCompare(b.match_value))
				.map<PathEntry>((p) => ({
					key: `sub-${sub.id}-${p.id}`,
					path: p,
					host: `${sub.name}.${domainName}`,
					parentToggles: sub.toggles,
					onUpdate: (req) => onUpdateSub(sub.id, p.id, req),
					onDelete: () => onDeleteSub(sub.id, p.id),
					onToggle: (enabled) => onToggleSub(sub.id, p.id, enabled),
				})),
		),
	];

	const targetOptions: { value: string; label: string; toggles: DomainToggles }[] = [
		{ value: "", label: domainName || "Root domain", toggles: rootToggles },
		...sortedSubdomains.map((s) => ({
			value: s.id,
			label: `${s.name}.${domainName}`,
			toggles: s.toggles,
		})),
	];

	const activeTarget = targetOptions.find((o) => o.value === addTarget) ?? targetOptions[0];

	function handleStartAdd() {
		setAddTarget("");
		setAdding(true);
	}

	function handleCancelAdd() {
		setAdding(false);
		setAddTarget("");
	}

	async function handleSubmitAdd(req: CreatePathRequest) {
		const ok = addTarget === "" ? await onCreateRoot(req) : await onCreateSub(addTarget, req);
		if (!ok) throw new Error("Failed to create path");
		handleCancelAdd();
	}

	return (
		<div className="path-group">
			{entries.length === 0 && !adding ? (
				<div className="empty-state">No paths.</div>
			) : (
				<div className="rule-list">
					{entries.map((e) => (
						<RuleCard
							key={e.key}
							kind="path"
							hostLabel={formatPathLabel(e.path, e.host)}
							rule={e.path.rule}
							toggles={e.path.toggle_overrides ?? e.parentToggles}
							parentToggles={e.parentToggles}
							pathMatch={e.path.path_match}
							matchValue={e.path.match_value}
							hasOverrides={!!e.path.toggle_overrides}
							enabled={e.path.enabled}
							saving={saving}
							ariaLabel={`${formatPathLabel(e.path, e.host)} path`}
							idPrefix={`path-${e.path.id}`}
							domainName={e.host}
							onSave={async ({ rule, toggles, pathMatch, matchValue, hasOverrides }) => {
								const ok = await e.onUpdate({
									path_match: pathMatch ?? e.path.path_match,
									match_value: matchValue ?? e.path.match_value,
									rule,
									toggle_overrides: hasOverrides ? toggles : null,
								});
								if (!ok) throw new Error("Failed to update path");
							}}
							onToggleEnabled={e.onToggle}
							onDelete={e.onDelete}
							deleteLabel={`Delete ${formatPathLabel(e.path, e.host)}`}
							enableLabel={`Enable ${formatPathLabel(e.path, e.host)}`}
							disableLabel={`Disable ${formatPathLabel(e.path, e.host)}`}
						/>
					))}
				</div>
			)}
			{adding ? (
				<div className="path-add-form">
					{targetOptions.length > 1 && (
						<div className="form-row">
							<div className="form-field">
								<label htmlFor={targetSelectId}>Target Domain</label>
								<select
									id={targetSelectId}
									value={addTarget}
									onChange={(e) => setAddTarget(e.target.value)}
									disabled={saving}
								>
									{targetOptions.map((opt) => (
										<option key={opt.value || "root"} value={opt.value}>
											{opt.label}
										</option>
									))}
								</select>
							</div>
						</div>
					)}
					<RuleCard
						kind="path"
						mode="create"
						hostLabel={`${activeTarget.label} (new path)`}
						rule={newPathRule}
						toggles={activeTarget.toggles}
						parentToggles={activeTarget.toggles}
						pathMatch="prefix"
						matchValue=""
						hasOverrides={false}
						saving={saving}
						ariaLabel={`New path for ${activeTarget.label}`}
						idPrefix={`path-new-${addTarget || "root"}`}
						domainName={activeTarget.label}
						onSave={async ({ rule, toggles, pathMatch, matchValue, hasOverrides }) => {
							await handleSubmitAdd({
								path_match: pathMatch ?? "prefix",
								match_value: matchValue ?? "",
								rule,
								toggle_overrides: hasOverrides ? toggles : null,
							});
						}}
						onCancel={handleCancelAdd}
					/>
				</div>
			) : (
				<button
					type="button"
					className="btn btn-primary"
					onClick={handleStartAdd}
					disabled={saving}
				>
					Add Path
				</button>
			)}
		</div>
	);
}
