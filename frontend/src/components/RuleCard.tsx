import { useId, useState } from "react";
import { cn } from "../cn";
import type {
	DomainToggles,
	ErrorConfig,
	FileServerConfig,
	PathMatch,
	RedirectConfig,
	ReverseProxyConfig,
	Rule,
	StaticResponseConfig,
	UpdateRuleRequest,
} from "../types/domain";
import {
	defaultDomainToggles,
	defaultErrorConfig,
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
	handlerLabels,
	pathMatchLabels,
	pathMatchOptions,
} from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { pathMatchWarning, validateRule } from "../utils/validate";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { RuleDetails } from "./RuleDetails";
import RuleEditor from "./RuleEditor";
import { Toggle } from "./Toggle";

export function normalizeToggles(t: DomainToggles | null | undefined): DomainToggles {
	const base = defaultDomainToggles;
	if (!t) return base;
	return {
		force_https: t.force_https ?? base.force_https,
		compression: t.compression ?? base.compression,
		access_log: t.access_log ?? base.access_log,
		basic_auth: { ...base.basic_auth, ...(t.basic_auth ?? {}) },
		ip_filtering: { ...base.ip_filtering, ...(t.ip_filtering ?? {}) },
		error_pages: t.error_pages ?? base.error_pages,
		headers: {
			response: { ...base.headers.response, ...(t.headers?.response ?? {}) },
		},
	};
}

export function normalizeRule(r: Rule): Rule {
	switch (r.handler_type) {
		case "reverse_proxy": {
			const c = (r.handler_config ?? {}) as Partial<ReverseProxyConfig>;
			const base = defaultReverseProxyConfig;
			return {
				...r,
				handler_config: {
					upstream: c.upstream ?? base.upstream,
					tls_skip_verify: c.tls_skip_verify ?? base.tls_skip_verify,
					websocket_passthrough: c.websocket_passthrough ?? base.websocket_passthrough,
					load_balancing: { ...base.load_balancing, ...(c.load_balancing ?? {}) },
					request_headers: { ...base.request_headers, ...(c.request_headers ?? {}) },
				},
			};
		}
		case "static_response": {
			const c = (r.handler_config ?? {}) as Partial<StaticResponseConfig>;
			return { ...r, handler_config: { ...defaultStaticResponseConfig, ...c } };
		}
		case "redirect": {
			const c = (r.handler_config ?? {}) as Partial<RedirectConfig>;
			return { ...r, handler_config: { ...defaultRedirectConfig, ...c } };
		}
		case "file_server": {
			const c = (r.handler_config ?? {}) as Partial<FileServerConfig>;
			return { ...r, handler_config: { ...defaultFileServerConfig, ...c } };
		}
		case "error": {
			const c = (r.handler_config ?? {}) as Partial<ErrorConfig>;
			return { ...r, handler_config: { ...defaultErrorConfig, ...c } };
		}
		default:
			return r;
	}
}

export type RuleCardKind = "domain" | "subdomain" | "path";

export interface RuleCardSavePayload {
	rule: UpdateRuleRequest;
	toggles: DomainToggles;
	pathMatch?: PathMatch;
	matchValue?: string;
	hasOverrides?: boolean;
}

interface RuleCardProps {
	kind: RuleCardKind;
	hostLabel: string;
	rule: Rule;
	toggles: DomainToggles;
	parentToggles?: DomainToggles;
	pathMatch?: PathMatch;
	matchValue?: string;
	hasOverrides?: boolean;
	enabled?: boolean;
	parentEnabled?: boolean;
	ruleEnabled?: boolean;
	saving: boolean;
	ariaLabel?: string;
	idPrefix: string;
	domainName?: string;
	deleteLabel?: string;
	enableLabel?: string;
	disableLabel?: string;
	enableRuleLabel?: string;
	disableRuleLabel?: string;
	mode?: "view" | "create";
	onSave: (payload: RuleCardSavePayload) => Promise<void>;
	onToggleEnabled?: (enabled: boolean) => void;
	onToggleRuleEnabled?: (enabled: boolean) => void;
	onDelete?: () => void;
	onCancel?: () => void;
}

