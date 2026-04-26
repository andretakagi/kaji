import { useCallback, useRef, useState } from "react";
import type {
	CreateDomainFullRequest,
	CreatePathRequest,
	CreateSubdomainRequest,
	Domain,
	DomainToggles,
	FileServerConfig,
	HandlerConfigValue,
	HandlerType,
	PathMatch,
	RedirectConfig,
	ReverseProxyConfig,
	StaticResponseConfig,
	UpdateRuleRequest,
} from "../types/domain";
import {
	defaultDomainToggles,
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
} from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { pathMatchWarning, validateDomain, validateUpstream } from "../utils/validate";
import { DomainToggleGrid } from "./DomainToggleGrid";
import HandlerConfig from "./HandlerConfig";
import { Toggle } from "./Toggle";
import WizardReview from "./WizardReview";

type HandlerSelection = "none" | HandlerType;

export interface WizardPath {
	key: number;
	pathMatch: PathMatch;
	matchValue: string;
	handlerType: HandlerType;
	handlerConfig: HandlerConfigValue;
	toggleOverrides: DomainToggles | null;
}

export interface WizardSubdomain {
	key: number;
	prefix: string;
	rule: {
		handlerType: HandlerSelection;
		handlerConfig: HandlerConfigValue;
	};
	toggles: DomainToggles;
	paths: WizardPath[];
}

export interface WizardData {
	name: string;
	toggles: DomainToggles;
	rootRule: {
		handlerType: HandlerSelection;
		handlerConfig: HandlerConfigValue;
	};
	subdomains: WizardSubdomain[];
	rootPaths: WizardPath[];
}

const STEP_LABELS = ["URL", "Root Domain Rule", "Subdomain Rules", "Paths", "Review"];

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

