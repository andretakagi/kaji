import { useCallback, useRef, useState } from "react";
import { createDomainFull, createSubdomain, createSubdomainRule } from "../api";
import type {
	CreateDomainFullRequest,
	CreateRuleRequest,
	CreateSubdomainRequest,
	Domain,
	DomainToggles,
	FileServerConfig,
	HandlerConfigValue,
	HandlerType,
	MatchType,
	PathMatch,
	RedirectConfig,
	ReverseProxyConfig,
	StaticResponseConfig,
	SubdomainHandlerType,
} from "../types/domain";
import {
	defaultDomainToggles,
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
} from "../types/domain";
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
	handlerConfig: HandlerConfigValue;
	toggleOverrides: DomainToggles | null;
}

export interface WizardSubdomain {
	key: number;
	prefix: string;
	handlerType: HandlerSelection;
	handlerConfig: HandlerConfigValue;
	toggles: DomainToggles;
	rules: WizardRule[];
}

export interface WizardData {
	name: string;
	toggles: DomainToggles;
	rootRule: {
		handlerType: HandlerSelection;
		handlerConfig: HandlerConfigValue;
	};
	subdomains: WizardSubdomain[];
	rules: WizardRule[];
}

const STEP_LABELS = ["URL", "Root Domain", "Subdomains", "Path Rules", "Review"];

const handlerOptions: readonly { value: HandlerSelection; label: string }[] = [
	{ value: "none", label: "None" },
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
] as const;

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