export default function RuleCard({
	kind,
	hostLabel,
	rule,
	toggles,
	parentToggles,
	pathMatch,
	matchValue,
	hasOverrides,
	enabled,
	parentEnabled,
	ruleEnabled,
	saving,
	ariaLabel,
	idPrefix,
	domainName,
	deleteLabel,
	enableLabel,
	disableLabel,
	enableRuleLabel,
	disableRuleLabel,
	mode = "view",
	onSave,
	onToggleEnabled,
	onToggleRuleEnabled,
	onDelete,
	onCancel,
}: RuleCardProps) {
	const formId = useId();
	const isPath = kind === "path";
	const isCreate = mode === "create";
	const parentLocked = parentEnabled === false;
	const wrapperOff = enabled === false;
	const ruleOff = ruleEnabled === false;
	const showEnableToggle =
		!isCreate && kind !== "domain" && enabled !== undefined && !!onToggleEnabled;
	const showRuleToggle =
		!isCreate && rule.handler_type !== "none" && ruleEnabled !== undefined && !!onToggleRuleEnabled;
	const showDelete = !isCreate && kind !== "domain" && !!onDelete;

	const errorMessageForRule = (r: Rule): string | undefined =>
		r.handler_type === "error" ? (r.handler_config as ErrorConfig).message : undefined;

	const [editing, setEditing] = useState(isCreate);
	const [editedRule, setEditedRule] = useState<Rule>(normalizeRule(rule));
	const [editedToggles, setEditedToggles] = useState<DomainToggles>(normalizeToggles(toggles));
	const [editedPathMatch, setEditedPathMatch] = useState<PathMatch>(pathMatch ?? "prefix");
	const [editedMatchValue, setEditedMatchValue] = useState<string>(matchValue ?? "");
	const [editedHasOverrides, setEditedHasOverrides] = useState<boolean>(!!hasOverrides);
	const [savingEdit, setSavingEdit] = useState(false);
	const [formError, setFormError] = useState<string | null>(null);

	const seedFromProps = () => {
		setEditedRule(normalizeRule(rule));
		setEditedToggles(normalizeToggles(toggles));
		setEditedPathMatch(pathMatch ?? "prefix");
		setEditedMatchValue(matchValue ?? "");
		setEditedHasOverrides(!!hasOverrides);
		setFormError(null);
	};

	const handleStartEdit = () => {
		seedFromProps();
		setEditing(true);
	};

	const handleCancel = () => {
		if (isCreate) {
			onCancel?.();
			return;
		}
		setEditing(false);
	};

	async function handleSave() {
		if (isPath && !editedMatchValue.trim()) {
			setFormError("Path value is required");
			return;
		}
		const ruleError = validateRule(editedRule);
		if (ruleError) {
			setFormError(ruleError);
			return;
		}

		setFormError(null);
		setSavingEdit(true);
		try {
			await onSave({
				rule: {
					handler_type: editedRule.handler_type,
					handler_config: editedRule.handler_config,
					advanced_headers: editedRule.advanced_headers,
				},
				toggles: editedToggles,
				...(isPath
					? {
							pathMatch: editedPathMatch,
							matchValue: editedMatchValue.trim(),
							hasOverrides: editedHasOverrides,
						}
					: {}),
			});
			if (!isCreate) setEditing(false);
		} catch (err) {
			setFormError(getErrorMessage(err, "Save failed"));
		} finally {
			setSavingEdit(false);
		}
	}

	const title = (
		<>
			<span className="rule-card-match">{hostLabel}</span>
			{isPath && pathMatch && (
				<span className={cn("path-match-badge", `path-match-${pathMatch}`)}>
					{pathMatchLabels[pathMatch]}
				</span>
			)}
			<span className={cn("rule-card-handler-badge", `handler-${rule.handler_type}`)}>
				{handlerLabels[rule.handler_type]}
			</span>
		</>
	);

	const actions = (showEnableToggle || showRuleToggle || showDelete) && (
		<>
			{showRuleToggle && ruleEnabled !== undefined && onToggleRuleEnabled && (
				<Toggle
					value={ruleEnabled}
					onChange={onToggleRuleEnabled}
					disabled={saving || parentLocked || wrapperOff}
					title={ruleEnabled ? "Disable rule" : "Enable rule"}
					aria-label={
						ruleEnabled
							? (disableRuleLabel ?? `Disable ${kind} rule`)
							: (enableRuleLabel ?? `Enable ${kind} rule`)
					}
					stopPropagation
				/>
			)}
			{showEnableToggle && enabled !== undefined && onToggleEnabled && (
				<Toggle
					value={enabled}
					onChange={onToggleEnabled}
					disabled={saving || parentLocked}
					title={enabled ? "Disable" : "Enable"}
					aria-label={
						enabled ? (disableLabel ?? `Disable ${kind}`) : (enableLabel ?? `Enable ${kind}`)
					}
					stopPropagation
				/>
			)}
			{showDelete && onDelete && (
				<ConfirmDeleteButton
					onConfirm={onDelete}
					label={deleteLabel ?? `Delete ${kind}`}
					disabled={saving}
				/>
			)}
		</>
	);

	const togglesIdPrefix = `${idPrefix}-toggles`;
	const ruleIdPrefix = `${idPrefix}-rule`;

	const togglesForView = normalizeToggles(
		isPath ? (hasOverrides ? toggles : (parentToggles ?? toggles)) : toggles,
	);

	return (
		<CollapsibleCard
			title={title}
			actions={actions || undefined}
			disabled={(wrapperOff || ruleOff) && !editing}
			forceExpanded={editing}
			ariaLabel={ariaLabel ?? hostLabel}
		>
			<div className="rule-card-body">
				{editing ? (
					<>
						{isPath && (
							<div className="rule-card-section">
								<h4 className="rule-card-section-title">Path</h4>
								<div className="form-row">
									<div className="form-field">
										<label htmlFor={`path-match-${formId}`}>Path Match</label>
										<select
											id={`path-match-${formId}`}
											value={editedPathMatch}
											onChange={(e) => setEditedPathMatch(e.target.value as PathMatch)}
											disabled={savingEdit}
										>
											{pathMatchOptions.map((o) => (
												<option key={o.value} value={o.value}>
													{o.label}
												</option>
											))}
										</select>
									</div>
									<div className="form-field">
										<label htmlFor={`path-value-${formId}`}>Path</label>
										<input
											id={`path-value-${formId}`}
											type="text"
											placeholder="/api/*"
											value={editedMatchValue}
											onChange={(e) => setEditedMatchValue(e.target.value)}
											maxLength={253}
											required
											disabled={savingEdit}
										/>
										{pathMatchWarning(editedPathMatch, editedMatchValue) && (
											<span className="field-warning">
												{pathMatchWarning(editedPathMatch, editedMatchValue)}
											</span>
										)}
									</div>
								</div>
							</div>
						)}

						<div className="rule-card-section">
							<h4 className="rule-card-section-title">Rule</h4>
							<RuleEditor
								allowNone={!isPath}
								value={editedRule}
								onChange={setEditedRule}
								idPrefix={ruleIdPrefix}
							/>
						</div>

						<div className="rule-card-section">
							<div className="rule-card-section-heading-row">
								<h4 className="rule-card-section-title">Toggles</h4>
								{isPath && (
									<label className="rule-card-overrides-switch">
										<input
											type="checkbox"
											checked={editedHasOverrides}
											onChange={(e) => {
												const next = e.target.checked;
												setEditedHasOverrides(next);
												if (next && !hasOverrides) {
													setEditedToggles(normalizeToggles(parentToggles));
												}
											}}
											disabled={savingEdit}
										/>
										<span>Override parent toggles</span>
									</label>
								)}
							</div>
							{isPath && !editedHasOverrides ? (
								<div className="rule-card-detail-empty">
									Inherits parent toggles. Enable override to customize.
								</div>
							) : (
								<DomainToggleGrid
									toggles={editedToggles}
									onUpdate={(key, value) => setEditedToggles((prev) => ({ ...prev, [key]: value }))}
									idPrefix={togglesIdPrefix}
									domain={domainName ?? hostLabel}
									errorMessage={errorMessageForRule(editedRule)}
								/>
							)}
						</div>

						{formError && (
							<div className="inline-error" role="alert">
								{formError}
							</div>
						)}

						<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
							<button
								type="button"
								className="btn btn-ghost"
								onClick={handleCancel}
								disabled={savingEdit}
							>
								Cancel
							</button>
							<button
								type="button"
								className="btn btn-primary"
								onClick={handleSave}
								disabled={savingEdit}
							>
								{savingEdit ? "Saving..." : "Save"}
							</button>
						</div>
					</>
				) : (
					<>
						<div className="rule-card-section">
							<h4 className="rule-card-section-title">Rule</h4>
							<RuleDetails rule={normalizeRule(rule)} />
						</div>

						<div className="rule-card-section">
							<h4 className="rule-card-section-title">Toggles</h4>
							{isPath && !hasOverrides ? (
								<div className="rule-card-detail-empty">Inherits parent toggles.</div>
							) : (
								<DomainToggleGrid
									toggles={togglesForView}
									onUpdate={() => {}}
									idPrefix={togglesIdPrefix}
									domain={domainName ?? hostLabel}
									disabled
									errorMessage={errorMessageForRule(rule)}
								/>
							)}
						</div>

						<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
							<button
								type="button"
								className="btn btn-primary"
								onClick={handleStartEdit}
								disabled={saving}
							>
								Edit
							</button>
						</div>
					</>
				)}
			</div>
		</CollapsibleCard>
	);
}