export default function DomainWizard({ onCreate, onCancel, existingDomains }: Props) {
	const pathKeyRef = useRef(0);
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
		rootPaths: [],
	});

	const [pathFormActive, setPathFormActive] = useState(false);
	const [editingPathId, setEditingPathId] = useState<string | null>(null);
	const [pathTarget, setPathTarget] = useState("");
	const [pathMatch, setPathMatch] = useState<PathMatch>("prefix");
	const [pathMatchValue, setPathMatchValue] = useState("");
	const [pathHandlerType, setPathHandlerType] = useState<HandlerType>("reverse_proxy");
	const [pathHandlerConfig, setPathHandlerConfig] = useState<HandlerConfigValue>({
		...defaultReverseProxyConfig,
	});
	const [pathOverridesOpen, setPathOverridesOpen] = useState(false);
	const [pathToggleOverrides, setPathToggleOverrides] = useState<DomainToggles>({
		...defaultDomainToggles,
	});

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

	const resetPathForm = useCallback(() => {
		setPathTarget("");
		setPathMatch("prefix");
		setPathMatchValue("");
		setPathHandlerType("reverse_proxy");
		setPathHandlerConfig({ ...defaultReverseProxyConfig });
		setPathOverridesOpen(false);
		setPathToggleOverrides({ ...data.toggles });
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
									rule: {
										handlerType: "none",
										handlerConfig: {},
									},
									toggles: { ...defaultDomainToggles },
									paths: [],
								},
							],
				}));
				setPathTarget(prefix);
				setPathMatchValue(pathValue);
				setPathFormActive(true);
				goToStep(3);
			} else if (parsed.subPrefix) {
				setSubPrefix(parsed.subPrefix);
				setSubFormActive(true);
				goToStep(2);
			} else if (parsed.path) {
				setPathTarget("");
				setPathMatchValue(parsed.path);
				setPathFormActive(true);
				goToStep(3);
			} else {
				goToStep(1);
			}
			return;
		}

		if (step === 2) {
			resetPathForm();
			setPathFormActive(false);
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

	const addPath = () => {
		setError("");
		if (!pathMatchValue.trim()) {
			setError("Path value is required");
			return;
		}
		if (!isSupportedHandler(pathHandlerType)) {
			setError("This handler type is not yet supported");
			return;
		}
		const handlerErr = validateHandlerConfig(pathHandlerType, pathHandlerConfig);
		if (handlerErr) {
			setError(handlerErr);
			return;
		}
		if (pathOverridesOpen) {
			const toggleErr = validateToggles(pathToggleOverrides);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}

		pathKeyRef.current += 1;
		const newPath: WizardPath = {
			key: pathKeyRef.current,
			pathMatch,
			matchValue: pathMatchValue.trim(),
			handlerType: pathHandlerType,
			handlerConfig: pathHandlerConfig,
			toggleOverrides: pathOverridesOpen ? pathToggleOverrides : null,
		};

		if (pathTarget === "") {
			setData((prev) => ({ ...prev, rootPaths: [...prev.rootPaths, newPath] }));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === pathTarget.toLowerCase()
						? { ...s, paths: [...s.paths, newPath] }
						: s,
				),
			}));
		}
		setPathFormActive(false);
		resetPathForm();
	};

	const removePath = (target: string, index: number) => {
		if (target === "") {
			setData((prev) => ({ ...prev, rootPaths: prev.rootPaths.filter((_, i) => i !== index) }));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === target.toLowerCase()
						? { ...s, paths: s.paths.filter((_, i) => i !== index) }
						: s,
				),
			}));
		}
	};

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
			rule: {
				handlerType: subHandlerType,
				handlerConfig: subHandlerType === "none" ? {} : subHandlerConfig,
			},
			toggles: subTogglesOpen ? subToggles : { ...data.toggles },
			paths: [],
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
		setSubHandlerType(sub.rule.handlerType);
		setSubHandlerConfig(sub.rule.handlerConfig);
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
							rule: {
								handlerType: subHandlerType,
								handlerConfig: subHandlerType === "none" ? {} : subHandlerConfig,
							},
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

	const startEditPath = (target: string, index: number) => {
		const paths =
			target === ""
				? data.rootPaths
				: data.subdomains.find((s) => s.prefix.toLowerCase() === target.toLowerCase())?.paths;
		if (!paths) return;
		const path = paths[index];
		setPathTarget(target);
		setPathMatch(path.pathMatch);
		setPathMatchValue(path.matchValue);
		setPathHandlerType(path.handlerType);
		setPathHandlerConfig(path.handlerConfig);
		setPathOverridesOpen(path.toggleOverrides !== null);
		setPathToggleOverrides(path.toggleOverrides ?? { ...data.toggles });
		setEditingPathId(`${target}-${index}`);
		setPathFormActive(true);
	};

	const saveEditPath = () => {
		if (editingPathId === null) return;
		setError("");
		if (!pathMatchValue.trim()) {
			setError("Path value is required");
			return;
		}
		const handlerErr = validateHandlerConfig(pathHandlerType, pathHandlerConfig);
		if (handlerErr) {
			setError(handlerErr);
			return;
		}
		if (pathOverridesOpen) {
			const toggleErr = validateToggles(pathToggleOverrides);
			if (toggleErr) {
				setError(toggleErr);
				return;
			}
		}
		const parts = editingPathId.split("-");
		const editIndex = Number.parseInt(parts[parts.length - 1], 10);
		const editTarget = parts.slice(0, -1).join("-");
		const updated: WizardPath = {
			key: 0,
			pathMatch,
			matchValue: pathMatchValue.trim(),
			handlerType: pathHandlerType,
			handlerConfig: pathHandlerConfig,
			toggleOverrides: pathOverridesOpen ? pathToggleOverrides : null,
		};
		if (editTarget === "") {
			setData((prev) => ({
				...prev,
				rootPaths: prev.rootPaths.map((p, i) => (i === editIndex ? { ...updated, key: p.key } : p)),
			}));
		} else {
			setData((prev) => ({
				...prev,
				subdomains: prev.subdomains.map((s) =>
					s.prefix.toLowerCase() === editTarget.toLowerCase()
						? {
								...s,
								paths: s.paths.map((p, i) => (i === editIndex ? { ...updated, key: p.key } : p)),
							}
						: s,
				),
			}));
		}
		setEditingPathId(null);
		setPathFormActive(false);
		resetPathForm();
	};

	const cancelEditPath = () => {
		setEditingPathId(null);
		setPathFormActive(false);
		resetPathForm();
	};

	const handleSubmit = async () => {
		setSubmitting(true);
		setError("");
		try {
			const req = buildCreateRequest(data);
			await onCreate(req);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to create domain"));
		} finally {
			setSubmitting(false);
		}
	};

	const allPaths: { target: string; targetLabel: string; path: WizardPath; index: number }[] = [];
	for (let i = 0; i < data.rootPaths.length; i++) {
		allPaths.push({ target: "", targetLabel: data.name, path: data.rootPaths[i], index: i });
	}
	for (const sub of data.subdomains) {
		for (let i = 0; i < sub.paths.length; i++) {
			allPaths.push({
				target: sub.prefix,
				targetLabel: `${sub.prefix}.${data.name}`,
				path: sub.paths[i],
				index: i,
			});
		}
	}

	const pathTargetOptions = [
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
								const pathTargetLabel = parsed.subPrefix
									? `${parsed.subPrefix}.${parsed.domain}`
									: parsed.domain;
								parts.push(`path rule "${parsed.path}" on ${pathTargetLabel}`);
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
												{sub.rule.handlerType !== "none" ? (
													<span
														className={`rule-card-handler-badge handler-${sub.rule.handlerType}`}
													>
														{sub.rule.handlerType.replace("_", " ")}
													</span>
												) : (
													<span className="rule-card-handler-badge handler-none">no handler</span>
												)}
												{sub.paths.length > 0 && (
													<span className="wizard-rule-upstream">
														{sub.paths.length} path
														{sub.paths.length !== 1 ? "s" : ""}
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
											No subdomains. Add subdomains, or skip to paths.
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

				{step === 3 && (
					<div className="wizard-section">
						{allPaths.length > 0 && (
							<div className="wizard-rule-list">
								{allPaths.map((entry) => {
									const entryId = `${entry.target}-${entry.index}`;
									const isEditing = editingPathId === entryId;
									return (
										<div key={`${entry.target}-${entry.path.key}`} className="wizard-editable-card">
											<button
												type="button"
												className="wizard-editable-card-header"
												onClick={() =>
													isEditing ? cancelEditPath() : startEditPath(entry.target, entry.index)
												}
											>
												<div className="wizard-rule-summary-info">
													<span className="wizard-rule-match">
														{entry.targetLabel}
														{entry.path.matchValue}
													</span>
													<span
														className={`rule-card-handler-badge handler-${entry.path.handlerType}`}
													>
														{entry.path.handlerType.replace("_", " ")}
													</span>
													{entry.path.handlerType === "reverse_proxy" && (
														<span className="wizard-rule-upstream">
															{(entry.path.handlerConfig as ReverseProxyConfig).upstream}
														</span>
													)}
												</div>
												<span className={isEditing ? "chevron open" : "chevron"} />
											</button>
											{isEditing && pathFormActive && (
												<div className="wizard-editable-card-body">
													{pathTargetOptions.length > 1 && (
														<div className="form-row">
															<div className="form-field">
																<label htmlFor="wizard-rule-target">Target</label>
																<select
																	id="wizard-rule-target"
																	value={pathTarget}
																	onChange={(e) => setPathTarget(e.target.value)}
																>
																	{pathTargetOptions.map((opt) => (
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
																value={pathMatch}
																onChange={setPathMatch}
															/>
														</div>
														<div className="form-field">
															<label htmlFor="wizard-rule-path-value">Path</label>
															<input
																id="wizard-rule-path-value"
																type="text"
																placeholder={
																	pathMatch === "prefix"
																		? "/api/"
																		: pathMatch === "exact"
																			? "/api/v1/health"
																			: "^/api/.*"
																}
																value={pathMatchValue}
																onChange={(e) => setPathMatchValue(e.target.value)}
																maxLength={253}
																required
															/>
															{pathMatchWarning(pathMatch, pathMatchValue) && (
																<span className="field-warning">
																	{pathMatchWarning(pathMatch, pathMatchValue)}
																</span>
															)}
														</div>
													</div>

													<div className="form-row">
														<div className="form-field">
															<span className="form-label">Handler Type</span>
															<Toggle
																options={handlerTypeOptions}
																value={pathHandlerType}
																onChange={(next: HandlerType) => {
																	setPathHandlerType(next);
																	if (next === "reverse_proxy") {
																		setPathHandlerConfig({
																			...defaultReverseProxyConfig,
																		});
																	} else if (next === "static_response") {
																		setPathHandlerConfig({
																			...defaultStaticResponseConfig,
																		});
																	} else if (next === "redirect") {
																		setPathHandlerConfig({
																			...defaultRedirectConfig,
																		});
																	} else if (next === "file_server") {
																		setPathHandlerConfig({
																			...defaultFileServerConfig,
																		});
																	} else {
																		setPathHandlerConfig({});
																	}
																}}
															/>
														</div>
													</div>

													{!isSupportedHandler(pathHandlerType) && (
														<div className="alert-warning" role="status">
															This handler type is not yet supported.
														</div>
													)}

													{isSupportedHandler(pathHandlerType) && (
														<HandlerConfig
															type={pathHandlerType}
															config={pathHandlerConfig}
															onChange={setPathHandlerConfig}
															disabled={false}
															domain={pathTarget ? `${pathTarget}.${data.name}` : data.name}
														/>
													)}

													<div className="rule-form-overrides">
														<button
															type="button"
															className="btn btn-ghost rule-form-overrides-toggle"
															onClick={() => setPathOverridesOpen(!pathOverridesOpen)}
														>
															<span className={pathOverridesOpen ? "chevron open" : "chevron"} />
															Override domain toggles
														</button>
														{pathOverridesOpen && (
															<div className="rule-form-overrides-body">
																<DomainToggleGrid
																	toggles={pathToggleOverrides}
																	onUpdate={(key, value) =>
																		setPathToggleOverrides((prev) => ({
																			...prev,
																			[key]: value,
																		}))
																	}
																	idPrefix="wizard-rule-override"
																	domain={pathTarget ? `${pathTarget}.${data.name}` : data.name}
																	hideResponseHeaders={pathHandlerType === "static_response"}
																/>
															</div>
														)}
													</div>

													<div className="wizard-rule-form-actions">
														<button
															type="button"
															className="btn btn-ghost btn-sm"
															onClick={() => {
																removePath(entry.target, entry.index);
																cancelEditPath();
															}}
														>
															Remove
														</button>
														<div className="wizard-rule-form-actions-right">
															<button
																type="button"
																className="btn btn-ghost"
																onClick={cancelEditPath}
															>
																Cancel
															</button>
															<button
																type="button"
																className="btn btn-primary"
																onClick={saveEditPath}
																disabled={!isSupportedHandler(pathHandlerType)}
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

						{pathFormActive && editingPathId === null ? (
							<div className="wizard-rule-form">
								{pathTargetOptions.length > 1 && (
									<div className="form-row">
										<div className="form-field">
											<label htmlFor="wizard-rule-target">Add path to</label>
											<select
												id="wizard-rule-target"
												value={pathTarget}
												onChange={(e) => setPathTarget(e.target.value)}
											>
												{pathTargetOptions.map((opt) => (
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
										<Toggle options={pathMatchOptions} value={pathMatch} onChange={setPathMatch} />
									</div>
									<div className="form-field">
										<label htmlFor="wizard-rule-path-value">Path</label>
										<input
											id="wizard-rule-path-value"
											type="text"
											placeholder={
												pathMatch === "prefix"
													? "/api/"
													: pathMatch === "exact"
														? "/api/v1/health"
														: "^/api/.*"
											}
											value={pathMatchValue}
											onChange={(e) => setPathMatchValue(e.target.value)}
											maxLength={253}
											required
										/>
										{pathMatchWarning(pathMatch, pathMatchValue) && (
											<span className="field-warning">
												{pathMatchWarning(pathMatch, pathMatchValue)}
											</span>
										)}
									</div>
								</div>

								<div className="form-row">
									<div className="form-field">
										<span className="form-label">Handler Type</span>
										<Toggle
											options={handlerTypeOptions}
											value={pathHandlerType}
											onChange={(next: HandlerType) => {
												setPathHandlerType(next);
												if (next === "reverse_proxy") {
													setPathHandlerConfig({
														...defaultReverseProxyConfig,
													});
												} else if (next === "static_response") {
													setPathHandlerConfig({
														...defaultStaticResponseConfig,
													});
												} else if (next === "redirect") {
													setPathHandlerConfig({
														...defaultRedirectConfig,
													});
												} else if (next === "file_server") {
													setPathHandlerConfig({
														...defaultFileServerConfig,
													});
												} else {
													setPathHandlerConfig({});
												}
											}}
										/>
									</div>
								</div>

								{!isSupportedHandler(pathHandlerType) && (
									<div className="alert-warning" role="status">
										This handler type is not yet supported.
									</div>
								)}

								{isSupportedHandler(pathHandlerType) && (
									<HandlerConfig
										type={pathHandlerType}
										config={pathHandlerConfig}
										onChange={setPathHandlerConfig}
										disabled={false}
										domain={pathTarget ? `${pathTarget}.${data.name}` : data.name}
									/>
								)}

								<div className="rule-form-overrides">
									<button
										type="button"
										className="btn btn-ghost rule-form-overrides-toggle"
										onClick={() => setPathOverridesOpen(!pathOverridesOpen)}
									>
										<span className={pathOverridesOpen ? "chevron open" : "chevron"} />
										Override domain toggles
									</button>
									{pathOverridesOpen && (
										<div className="rule-form-overrides-body">
											<DomainToggleGrid
												toggles={pathToggleOverrides}
												onUpdate={(key, value) =>
													setPathToggleOverrides((prev) => ({
														...prev,
														[key]: value,
													}))
												}
												idPrefix="wizard-rule-override"
												domain={pathTarget ? `${pathTarget}.${data.name}` : data.name}
												hideResponseHeaders={pathHandlerType === "static_response"}
											/>
										</div>
									)}
								</div>

								<div className="wizard-rule-form-actions">
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											setPathFormActive(false);
											resetPathForm();
										}}
									>
										Cancel
									</button>
									<button
										type="button"
										className="btn btn-primary"
										onClick={addPath}
										disabled={!isSupportedHandler(pathHandlerType)}
									>
										Add Path
									</button>
								</div>
							</div>
						) : !pathFormActive ? (
							<div className="wizard-add-rule-prompt">
								{allPaths.length > 0 ? (
									<button
										type="button"
										className="btn btn-ghost"
										onClick={() => {
											resetPathForm();
											setEditingPathId(null);
											setPathFormActive(true);
										}}
									>
										+ Add Another Path
									</button>
								) : (
									<>
										<div className="wizard-add-rule-empty">
											No paths added yet. Add paths, or skip to review.
										</div>
										<div className="wizard-add-rule-actions">
											<button
												type="button"
												className="btn btn-primary"
												onClick={() => {
													resetPathForm();
													setEditingPathId(null);
													setPathFormActive(true);
												}}
											>
												Add Path
											</button>
										</div>
									</>
								)}
							</div>
						) : null}
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
								resetPathForm();
								setPathFormActive(false);
								goToStep(3);
							}}
						>
							{data.subdomains.length > 0 ? "Continue to Paths" : "Skip to Paths"}
						</button>
					)}
					{step === 3 && !pathFormActive && allPaths.length === 0 && (
						<button type="button" className="btn btn-ghost" onClick={() => goToStep(4)}>
							Skip to Review
						</button>
					)}
					{step === 3 && !pathFormActive && allPaths.length > 0 && (
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

function buildRule(
	handlerType: HandlerSelection,
	handlerConfig: HandlerConfigValue,
): UpdateRuleRequest {
	return handlerType === "none"
		? { handler_type: "none", handler_config: {}, advanced_headers: false }
		: { handler_type: handlerType, handler_config: handlerConfig, advanced_headers: false };
}

function buildPath(p: WizardPath): CreatePathRequest {
	return {
		path_match: p.pathMatch,
		match_value: p.matchValue,
		rule: { handler_type: p.handlerType, handler_config: p.handlerConfig, advanced_headers: false },
		toggle_overrides: p.toggleOverrides,
	};
}

function buildCreateRequest(data: WizardData): CreateDomainFullRequest {
	return {
		name: data.name.trim(),
		toggles: data.toggles,
		rule: buildRule(data.rootRule.handlerType, data.rootRule.handlerConfig),
		subdomains: data.subdomains.map<CreateSubdomainRequest>((s) => ({
			name: s.prefix,
			rule: buildRule(s.rule.handlerType, s.rule.handlerConfig),
			toggles: s.toggles,
		})),
		paths: data.rootPaths.map(buildPath),
	};
}