function parseUrl(input: string): {
	domain: string;
	subPrefix: string | null;
	path: string | null;
} {
	const trimmed = input.trim().replace(/^https?:\/\//, "");
	const slashIdx = trimmed.indexOf("/");
	const host = slashIdx >= 0 ? trimmed.slice(0, slashIdx) : trimmed;
	const path = slashIdx >= 0 ? trimmed.slice(slashIdx) : null;
	const parts = host.split(".");
	if (parts.length >= 3) {
		return {
			domain: parts.slice(1).join("."),
			subPrefix: parts[0],
			path,
		};
	}
	return { domain: host, subPrefix: null, path };
}

interface Props {
	onCreate: (req: CreateDomainFullRequest) => Promise<void>;
	onCancel: () => void;
	onReload: () => Promise<void>;
	existingDomains: Domain[];
}

export default function DomainWizard({ onCreate, onCancel, onReload, existingDomains }: Props) {
	const ruleKeyRef = useRef(0);
	const subKeyRef = useRef(0);
	const [step, setStep] = useState(0);
	const [highestStep, setHighestStep] = useState(0);
	const [submitting, setSubmitting] = useState(false);
	const [error, setError] = useState("");
	const [urlInput, setUrlInput] = useState("");

	const [data, setData] = useState<WizardData>({
		name: "",
		toggles: { ...defaultDomainToggles },
		rootRule: {
			handlerType: "none",
			handlerConfig: { ...defaultReverseProxyConfig },
		},
		subdomains: [],
		rules: [],
	});

	// Path rule form state
	const [ruleFormActive, setRuleFormActive] = useState(false);
	const [editingRuleId, setEditingRuleId] = useState<string | null>(null);
	const [ruleTarget, setRuleTarget] = useState("");
	const [ruleMatchType, setRuleMatchType] = useState<Exclude<MatchType, "">>("path");
	const [rulePathMatch, setRulePathMatch] = useState<PathMatch>("prefix");
	const [ruleMatchValue, setRuleMatchValue] = useState("");
	const [ruleHandlerType, setRuleHandlerType] = useState<HandlerType>("reverse_proxy");
	const [ruleHandlerConfig, setRuleHandlerConfig] = useState<HandlerConfigValue>({
		...defaultReverseProxyConfig,
	});
	const [ruleOverridesOpen, setRuleOverridesOpen] = useState(false);
	const [ruleToggleOverrides, setRuleToggleOverrides] = useState<DomainToggles>({
		...defaultDomainToggles,
	});

	// Subdomain form state
	const [subFormActive, setSubFormActive] = useState(false);
	const [editingSubIndex, setEditingSubIndex] = useState<number | null>(null);
	const [subPrefix, setSubPrefix] = useState("");
	const [subHandlerType, setSubHandlerType] = useState<HandlerSelection>("none");
	const [subHandlerConfig, setSubHandlerConfig] = useState<HandlerConfigValue>({
		...defaultReverseProxyConfig,
	});
	const [subTogglesOpen, setSubTogglesOpen] = useState(false);
	const [subToggles, setSubToggles] = useState<DomainToggles>({ ...defaultDomainToggles });

	const resetSubForm = useCallback(() => {
		setSubPrefix("");
		setSubHandlerType("none");
		setSubHandlerConfig({ ...defaultReverseProxyConfig });
		setSubTogglesOpen(false);
		setSubToggles({ ...data.toggles });
	}, [data.toggles]);

	const resetRuleForm = useCallback(() => {
		setRuleTarget("");
		setRuleMatchType("path");
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

	const validateHandlerConfig = (
		ht: HandlerSelection,
		config: HandlerConfigValue,
	): string | null => {
		if (ht === "reverse_proxy") {
			return validateReverseProxy(config as ReverseProxyConfig);
		}
		if (ht === "static_response") {
			const sr = config as StaticResponseConfig;
			if (!sr.close && sr.status_code) {
				const code = Number.parseInt(sr.status_code, 10);
				if (Number.isNaN(code) || code < 100 || code > 599) {
					return "Status code must be between 100 and 599";
				}
			}
		}
		if (ht === "redirect") {
			const rd = config as RedirectConfig;
			if (!rd.target_url.trim()) return "Target URL is required";
			if (rd.status_code) {
				const code = Number.parseInt(rd.status_code, 10);
				if (Number.isNaN(code) || code < 100 || code > 599) {
					return "Status code must be between 100 and 599";
				}
			}
		}
		if (ht === "file_server") {
			const fs = config as FileServerConfig;
			if (!fs.root.trim()) return "Root directory is required";
		}
		return null;
	};

	const isSupportedHandler = (ht: HandlerSelection): boolean =>
		ht === "none" ||
		ht === "reverse_proxy" ||
		ht === "static_response" ||
		ht === "redirect" ||
		ht === "file_server";

	const validateStep = (): boolean => {
		setError("");
		if (step === 0) {
			const parsed = parseUrl(urlInput);
			const domainErr = validateDomain(parsed.domain);
			if (domainErr) {
				setError(domainErr);
				return false;
			}
			const existing = existingDomains.find(
				(d) => d.name.toLowerCase() === parsed.domain.toLowerCase(),
			);
			if (existing && !parsed.subPrefix && !parsed.path) {
				setError("A domain with this name already exists");
				return false;
			}
			if (parsed.subPrefix && existing) {
				const sub = parsed.subPrefix;
				const existingSub = existing.subdomains?.some(
					(s) => s.name.toLowerCase() === sub.toLowerCase(),
				);
				if (existingSub && !parsed.path) {
					setError(`Subdomain "${parsed.subPrefix}" already exists under ${parsed.domain}`);
					return false;
				}
			}
		}
		if (step === 1) {
			const ht = data.rootRule.handlerType;
			if (!isSupportedHandler(ht)) {
				setError("This handler type is not yet supported");
				return false;
			}
			if (ht !== "none") {
				const err = validateHandlerConfig(ht, data.rootRule.handlerConfig);
				if (err) {
					setError(err);
					return false;
				}
			}
			const toggleErr = validateToggles(data.toggles);
			if (toggleErr) {
				setError(toggleErr);
				return false;
			}
		}
		return true;
	};

	const goToStep = (target: number) => {
		setStep(target);
		setHighestStep((prev) => Math.max(prev, target));
	};

	const handleNext = () => {
		if (!validateStep()) return;
		setError("");

		if (step === 0) {
			const parsed = parseUrl(urlInput);
			setData((prev) => ({ ...prev, name: parsed.domain }));

			if (parsed.subPrefix && parsed.path) {
				// subdomain + path: auto-create subdomain, jump to path rules
				const prefix = parsed.subPrefix;
				const pathValue = parsed.path;
				subKeyRef.current += 1;
				setData((prev) => ({
					...prev,
					name: parsed.domain,
					subdomains: prev.subdomains.some((s) => s.prefix.toLowerCase() === prefix.toLowerCase())
						? prev.subdomains
						: [
								...prev.subdomains,
								{
									key: subKeyRef.current,
									prefix,
									handlerType: "none" as HandlerSelection,
									handlerConfig: {},
									toggles: { ...defaultDomainToggles },
									rules: [],
								},
							],
				}));
				setRuleTarget(prefix);
				setRuleMatchValue(pathValue);
				setRuleFormActive(true);
				goToStep(3);
			} else if (parsed.subPrefix) {
				// subdomain only: open sub form pre-filled, jump to subdomains
				setSubPrefix(parsed.subPrefix);
				setSubFormActive(true);
				goToStep(2);
			} else if (parsed.path) {
				// path only: jump to path rules
				setRuleTarget("");
				setRuleMatchValue(parsed.path);
				setRuleFormActive(true);
				goToStep(3);
			} else {
				// root domain only: go to root domain config
				goToStep(1);
			}
			return;
		}

		if (step === 2) {
			resetRuleForm();
			setRuleFormActive(false);
		}
		goToStep(step + 1);
	};

	const handleBack = () => {
		setError("");
		setStep((s) => s - 1);
	};

	const handleStepClick = (targetStep: number) => {
		if (targetStep < step || targetStep <= highestStep) {
			setError("");
			setStep(targetStep);
		}
	};

	// --- Rule helpers (path rules, step 3) ---

	const addRule = () => {
		setError("");
		if (ruleMatchType === "path" && !ruleMatchValue.trim()) {
			setError("Path value is required");
			return;
		}
		if (!isSupportedHandler(ruleHandlerType)) {
			setError("This handler type is not yet supported");
			return;
		}
		const handlerErr = validateHandlerConfig(ruleHandlerType, ruleHandlerConfig);
		if (handlerErr) {
			setError(handlerErr);
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

		if (ruleTarget === "") {
			setData((prev) => ({ ...prev, rules: [...prev.rules, newRule] }));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === ruleTarget.toLowerCase()
						? { ...s, rules: [...s.rules, newRule] }
						: s,
				),
			}));
		}
		setRuleFormActive(false);
		resetRuleForm();
	};

	const removeRule = (target: string, index: number) => {
		if (target === "") {
			setData((prev) => ({ ...prev, rules: prev.rules.filter((_, i) => i !== index) }));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === target.toLowerCase()
						? { ...s, rules: s.rules.filter((_, i) => i !== index) }
						: s,
				),
			}));
		}
	};

	// --- Subdomain rule helpers (nested in subdomain form, step 2) ---

	// --- Subdomain helpers (step 2) ---

	const addSubdomain = () => {
		setError("");
		const trimmed = subPrefix.trim().toLowerCase();
		if (!trimmed) {
			setError("Subdomain prefix is required");
			return;
		}
		if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/i.test(trimmed)) {
			setError("Invalid subdomain prefix");
			return;
		}
		if (data.subdomains.some((s) => s.prefix.toLowerCase() === trimmed)) {
			setError(`Subdomain "${trimmed}" already added`);
			return;
		}
		if (subHandlerType !== "none") {
			const handlerErr = validateHandlerConfig(subHandlerType, subHandlerConfig);
			if (handlerErr) {
				setError(handlerErr);
				return;
			}
		}
		if (subTogglesOpen) {
			const toggleErr = validateToggles(subToggles);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}
		subKeyRef.current += 1;
		const newSub: WizardSubdomain = {
			key: subKeyRef.current,
			prefix: trimmed,
			handlerType: subHandlerType,
			handlerConfig: subHandlerType === "none" ? {} : subHandlerConfig,
			toggles: subTogglesOpen ? subToggles : { ...data.toggles },
			rules: [],
		};
		setData((prev) => ({ ...prev, subdomains: [...prev.subdomains, newSub] }));
		setSubFormActive(false);
		resetSubForm();
	};

	const removeSubdomain = (index: number) => {
		setData((prev) => ({
			...prev,
			subdomains: prev.subdomains.filter((_, i) => i !== index),
		}));
	};

	const startEditSub = (index: number) => {
		const sub = data.subdomains[index];
		setSubPrefix(sub.prefix);
		setSubHandlerType(sub.handlerType);
		setSubHandlerConfig(sub.handlerConfig);
		setSubToggles(sub.toggles);
		setSubTogglesOpen(false);
		setEditingSubIndex(index);
		setSubFormActive(true);
	};

	const saveEditSub = () => {
		if (editingSubIndex === null) return;
		setError("");
		const trimmed = subPrefix.trim().toLowerCase();
		if (!trimmed) {
			setError("Subdomain prefix is required");
			return;
		}
		if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/i.test(trimmed)) {
			setError("Invalid subdomain prefix");
			return;
		}
		const duplicate = data.subdomains.some(
			(s, i) => i !== editingSubIndex && s.prefix.toLowerCase() === trimmed,
		);
		if (duplicate) {
			setError(`Subdomain "${trimmed}" already added`);
			return;
		}
		if (subHandlerType !== "none") {
			const handlerErr = validateHandlerConfig(subHandlerType, subHandlerConfig);
			if (handlerErr) {
				setError(handlerErr);
				return;
			}
		}
		if (subTogglesOpen) {
			const toggleErr = validateToggles(subToggles);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}
		setData((prev) => ({
			...prev,
			subdomains: prev.subdomains.map((s, i) =>
				i === editingSubIndex
					? {
							...s,
							prefix: trimmed,
							handlerType: subHandlerType,
							handlerConfig: subHandlerType === "none" ? {} : subHandlerConfig,
							toggles: subTogglesOpen ? subToggles : s.toggles,
						}
					: s,
			),
		}));
		setEditingSubIndex(null);
		setSubFormActive(false);
		resetSubForm();
	};

	const cancelEditSub = () => {
		setEditingSubIndex(null);
		setSubFormActive(false);
		resetSubForm();
	};

	const startEditRule = (target: string, index: number) => {
		const rules =
			target === ""
				? data.rules
				: data.subdomains.find((s) => s.prefix.toLowerCase() === target.toLowerCase())?.rules;
		if (!rules) return;
		const rule = rules[index];
		setRuleTarget(target);
		setRuleMatchType(rule.matchType);
		setRulePathMatch(rule.pathMatch);
		setRuleMatchValue(rule.matchValue);
		setRuleHandlerType(rule.handlerType);
		setRuleHandlerConfig(rule.handlerConfig);
		setRuleOverridesOpen(rule.toggleOverrides !== null);
		setRuleToggleOverrides(rule.toggleOverrides ?? { ...data.toggles });
		setEditingRuleId(`${target}-${index}`);
		setRuleFormActive(true);
	};

	const saveEditRule = () => {
		if (editingRuleId === null) return;
		setError("");
		if (ruleMatchType === "path" && !ruleMatchValue.trim()) {
			setError("Path value is required");
			return;
		}
		const handlerErr = validateHandlerConfig(ruleHandlerType, ruleHandlerConfig);
		if (handlerErr) {
			setError(handlerErr);
			return;
		}
		if (ruleOverridesOpen) {
			const toggleErr = validateToggles(ruleToggleOverrides);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}
		const parts = editingRuleId.split("-");
		const editIndex = Number.parseInt(parts[parts.length - 1], 10);
		const editTarget = parts.slice(0, -1).join("-");
		const updated: WizardRule = {
			key: 0,
			matchType: ruleMatchType,
			pathMatch: rulePathMatch,
			matchValue: ruleMatchValue.trim(),
			handlerType: ruleHandlerType,
			handlerConfig: ruleHandlerConfig,
			toggleOverrides: ruleOverridesOpen ? ruleToggleOverrides : null,
		};
		if (editTarget === "") {
			setData((prev) => ({
				...prev,
				rules: prev.rules.map((r, i) => (i === editIndex ? { ...updated, key: r.key } : r)),
			}));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === editTarget.toLowerCase()
						? {
								...s,
								rules: s.rules.map((r, i) => (i === editIndex ? { ...updated, key: r.key } : r)),
							}
						: s,
				),
			}));
		}
		setEditingRuleId(null);
		setRuleFormActive(false);
		resetRuleForm();
	};

	const cancelEditRule = () => {
		setEditingRuleId(null);
		setRuleFormActive(false);
		resetRuleForm();
	};

	// --- Submit ---

	const handleSubmit = async () => {
		setSubmitting(true);
		setError("");

		try {
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

			const existingDomain = existingDomains.find(
				(d) => d.name.toLowerCase() === data.name.trim().toLowerCase(),
			);

			if (data.subdomains.length > 0) {
				let domainId: string;
				if (existingDomain) {
					domainId = existingDomain.id;
				} else {
					const created = await createDomainFull(req);
					domainId = created.id;
				}

				for (const sub of data.subdomains) {
					const existingSub = existingDomain?.subdomains?.find(
						(s) => s.name.toLowerCase() === sub.prefix.toLowerCase(),
					);
					if (existingSub) {
						for (const rule of sub.rules) {
							await createSubdomainRule(domainId, existingSub.id, buildRuleReq(rule));
						}
					} else {
						const ht = sub.handlerType as SubdomainHandlerType;
						const subReq: CreateSubdomainRequest = {
							name: sub.prefix,
							handler_type: ht,
							handler_config: ht === "none" ? null : sub.handlerConfig,
							toggles: sub.toggles,
						};
						const updated = await createSubdomain(domainId, subReq);
						const createdSub = updated.subdomains.find(
							(s) => s.name.toLowerCase() === sub.prefix.toLowerCase(),
						);
						if (createdSub && sub.rules.length > 0) {
							for (const rule of sub.rules) {
								await createSubdomainRule(domainId, createdSub.id, buildRuleReq(rule));
							}
						}
					}
				}
				await onReload();
				onCancel();
			} else if (existingDomain) {
				await onReload();
				onCancel();
			} else {
				await onCreate(req);
			}
		} catch (err) {
			setError(getErrorMessage(err, "Failed to create domain"));
		} finally {
			setSubmitting(false);
		}
	};

	// --- Collect all path rules for display in step 3 ---

	const allRules: { target: string; targetLabel: string; rule: WizardRule; index: number }[] = [];
	for (let i = 0; i < data.rules.length; i++) {
		allRules.push({ target: "", targetLabel: data.name, rule: data.rules[i], index: i });
	}
	for (const sub of data.subdomains) {
		for (let i = 0; i < sub.rules.length; i++) {
			allRules.push({
				target: sub.prefix,
				targetLabel: `${sub.prefix}.${data.name}`,
				rule: sub.rules[i],
				index: i,
			});
		}
	}

	// --- Target options for rule form dropdown ---

	const ruleTargetOptions = [
		{ value: "", label: data.name || "Root domain" },
		...data.subdomains.map((s) => ({
			value: s.prefix,
			label: `${s.prefix}.${data.name}`,
		})),
	];

	const rootRuleSupported = isSupportedHandler(data.rootRule.handlerType);

	return (
		<div className="domain-wizard">
			<div className="wizard-steps">
				{STEP_LABELS.map((label, i) => (
					<button
						type="button"
						key={label}
						className={`wizard-step${i === step ? " active" : ""}${i < step ? " completed" : ""}`}
						onClick={() => handleStepClick(i)}
						tabIndex={i < step || i <= highestStep ? 0 : -1}
						disabled={i > step && i > highestStep}
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
				{/* Step 0: URL */}
				{step === 0 && (
					<div className="wizard-section">
						<div className="form-row">
							<div className="form-field">
								<label htmlFor="wizard-url-input">URL</label>
								<input
									id="wizard-url-input"
									type="text"
									placeholder="example.com, api.example.com, example.com/api"
									value={urlInput}
									onChange={(e) => setUrlInput(e.target.value)}
									maxLength={500}
									required
								/>
							</div>
						</div>
						{(() => {
							const parsed = parseUrl(urlInput);
							if (!parsed.subPrefix && !parsed.path) return null;
							const parts: string[] = [];
							if (parsed.subPrefix) {
								parts.push(`subdomain "${parsed.subPrefix}" under ${parsed.domain}`);
							}
							if (parsed.path) {
								const pathTarget = parsed.subPrefix
									? `${parsed.subPrefix}.${parsed.domain}`
									: parsed.domain;
								parts.push(`path rule "${parsed.path}" on ${pathTarget}`);
							}
							const parentExists = existingDomains.some(
								(d) => d.name.toLowerCase() === parsed.domain.toLowerCase(),
							);
							return (
								<div className="wizard-subdomain-hint">
									This will create {parts.join(" and ")}
									{!parentExists && parsed.subPrefix && ` (${parsed.domain} will be auto-created)`}
								</div>
							);
						})()}
					</div>
				)}

				{/* Step 1: Root Domain (handler + toggles) */}
				{step === 1 && (
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
													next === "reverse_proxy"
														? { ...defaultReverseProxyConfig }
														: next === "static_response"
															? { ...defaultStaticResponseConfig }
															: next === "redirect"
																? { ...defaultRedirectConfig }
																: next === "file_server"
																	? { ...defaultFileServerConfig }
																	: {},
											},
										}));
									}}
								/>
							</div>
						</div>

						{!isSupportedHandler(data.rootRule.handlerType) && (
							<div className="alert-warning" role="status">
								This handler type is not yet supported.
							</div>
						)}

						{data.rootRule.handlerType !== "none" &&
							isSupportedHandler(data.rootRule.handlerType) && (
								<HandlerConfig
									type={data.rootRule.handlerType as HandlerType}
									config={data.rootRule.handlerConfig}
									onChange={(config) =>
										setData((prev) => ({
											...prev,
											rootRule: { ...prev.rootRule, handlerConfig: config },
										}))
									}
									disabled={false}
									domain={data.name}
								/>
							)}

						<div className="wizard-step-divider" />
						<span className="form-label">Toggles</span>
						<DomainToggleGrid
							toggles={data.toggles}
							onUpdate={(key, value) =>
								setData((prev) => ({
									...prev,
									toggles: { ...prev.toggles, [key]: value },
								}))
							}
							idPrefix="wizard-domain"
							domain={data.name}
							hideResponseHeaders={data.rootRule.handlerType === "static_response"}
						/>
					</div>
				)}

				{/* Step 2: Subdomains */}
				{step === 2 && (
					<div className="wizard-section">
						{data.subdomains.length > 0 && (
							<div className="wizard-rule-list">
								{data.subdomains.map((sub, i) => (
									<div key={sub.key} className="wizard-editable-card">
										<button
											type="button"
											className="wizard-editable-card-header"
											onClick={() => (editingSubIndex === i ? cancelEditSub() : startEditSub(i))}
										>
											<div className="wizard-rule-summary-info">
												<span className="wizard-rule-match">
													{sub.prefix}.{data.name}
												</span>
												{sub.handlerType !== "none" ? (
													<span className={`rule-card-handler-badge handler-${sub.handlerType}`}>
														{sub.handlerType.replace("_", " ")}
													</span>
												) : (
													<span className="rule-card-handler-badge handler-none">no handler</span>
												)}
												{sub.rules.length > 0 && (
													<span className="wizard-rule-upstream">
														{sub.rules.length} rule
														{sub.rules.length !== 1 ? "s" : ""}
													</span>
												)}
											</div>
											<span className={editingSubIndex === i ? "chevron open" : "chevron"} />
										</button>
										{editingSubIndex === i && subFormActive && (
											<div className="wizard-editable-card-body">
												<div className="form-row">
													<div className="form-field">
														<label htmlFor="wizard-sub-prefix">Subdomain</label>
														<div className="wizard-sub-input-row">
															<input
																id="wizard-sub-prefix"
																type="text"
																placeholder="api"
																value={subPrefix}
																onChange={(e) => setSubPrefix(e.target.value)}
																maxLength={63}
																required
															/>
															<span className="wizard-sub-suffix">.{data.name}</span>
														</div>
													</div>
												</div>

												<div className="form-row">
													<div className="form-field">
														<span className="form-label">Handler</span>
														<Toggle
															options={handlerOptions}
															value={subHandlerType}
															onChange={(next: HandlerSelection) => {
																setSubHandlerType(next);
																if (next === "reverse_proxy") {
																	setSubHandlerConfig({
																		...defaultReverseProxyConfig,
																	});
																} else if (next === "static_response") {
																	setSubHandlerConfig({
																		...defaultStaticResponseConfig,
																	});
																} else if (next === "redirect") {
																	setSubHandlerConfig({
																		...defaultRedirectConfig,
																	});
																} else if (next === "file_server") {
																	setSubHandlerConfig({
																		...defaultFileServerConfig,
																	});
																} else {
																	setSubHandlerConfig({});
																}
															}}
														/>
													</div>
												</div>

												{subHandlerType !== "none" && isSupportedHandler(subHandlerType) && (
													<HandlerConfig
														type={subHandlerType as HandlerType}
														config={subHandlerConfig}
														onChange={setSubHandlerConfig}
														disabled={false}
														domain={`${subPrefix || "sub"}.${data.name}`}
													/>
												)}

												<div className="rule-form-overrides">
													<button
														type="button"
														className="btn btn-ghost rule-form-overrides-toggle"
														onClick={() => setSubTogglesOpen(!subTogglesOpen)}
													>
														<span className={subTogglesOpen ? "chevron open" : "chevron"} />
														Toggles
													</button>
													{subTogglesOpen && (
														<div className="rule-form-overrides-body">
															<DomainToggleGrid
																toggles={subToggles}
																onUpdate={(key, value) =>
																	setSubToggles((prev) => ({
																		...prev,
																		[key]: value,
																	}))
																}
																idPrefix="wizard-sub-toggle"
																domain={`${subPrefix || "sub"}.${data.name}`}
																hideResponseHeaders={subHandlerType === "static_response"}
															/>
														</div>
													)}
												</div>

												<div className="wizard-rule-form-actions">
													<button
														type="button"
														className="btn btn-ghost btn-sm"
														onClick={() => {
															removeSubdomain(i);
															cancelEditSub();
														}}
													>
														Remove
													</button>
													<div className="wizard-rule-form-actions-right">
														<button type="button" className="btn btn-ghost" onClick={cancelEditSub}>
															Cancel
														</button>
														<button type="button" className="btn btn-primary" onClick={saveEditSub}>
															Save
														</button>
													</div>
												</div>
											</div>
										)}
									</div>
								))}
							</div>
						)}

						{subFormActive && editingSubIndex === null ? (
							<div className="wizard-rule-form">
								<div className="form-row">
									<div className="form-field">
										<label htmlFor="wizard-sub-prefix">Subdomain</label>
										<div className="wizard-sub-input-row">
											<input
												id="wizard-sub-prefix"
												type="text"
												placeholder="api"
												value={subPrefix}
												onChange={(e) => setSubPrefix(e.target.value)}
												maxLength={63}
												required
											/>
											<span className="wizard-sub-suffix">.{data.name}</span>
										</div>
									</div>
								</div>

								<div className="form-row">
									<div className="form-field">
										<span className="form-label">Handler</span>
										<Toggle
											options={handlerOptions}
											value={subHandlerType}
											onChange={(next: HandlerSelection) => {
												setSubHandlerType(next);
												if (next === "reverse_proxy") {
													setSubHandlerConfig({
														...defaultReverseProxyConfig,
													});
												} else if (next === "static_response") {
													setSubHandlerConfig({
														...defaultStaticResponseConfig,
													});
												} else if (next === "redirect") {
													setSubHandlerConfig({
														...defaultRedirectConfig,
													});
												} else if (next === "file_server") {
													setSubHandlerConfig({
														...defaultFileServerConfig,
													});
												} else {
													setSubHandlerConfig({});
												}
											}}
										/>
									</div>
								</div>

								{subHandlerType !== "none" && isSupportedHandler(subHandlerType) && (
									<HandlerConfig
										type={subHandlerType as HandlerType}
										config={subHandlerConfig}
										onChange={setSubHandlerConfig}
										disabled={false}
										domain={`${subPrefix || "sub"}.${data.name}`}
									/>
								)}

								<div className="rule-form-overrides">
									<button
										type="button"
										className="btn btn-ghost rule-form-overrides-toggle"
										onClick={() => setSubTogglesOpen(!subTogglesOpen)}
									>
										<span className={subTogglesOpen ? "chevron open" : "chevron"} />
										Toggles
									</button>
									{subTogglesOpen && (
										<div className="rule-form-overrides-body">
											<DomainToggleGrid
												toggles={subToggles}
												onUpdate={(key, value) =>
													setSubToggles((prev) => ({
														...prev,
														[key]: value,
													}))
												}
												idPrefix="wizard-sub-toggle"
												domain={`${subPrefix || "sub"}.${data.name}`}
												hideResponseHeaders={subHandlerType === "static_response"}
											/>
										</div>
									)}
								</div>

								<div className="wizard-rule-form-actions">
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											setSubFormActive(false);
											resetSubForm();
										}}
									>
										Cancel
									</button>
									<button type="button" className="btn btn-primary" onClick={addSubdomain}>
										Add Subdomain
									</button>
								</div>
							</div>
						) : !subFormActive ? (
							<div className="wizard-add-rule-prompt">
								{data.subdomains.length > 0 ? (
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											resetSubForm();
											setEditingSubIndex(null);
											setSubFormActive(true);
										}}
									>
										+ Add Another Subdomain
									</button>
								) : (
									<>
										<div className="wizard-add-rule-empty">
											No subdomains. Add subdomains, or skip to path rules.
										</div>
										<div className="wizard-add-rule-actions">
											<button
												type="button"
												className="btn btn-primary"
												onClick={() => {
													resetSubForm();
													setEditingSubIndex(null);
													setSubFormActive(true);
												}}
											>
												Add Subdomain
											</button>
										</div>
									</>
								)}
							</div>
						) : null}
					</div>
				)}

				{/* Step 3: Path Rules */}
				{step === 3 && (
					<div className="wizard-section">
						{allRules.length > 0 && (
							<div className="wizard-rule-list">
								{allRules.map((entry) => {
									const entryId = `${entry.target}-${entry.index}`;
									const isEditing = editingRuleId === entryId;
									return (
										<div key={`${entry.target}-${entry.rule.key}`} className="wizard-editable-card">
											<button
												type="button"
												className="wizard-editable-card-header"
												onClick={() =>
													isEditing ? cancelEditRule() : startEditRule(entry.target, entry.index)
												}
											>
												<div className="wizard-rule-summary-info">
													<span className="wizard-rule-match">
														{entry.targetLabel}
														{entry.rule.matchValue}
													</span>
													<span
														className={`rule-card-handler-badge handler-${entry.rule.handlerType}`}
													>
														{entry.rule.handlerType.replace("_", " ")}
													</span>
													{entry.rule.handlerType === "reverse_proxy" && (
														<span className="wizard-rule-upstream">
															{(entry.rule.handlerConfig as ReverseProxyConfig).upstream}
														</span>
													)}
												</div>
												<span className={isEditing ? "chevron open" : "chevron"} />
											</button>
											{isEditing && ruleFormActive && (
												<div className="wizard-editable-card-body">
													{ruleTargetOptions.length > 1 && (
														<div className="form-row">
															<div className="form-field">
																<label htmlFor="wizard-rule-target">Target</label>
																<select
																	id="wizard-rule-target"
																	value={ruleTarget}
																	onChange={(e) => setRuleTarget(e.target.value)}
																>
																	{ruleTargetOptions.map((opt) => (
																		<option key={opt.value} value={opt.value}>
																			{opt.label}
																		</option>
																	))}
																</select>
															</div>
														</div>
													)}

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

													<div className="form-row">
														<div className="form-field">
															<span className="form-label">Handler Type</span>
															<Toggle
																options={handlerTypeOptions}
																value={ruleHandlerType}
																onChange={(next: HandlerType) => {
																	setRuleHandlerType(next);
																	if (next === "reverse_proxy") {
																		setRuleHandlerConfig({
																			...defaultReverseProxyConfig,
																		});
																	} else if (next === "static_response") {
																		setRuleHandlerConfig({
																			...defaultStaticResponseConfig,
																		});
																	} else if (next === "redirect") {
																		setRuleHandlerConfig({
																			...defaultRedirectConfig,
																		});
																	} else if (next === "file_server") {
																		setRuleHandlerConfig({
																			...defaultFileServerConfig,
																		});
																	} else {
																		setRuleHandlerConfig({});
																	}
																}}
															/>
														</div>
													</div>

													{!isSupportedHandler(ruleHandlerType) && (
														<div className="alert-warning" role="status">
															This handler type is not yet supported.
														</div>
													)}

													{isSupportedHandler(ruleHandlerType) && (
														<HandlerConfig
															type={ruleHandlerType}
															config={ruleHandlerConfig}
															onChange={setRuleHandlerConfig}
															disabled={false}
															domain={ruleTarget ? `${ruleTarget}.${data.name}` : data.name}
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
																	domain={ruleTarget ? `${ruleTarget}.${data.name}` : data.name}
																	hideResponseHeaders={ruleHandlerType === "static_response"}
																/>
															</div>
														)}
													</div>

													<div className="wizard-rule-form-actions">
														<button
															type="button"
															className="btn btn-ghost btn-sm"
															onClick={() => {
																removeRule(entry.target, entry.index);
																cancelEditRule();
															}}
														>
															Remove
														</button>
														<div className="wizard-rule-form-actions-right">
															<button
																type="button"
																className="btn btn-ghost"
																onClick={cancelEditRule}
															>
																Cancel
															</button>
															<button
																type="button"
																className="btn btn-primary"
																onClick={saveEditRule}
																disabled={!isSupportedHandler(ruleHandlerType)}
															>
																Save
															</button>
														</div>
													</div>
												</div>
											)}
										</div>
									);
								})}
							</div>
						)}

						{ruleFormActive && editingRuleId === null ? (
							<div className="wizard-rule-form">
								{ruleTargetOptions.length > 1 && (
									<div className="form-row">
										<div className="form-field">
											<label htmlFor="wizard-rule-target">Add rule to</label>
											<select
												id="wizard-rule-target"
												value={ruleTarget}
												onChange={(e) => setRuleTarget(e.target.value)}
											>
												{ruleTargetOptions.map((opt) => (
													<option key={opt.value} value={opt.value}>
														{opt.label}
													</option>
												))}
											</select>
										</div>
									</div>
								)}

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

								<div className="form-row">
									<div className="form-field">
										<span className="form-label">Handler Type</span>
										<Toggle
											options={handlerTypeOptions}
											value={ruleHandlerType}
											onChange={(next: HandlerType) => {
												setRuleHandlerType(next);
												if (next === "reverse_proxy") {
													setRuleHandlerConfig({
														...defaultReverseProxyConfig,
													});
												} else if (next === "static_response") {
													setRuleHandlerConfig({
														...defaultStaticResponseConfig,
													});
												} else if (next === "redirect") {
													setRuleHandlerConfig({
														...defaultRedirectConfig,
													});
												} else if (next === "file_server") {
													setRuleHandlerConfig({
														...defaultFileServerConfig,
													});
												} else {
													setRuleHandlerConfig({});
												}
											}}
										/>
									</div>
								</div>

								{!isSupportedHandler(ruleHandlerType) && (
									<div className="alert-warning" role="status">
										This handler type is not yet supported.
									</div>
								)}

								{isSupportedHandler(ruleHandlerType) && (
									<HandlerConfig
										type={ruleHandlerType}
										config={ruleHandlerConfig}
										onChange={setRuleHandlerConfig}
										disabled={false}
										domain={ruleTarget ? `${ruleTarget}.${data.name}` : data.name}
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
												domain={ruleTarget ? `${ruleTarget}.${data.name}` : data.name}
												hideResponseHeaders={ruleHandlerType === "static_response"}
											/>
										</div>
									)}
								</div>

								<div className="wizard-rule-form-actions">
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											setRuleFormActive(false);
											resetRuleForm();
										}}
									>
										Cancel
									</button>
									<button
										type="button"
										className="btn btn-primary"
										onClick={addRule}
										disabled={!isSupportedHandler(ruleHandlerType)}
									>
										Add Rule
									</button>
								</div>
							</div>
						) : !ruleFormActive ? (
							<div className="wizard-add-rule-prompt">
								{allRules.length > 0 ? (
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											resetRuleForm();
											setEditingRuleId(null);
											setRuleFormActive(true);
										}}
									>
										+ Add Another Rule
									</button>
								) : (
									<>
										<div className="wizard-add-rule-empty">
											No path rules added yet. Add path rules, or skip to review.
										</div>
										<div className="wizard-add-rule-actions">
											<button
												type="button"
												className="btn btn-primary"
												onClick={() => {
													resetRuleForm();
													setEditingRuleId(null);
													setRuleFormActive(true);
												}}
											>
												Add Rule
											</button>
										</div>
									</>
								)}
							</div>
						) : null}
					</div>
				)}

				{/* Step 4: Review */}
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
					{step === 0 && (
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleNext}
							disabled={submitting || !urlInput.trim()}
						>
							Next
						</button>
					)}
					{step === 1 && (
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleNext}
							disabled={submitting || !rootRuleSupported}
						>
							Next
						</button>
					)}
					{step === 2 && !subFormActive && (
						<button
							type="button"
							className={data.subdomains.length > 0 ? "btn btn-primary" : "btn btn-ghost"}
							onClick={() => {
								resetRuleForm();
								setRuleFormActive(false);
								goToStep(3);
							}}
						>
							{data.subdomains.length > 0 ? "Continue to Path Rules" : "Skip to Path Rules"}
						</button>
					)}
					{step === 3 && !ruleFormActive && allRules.length === 0 && (
						<button type="button" className="btn btn-ghost" onClick={() => goToStep(4)}>
							Skip to Review
						</button>
					)}
					{step === 3 && !ruleFormActive && allRules.length > 0 && (
						<button type="button" className="btn btn-primary" onClick={() => goToStep(4)}>
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

function buildRuleReq(rule: WizardRule): CreateRuleRequest {
	return {
		match_type: rule.matchType,
		...(rule.matchType === "path" ? { path_match: rule.pathMatch } : {}),
		match_value: rule.matchValue,
		handler_type: rule.handlerType,
		handler_config: rule.handlerConfig,
		toggle_overrides: rule.toggleOverrides,
	};
}
