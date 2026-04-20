import { useCallback, useEffect, useRef, useState } from "react";
import { deleteDomain, disableDomain, enableDomain } from "../api";
import { useDomain } from "../hooks/useDomain";
import { useFormToggle } from "../hooks/useFormToggle";
import type {
	CreateSubdomainRequest,
	DomainToggles,
	HandlerConfigValue,
	HandlerType,
	SubdomainHandlerType,
	UpdateRuleRequest,
} from "../types/domain";
import {
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
} from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import { DomainToggleGrid } from "./DomainToggleGrid";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import HandlerConfig from "./HandlerConfig";
import LoadingState from "./LoadingState";
import RuleCard from "./RuleCard";
import RuleForm from "./RuleForm";
import { SectionHeader } from "./SectionHeader";
import SubdomainCard from "./SubdomainCard";
import SubdomainDetail from "./SubdomainDetail";
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
		handleCreateSubdomain,
		handleUpdateSubdomain,
		handleDeleteSubdomain,
		handleToggleSubdomain,
		handleCreateSubdomainRule,
		handleUpdateSubdomainRule,
		handleDeleteSubdomainRule,
		handleToggleSubdomainRule,
	} = useDomain(id);

	const [deleting, setDeleting] = useState(false);
	const [localToggles, setLocalToggles] = useState<DomainToggles | null>(null);
	const lastSyncedToggles = useRef<string>("");
	const ruleForm = useFormToggle();
	const [selectedSubdomain, setSelectedSubdomain] = useState<string | null>(null);
	const subdomainForm = useFormToggle();
	const [newSubName, setNewSubName] = useState("");
	const [newSubHandlerType, setNewSubHandlerType] = useState<SubdomainHandlerType>("none");
	const [newSubHandlerConfig, setNewSubHandlerConfig] = useState<HandlerConfigValue>({});

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

	function resetSubdomainForm() {
		setNewSubName("");
		setNewSubHandlerType("none");
		setNewSubHandlerConfig({});
	}

	async function handleSubmitSubdomain(e: React.SubmitEvent) {
		e.preventDefault();
		if (!newSubName.trim()) return;
		const req: CreateSubdomainRequest = {
			name: newSubName.trim(),
			handler_type: newSubHandlerType,
			handler_config: newSubHandlerType === "none" ? null : newSubHandlerConfig,
		};
		await handleCreateSubdomain(req);
		resetSubdomainForm();
		subdomainForm.close();
	}

	if (loading) {
		return <LoadingState label="domain" />;
	}

	const activeSub = selectedSubdomain
		? domain.subdomains.find((s) => s.id === selectedSubdomain)
		: null;

	if (activeSub) {
		return (
			<SubdomainDetail
				subdomain={activeSub}
				domainName={domain.name}
				onBack={() => setSelectedSubdomain(null)}
				onUpdate={handleUpdateSubdomain}
				onDelete={(subId) => {
					handleDeleteSubdomain(subId);
					setSelectedSubdomain(null);
				}}
				onToggle={handleToggleSubdomain}
				onCreateRule={handleCreateSubdomainRule}
				onUpdateRule={handleUpdateSubdomainRule}
				onDeleteRule={handleDeleteSubdomainRule}
				onToggleRule={handleToggleSubdomainRule}
				saving={saving}
			/>
		);
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
				<SectionHeader title="Subdomains">
					<button
						type="button"
						className="btn btn-primary"
						onClick={subdomainForm.toggle}
						disabled={saving}
					>
						{subdomainForm.visible ? "Cancel" : "Add Subdomain"}
					</button>
				</SectionHeader>

				{subdomainForm.visible && (
					<form className="subdomain-create-form" onSubmit={handleSubmitSubdomain}>
						<div className="form-row">
							<div className="form-field">
								<label htmlFor="new-sub-name">Subdomain Name</label>
								<div className="subdomain-name-input">
									<input
										id="new-sub-name"
										type="text"
										placeholder="api"
										value={newSubName}
										onChange={(e) => setNewSubName(e.target.value)}
										maxLength={63}
										required
									/>
									<span className="subdomain-name-suffix">.{domain.name}</span>
								</div>
							</div>
						</div>
						<div className="form-row">
							<div className="form-field">
								<label htmlFor="new-sub-handler">Handler Type</label>
								<select
									id="new-sub-handler"
									value={newSubHandlerType}
									onChange={(e) => {
										const next = e.target.value as SubdomainHandlerType;
										setNewSubHandlerType(next);
										if (next === "none") {
											setNewSubHandlerConfig({});
										} else if (next === "reverse_proxy") {
											setNewSubHandlerConfig({ ...defaultReverseProxyConfig });
										} else if (next === "redirect") {
											setNewSubHandlerConfig({ ...defaultRedirectConfig });
										} else if (next === "file_server") {
											setNewSubHandlerConfig({ ...defaultFileServerConfig });
										} else if (next === "static_response") {
											setNewSubHandlerConfig({ ...defaultStaticResponseConfig });
										}
									}}
								>
									<option value="none">None (rules only)</option>
									<option value="reverse_proxy">Reverse Proxy</option>
									<option value="redirect">Redirect</option>
									<option value="file_server">File Server</option>
									<option value="static_response">Static Response</option>
								</select>
							</div>
						</div>
						{newSubHandlerType !== "none" && (
							<HandlerConfig
								type={newSubHandlerType as HandlerType}
								config={newSubHandlerConfig}
								onChange={setNewSubHandlerConfig}
								domain={`${newSubName}.${domain.name}`}
							/>
						)}
						<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
							<button
								type="button"
								className="btn btn-ghost"
								onClick={() => {
									resetSubdomainForm();
									subdomainForm.close();
								}}
							>
								Cancel
							</button>
							<button type="submit" className="btn btn-primary" disabled={saving}>
								{saving ? "Creating..." : "Add Subdomain"}
							</button>
						</div>
					</form>
				)}

				{domain.subdomains.length === 0 && !subdomainForm.visible ? (
					<div className="empty-state">
						No subdomains. Subdomains let you route prefixed names like api.{domain.name}{" "}
						separately.
					</div>
				) : (
					<div className="subdomain-list">
						{domain.subdomains.map((sub) => (
							<SubdomainCard
								key={sub.id}
								subdomain={sub}
								domainName={domain.name}
								onSelect={setSelectedSubdomain}
								onToggle={handleToggleSubdomain}
								onDelete={handleDeleteSubdomain}
								saving={saving}
							/>
						))}
					</div>
				)}
			</section>

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
