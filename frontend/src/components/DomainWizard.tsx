import { useCallback, useRef, useState } from "react";
import type {
	CreateDomainFullRequest,
	DomainToggles,
	HandlerType,
	MatchType,
	PathMatch,
	ReverseProxyConfig,
	StaticResponseConfig,
} from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validateDomain, validateUpstream } from "../utils/validate";
import { DomainToggleGrid } from "./DomainToggleGrid";
import HandlerConfig from "./HandlerConfig";
import { Toggle } from "./Toggle";
import WizardReview from "./WizardReview";

type HandlerSelection = "none" | HandlerType;

export interface WizardRule {
	key: number;
	matchType: Exclude<MatchType, "">;
	pathMatch: PathMatch;
	matchValue: string;
	handlerType: HandlerType;
	handlerConfig: ReverseProxyConfig | StaticResponseConfig | Record<string, unknown>;
	toggleOverrides: DomainToggles | null;
}

export interface WizardData {
	name: string;
	toggles: DomainToggles;
	rootRule: {
		handlerType: HandlerSelection;
		handlerConfig: ReverseProxyConfig | StaticResponseConfig | Record<string, unknown>;
	};
	rules: WizardRule[];
}

const STEP_LABELS = ["Domain", "Toggles", "Root Rule", "Rules", "Review"];

const handlerOptions: readonly { value: HandlerSelection; label: string }[] = [
	{ value: "none", label: "None" },
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
] as const;

const ruleMatchOptions: { value: Exclude<MatchType, "">; label: string }[] = [
	{ value: "subdomain", label: "Subdomain" },
	{ value: "path", label: "Path" },
];

const pathMatchOptions: { value: PathMatch; label: string }[] = [
	{ value: "prefix", label: "Prefix" },
	{ value: "exact", label: "Exact" },
	{ value: "regex", label: "Regex" },
];

const handlerTypeOptions: { value: HandlerType; label: string }[] = [
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
];

interface Props {
	onCreate: (req: CreateDomainFullRequest) => Promise<void>;
	onCancel: () => void;
	existingDomains: string[];
}

export default function DomainWizard({ onCreate, onCancel, existingDomains }: Props) {
	const ruleKeyRef = useRef(0);
	const [step, setStep] = useState(0);
	const [submitting, setSubmitting] = useState(false);
	const [error, setError] = useState("");
	const [data, setData] = useState<WizardData>({
		name: "",
		toggles: { ...defaultDomainToggles },
		rootRule: {
			handlerType: "none",
			handlerConfig: { ...defaultReverseProxyConfig },
		},
		rules: [],
	});

	const [ruleFormActive, setRuleFormActive] = useState(false);
	const [ruleMatchType, setRuleMatchType] = useState<Exclude<MatchType, "">>("subdomain");
	const [rulePathMatch, setRulePathMatch] = useState<PathMatch>("prefix");
	const [ruleMatchValue, setRuleMatchValue] = useState("");
	const [ruleHandlerType, setRuleHandlerType] = useState<HandlerType>("reverse_proxy");
	const [ruleHandlerConfig, setRuleHandlerConfig] = useState<
		ReverseProxyConfig | StaticResponseConfig | Record<string, unknown>
	>({ ...defaultReverseProxyConfig });
	const [ruleOverridesOpen, setRuleOverridesOpen] = useState(false);
	const [ruleToggleOverrides, setRuleToggleOverrides] = useState<DomainToggles>({
		...defaultDomainToggles,
	});

	const resetRuleForm = useCallback(() => {
		setRuleMatchType("subdomain");
		setRulePathMatch("prefix");
		setRuleMatchValue("");
		setRuleHandlerType("reverse_proxy");
		setRuleHandlerConfig({ ...defaultReverseProxyConfig });
		setRuleOverridesOpen(false);
		setRuleToggleOverrides({ ...data.toggles });
	}, [data.toggles]);

	const validateToggles = (toggles: DomainToggles): string | null => {
		if (toggles.basic_auth.enabled && !toggles.basic_auth.username.trim()) {
			return "Username is required for basic auth";
		}
		return null;
	};

	const validateReverseProxy = (rp: ReverseProxyConfig): string | null => {
		const upstreamErr = validateUpstream(rp.upstream);
		if (upstreamErr) return upstreamErr;
		if (rp.load_balancing.enabled) {
			if (rp.load_balancing.upstreams.length === 0) {
				return "Load balancing requires at least one additional upstream";
			}
			for (const u of rp.load_balancing.upstreams) {
				const err = validateUpstream(u);
				if (err) return `Additional upstream: ${err.toLowerCase()}`;
			}
		}
		return null;
	};

	const validateStep = (): boolean => {
		setError("");
		if (step === 0) {
			const domainErr = validateDomain(data.name);
			if (domainErr) {
				setError(domainErr);
				return false;
			}
			if (existingDomains.some((d) => d.toLowerCase() === data.name.trim().toLowerCase())) {
				setError("A domain with this name already exists");
				return false;
			}
		}
		if (step === 1) {
			const toggleErr = validateToggles(data.toggles);
			if (toggleErr) {
				setError(toggleErr);
				return false;
			}
		}
		if (step === 2) {
			const ht = data.rootRule.handlerType;
			if (ht !== "none" && ht !== "reverse_proxy") {
				setError("This handler type is not yet supported");
				return false;
			}
			if (ht === "reverse_proxy") {
				const rpErr = validateReverseProxy(data.rootRule.handlerConfig as ReverseProxyConfig);
				if (rpErr) {
					setError(rpErr);
					return false;
				}
			}
		}
		return true;
	};

	const handleNext = () => {
		if (!validateStep()) return;
		setError("");
		setStep((s) => s + 1);
		if (step === 2) {
			resetRuleForm();
			setRuleFormActive(false);
		}
	};

	const handleBack = () => {
		setError("");
		setStep((s) => s - 1);
	};

	const handleStepClick = (targetStep: number) => {
		if (targetStep < step) {
			setError("");
			setStep(targetStep);
		}
	};

	const addRule = () => {
		setError("");
		if (ruleMatchType === "subdomain" && !ruleMatchValue.trim()) {
			setError("Subdomain value is required");
			return;
		}
		if (ruleMatchType === "path" && !ruleMatchValue.trim()) {
			setError("Path value is required");
			return;
		}
		if (ruleHandlerType !== "reverse_proxy") {
			setError("This handler type is not yet supported");
			return;
		}
		const rp = ruleHandlerConfig as ReverseProxyConfig;
		const rpErr = validateReverseProxy(rp);
		if (rpErr) {
			setError(rpErr);
			return;
		}
		if (ruleOverridesOpen) {
			const toggleErr = validateToggles(ruleToggleOverrides);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}

		ruleKeyRef.current += 1;
		const newRule: WizardRule = {
			key: ruleKeyRef.current,
			matchType: ruleMatchType,
			pathMatch: rulePathMatch,
			matchValue: ruleMatchValue.trim(),
			handlerType: ruleHandlerType,
			handlerConfig: ruleHandlerConfig,
			toggleOverrides: ruleOverridesOpen ? ruleToggleOverrides : null,
		};
		setData((prev) => ({ ...prev, rules: [...prev.rules, newRule] }));
		setRuleFormActive(false);
	};

	const removeRule = (index: number) => {
		setData((prev) => ({ ...prev, rules: prev.rules.filter((_, i) => i !== index) }));
	};

	const handleSubmit = async () => {
		setSubmitting(true);
		setError("");

		const rules: CreateDomainFullRequest["rules"] = [];

		if (data.rootRule.handlerType !== "none") {
			rules.push({
				match_type: "",
				handler_type: data.rootRule.handlerType as HandlerType,
				handler_config: data.rootRule.handlerConfig,
			});
		}

		for (const rule of data.rules) {
			rules.push({
				match_type: rule.matchType,
				...(rule.matchType === "path" ? { path_match: rule.pathMatch } : {}),
				match_value: rule.matchValue,
				handler_type: rule.handlerType,
				handler_config: rule.handlerConfig,
				toggle_overrides: rule.toggleOverrides,
			});
		}

		const req: CreateDomainFullRequest = {
			name: data.name.trim(),
			toggles: data.toggles,
			rules,
		};

		try {
			await onCreate(req);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to create domain"));
		} finally {
			setSubmitting(false);
		}
	};

	const rootRuleSupported =
		data.rootRule.handlerType === "none" || data.rootRule.handlerType === "reverse_proxy";

	return (
		<div className="domain-wizard">
			<div className="wizard-steps">
				{STEP_LABELS.map((label, i) => (
					<button
						type="button"
						key={label}
						className={`wizard-step${i === step ? " active" : ""}${i < step ? " completed" : ""}`}
						onClick={() => handleStepClick(i)}
						tabIndex={i < step ? 0 : -1}
						disabled={i >= step}
					>
						{i > 0 && <div className="wizard-step-connector" />}
						<div className="wizard-step-number">{i + 1}</div>
						<div className="wizard-step-label">{label}</div>
					</button>
				))}
			</div>

			{error && (
				<div className="inline-error" role="alert">
					{error}
				</div>
			)}

			<div className="wizard-step-content">
				{step === 0 && (
					<div className="wizard-section">
						<div className="form-row">
							<div className="form-field">
								<label htmlFor="wizard-domain-name">Domain</label>
								<input
									id="wizard-domain-name"
									type="text"
									placeholder="example.com"
									value={data.name}
									onChange={(e) => setData((prev) => ({ ...prev, name: e.target.value }))}
									maxLength={253}
									required
								/>
							</div>
						</div>
					</div>
				)}

				{step === 1 && (
					<div className="wizard-section">
						<DomainToggleGrid
							toggles={data.toggles}
							onUpdate={(key, value) =>
								setData((prev) => ({ ...prev, toggles: { ...prev.toggles, [key]: value } }))
							}
							idPrefix="wizard-domain"
							domain={data.name}
						/>
					</div>
				)}

				{step === 2 && (
					<div className="wizard-section">
						<div className="form-row">
							<div className="form-field">
								<span className="form-label">Handler</span>
								<Toggle
									options={handlerOptions}
									value={data.rootRule.handlerType}
									onChange={(next: HandlerSelection) => {
										setData((prev) => ({
											...prev,
											rootRule: {
												handlerType: next,
												handlerConfig:
													next === "reverse_proxy" ? { ...defaultReverseProxyConfig } : {},
											},
										}));
									}}
								/>
							</div>
						</div>

						{data.rootRule.handlerType !== "none" &&
							data.rootRule.handlerType !== "reverse_proxy" && (
								<div className="alert-warning" role="status">
									This handler type is not yet supported. Only reverse proxy is available for now.
								</div>
							)}

						{data.rootRule.handlerType === "reverse_proxy" && (
							<HandlerConfig
								type="reverse_proxy"
								config={data.rootRule.handlerConfig}
								onChange={(config) =>
									setData((prev) => ({
										...prev,
										rootRule: { ...prev.rootRule, handlerConfig: config },
									}))
								}
								disabled={false}
							/>
						)}
					</div>
				)}

				{step === 3 && (
					<div className="wizard-section">
						{data.rules.length > 0 && (
							<div className="wizard-rule-list">
								{data.rules.map((rule, i) => (
									<div key={rule.key} className="wizard-rule-summary">
										<div className="wizard-rule-summary-info">
											<span className="wizard-rule-match">
												{rule.matchType === "subdomain"
													? `${rule.matchValue}.${data.name}`
													: `${data.name}${rule.matchValue}`}
											</span>
											<span className="rule-card-handler-badge handler-reverse_proxy">
												{rule.handlerType.replace("_", " ")}
											</span>
											{rule.handlerType === "reverse_proxy" && (
												<span className="wizard-rule-upstream">
													{(rule.handlerConfig as ReverseProxyConfig).upstream}
												</span>
											)}
										</div>
										<button
											type="button"
											className="btn btn-ghost btn-sm"
											onClick={() => removeRule(i)}
										>
											Remove
										</button>
									</div>
								))}
							</div>
						)}

						{ruleFormActive ? (
							<div className="wizard-rule-form">
								<div className="form-row">
									<div className="form-field">
										<span className="form-label">Match Type</span>
										<Toggle
											options={ruleMatchOptions}
											value={ruleMatchType}
											onChange={setRuleMatchType}
										/>
									</div>
								</div>

								{ruleMatchType === "subdomain" && (
									<div className="form-row">
										<div className="form-field">
											<label htmlFor="wizard-rule-match-value">Subdomain</label>
											<input
												id="wizard-rule-match-value"
												type="text"
												placeholder="api"
												value={ruleMatchValue}
												onChange={(e) => setRuleMatchValue(e.target.value)}
												maxLength={63}
												required
											/>
										</div>
									</div>
								)}

								{ruleMatchType === "path" && (
									<div className="form-row">
										<div className="form-field">
											<label htmlFor="wizard-rule-path-match">Path Match</label>
											<Toggle
												options={pathMatchOptions}
												value={rulePathMatch}
												onChange={setRulePathMatch}
											/>
										</div>
										<div className="form-field">
											<label htmlFor="wizard-rule-path-value">Path</label>
											<input
												id="wizard-rule-path-value"
												type="text"
												placeholder={
													rulePathMatch === "prefix"
														? "/api/"
														: rulePathMatch === "exact"
															? "/api/v1/health"
															: "^/api/.*"
												}
												value={ruleMatchValue}
												onChange={(e) => setRuleMatchValue(e.target.value)}
												maxLength={253}
												required
											/>
										</div>
									</div>
								)}

								<div className="form-row">
									<div className="form-field">
										<span className="form-label">Handler Type</span>
										<Toggle
											options={handlerTypeOptions}
											value={ruleHandlerType}
											onChange={(next: HandlerType) => {
												setRuleHandlerType(next);
												if (next === "reverse_proxy") {
													setRuleHandlerConfig({ ...defaultReverseProxyConfig });
												} else {
													setRuleHandlerConfig({});
												}
											}}
										/>
									</div>
								</div>

								{ruleHandlerType !== "reverse_proxy" && (
									<div className="alert-warning" role="status">
										This handler type is not yet supported.
									</div>
								)}

								{ruleHandlerType === "reverse_proxy" && (
									<HandlerConfig
										type="reverse_proxy"
										config={ruleHandlerConfig}
										onChange={setRuleHandlerConfig}
										disabled={false}
									/>
								)}

								<div className="rule-form-overrides">
									<button
										type="button"
										className="btn btn-ghost rule-form-overrides-toggle"
										onClick={() => setRuleOverridesOpen(!ruleOverridesOpen)}
									>
										<span className={ruleOverridesOpen ? "chevron open" : "chevron"} />
										Override domain toggles
									</button>
									{ruleOverridesOpen && (
										<div className="rule-form-overrides-body">
											<DomainToggleGrid
												toggles={ruleToggleOverrides}
												onUpdate={(key, value) =>
													setRuleToggleOverrides((prev) => ({
														...prev,
														[key]: value,
													}))
												}
												idPrefix="wizard-rule-override"
												domain={data.name}
											/>
										</div>
									)}
								</div>

								<div className="wizard-rule-form-actions">
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => setRuleFormActive(false)}
									>
										Cancel
									</button>
									<button
										type="button"
										className="btn btn-primary"
										onClick={addRule}
										disabled={ruleHandlerType !== "reverse_proxy"}
									>
										Add Rule
									</button>
								</div>
							</div>
						) : (
							<div className="wizard-add-rule-prompt">
								{data.rules.length > 0 ? (
									<>
										<span>Add another rule?</span>
										<div className="wizard-add-rule-actions">
											<button
												type="button"
												className="btn btn-ghost"
												onClick={() => {
													resetRuleForm();
													setRuleFormActive(true);
												}}
											>
												Add Another
											</button>
											<button type="button" className="btn btn-primary" onClick={() => setStep(4)}>
												Continue to Review
											</button>
										</div>
									</>
								) : (
									<>
										<div className="wizard-add-rule-empty">
											No rules added yet. Add subdomain or path rules, or skip to review.
										</div>
										<div className="wizard-add-rule-actions">
											<button
												type="button"
												className="btn btn-primary"
												onClick={() => {
													resetRuleForm();
													setRuleFormActive(true);
												}}
											>
												Add Rule
											</button>
										</div>
									</>
								)}
							</div>
						)}
					</div>
				)}

				{step === 4 && <WizardReview data={data} onEditStep={setStep} />}
			</div>

			<div className="wizard-nav">
				{step === 0 ? (
					<button type="button" className="btn btn-ghost" onClick={onCancel} disabled={submitting}>
						Cancel
					</button>
				) : (
					<button
						type="button"
						className="btn btn-ghost"
						onClick={handleBack}
						disabled={submitting}
					>
						Back
					</button>
				)}

				<div className="wizard-nav-right">
					{step < 3 && (
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleNext}
							disabled={submitting || (step === 2 && !rootRuleSupported)}
						>
							Next
						</button>
					)}
					{step === 3 && !ruleFormActive && data.rules.length === 0 && (
						<button type="button" className="btn btn-ghost" onClick={() => setStep(4)}>
							Skip to Review
						</button>
					)}
					{step === 3 && !ruleFormActive && data.rules.length > 0 && (
						<button type="button" className="btn btn-primary" onClick={() => setStep(4)}>
							Continue to Review
						</button>
					)}
					{step === 4 && (
						<button
							type="button"
							className="btn btn-primary submit-btn"
							onClick={handleSubmit}
							disabled={submitting}
						>
							{submitting ? "Creating..." : "Create Domain"}
						</button>
					)}
				</div>
			</div>
		</div>
	);
}
