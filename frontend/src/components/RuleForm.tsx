import { useState } from "react";
import type {
	CreateRuleRequest,
	DomainToggles,
	HandlerType,
	MatchType,
	PathMatch,
	ReverseProxyConfig,
	Rule,
	UpdateRuleRequest,
} from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { DomainToggleGrid } from "./DomainToggleGrid";
import HandlerConfig from "./HandlerConfig";

interface Props {
	domainId: string;
	domainName?: string;
	initial?: Rule;
	onSubmit: (req: CreateRuleRequest | UpdateRuleRequest) => Promise<void>;
	onCancel: () => void;
}

const handlerOptions: { value: HandlerType; label: string }[] = [
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
];

const matchOptions: { value: MatchType; label: string }[] = [
	{ value: "", label: "Root (entire domain)" },
	{ value: "subdomain", label: "Subdomain" },
	{ value: "path", label: "Path" },
];

const pathMatchOptions: { value: PathMatch; label: string }[] = [
	{ value: "prefix", label: "Prefix" },
	{ value: "exact", label: "Exact" },
	{ value: "regex", label: "Regex" },
];

export default function RuleForm({ domainId, domainName, initial, onSubmit, onCancel }: Props) {
	const isEdit = !!initial;

	const [matchType, setMatchType] = useState<MatchType>(initial?.match_type ?? "");
	const [pathMatch, setPathMatch] = useState<PathMatch>(
		initial?.path_match === "" ? "prefix" : (initial?.path_match ?? "prefix"),
	);
	const [matchValue, setMatchValue] = useState(initial?.match_value ?? "");
	const [handlerType, setHandlerType] = useState<HandlerType>(
		initial?.handler_type ?? "reverse_proxy",
	);
	const [handlerConfig, setHandlerConfig] = useState<ReverseProxyConfig | Record<string, unknown>>(
		initial?.handler_config ?? { ...defaultReverseProxyConfig },
	);
	const [overridesOpen, setOverridesOpen] = useState(initial?.toggle_overrides != null);
	const [toggleOverrides, setToggleOverrides] = useState<DomainToggles>(
		initial?.toggle_overrides ?? { ...defaultDomainToggles },
	);
	const [submitting, setSubmitting] = useState(false);
	const [formError, setFormError] = useState<string | null>(null);

	const supported = handlerType === "reverse_proxy";

	async function handleSubmit(e: React.SubmitEvent) {
		e.preventDefault();
		setFormError(null);

		if (matchType === "subdomain" && !matchValue.trim()) {
			setFormError("Subdomain value is required");
			return;
		}

		if (matchType === "path" && !matchValue.trim()) {
			setFormError("Path value is required");
			return;
		}

		if (supported) {
			const rp = handlerConfig as ReverseProxyConfig;
			if (!rp.upstream.trim()) {
				setFormError("Upstream is required");
				return;
			}
		}

		const req: CreateRuleRequest = {
			match_type: matchType,
			...(matchType === "path" ? { path_match: pathMatch } : {}),
			...(matchType !== "" ? { match_value: matchValue.trim() } : {}),
			handler_type: handlerType,
			handler_config: handlerConfig,
			toggle_overrides: overridesOpen ? toggleOverrides : null,
		};

		setSubmitting(true);
		try {
			await onSubmit(req);
		} catch (err) {
			setFormError(
				getErrorMessage(err, isEdit ? "Failed to update rule" : "Failed to create rule"),
			);
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<form className="add-route-form" onSubmit={handleSubmit}>
			<div className="form-row">
				<div className="form-field">
					<label htmlFor={`rule-match-type-${domainId}`}>Match Type</label>
					<select
						id={`rule-match-type-${domainId}`}
						value={matchType}
						onChange={(e) => setMatchType(e.target.value as MatchType)}
						disabled={submitting}
					>
						{matchOptions.map((o) => (
							<option key={o.value} value={o.value}>
								{o.label}
							</option>
						))}
					</select>
				</div>

				{matchType === "subdomain" && (
					<div className="form-field">
						<label htmlFor={`rule-match-value-${domainId}`}>Subdomain</label>
						<input
							id={`rule-match-value-${domainId}`}
							type="text"
							placeholder="api"
							value={matchValue}
							onChange={(e) => setMatchValue(e.target.value)}
							maxLength={63}
							required
							disabled={submitting}
						/>
					</div>
				)}

				{matchType === "path" && (
					<>
						<div className="form-field">
							<label htmlFor={`rule-path-match-${domainId}`}>Path Match</label>
							<select
								id={`rule-path-match-${domainId}`}
								value={pathMatch}
								onChange={(e) => setPathMatch(e.target.value as PathMatch)}
								disabled={submitting}
							>
								{pathMatchOptions.map((o) => (
									<option key={o.value} value={o.value}>
										{o.label}
									</option>
								))}
							</select>
						</div>
						<div className="form-field">
							<label htmlFor={`rule-path-value-${domainId}`}>Path</label>
							<input
								id={`rule-path-value-${domainId}`}
								type="text"
								placeholder="/api/*"
								value={matchValue}
								onChange={(e) => setMatchValue(e.target.value)}
								maxLength={253}
								required
								disabled={submitting}
							/>
						</div>
					</>
				)}
			</div>

			<div className="form-row">
				<div className="form-field">
					<label htmlFor={`rule-handler-type-${domainId}`}>Handler Type</label>
					<select
						id={`rule-handler-type-${domainId}`}
						value={handlerType}
						onChange={(e) => {
							const next = e.target.value as HandlerType;
							setHandlerType(next);
							if (next === "reverse_proxy") {
								setHandlerConfig({ ...defaultReverseProxyConfig });
							} else {
								setHandlerConfig({});
							}
						}}
						disabled={submitting}
					>
						{handlerOptions.map((o) => (
							<option key={o.value} value={o.value}>
								{o.label}
							</option>
						))}
					</select>
				</div>
			</div>

			<HandlerConfig
				type={handlerType}
				config={handlerConfig}
				onChange={setHandlerConfig}
				disabled={submitting}
			/>

			<div className="rule-form-overrides">
				<button
					type="button"
					className="btn btn-ghost rule-form-overrides-toggle"
					onClick={() => setOverridesOpen(!overridesOpen)}
				>
					<span className={overridesOpen ? "chevron open" : "chevron"} />
					Override domain toggles
				</button>
				{overridesOpen && (
					<div className="rule-form-overrides-body">
						<DomainToggleGrid
							toggles={toggleOverrides}
							onUpdate={(key, value) => setToggleOverrides((prev) => ({ ...prev, [key]: value }))}
							idPrefix={`rule-override-${domainId}`}
							domain={domainName}
						/>
					</div>
				)}
			</div>

			{formError && (
				<div className="inline-error" role="alert">
					{formError}
				</div>
			)}

			<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
				<button type="button" className="btn btn-ghost" onClick={onCancel} disabled={submitting}>
					Cancel
				</button>
				<button
					type="submit"
					className="btn btn-primary submit-btn"
					disabled={submitting || !supported}
				>
					{submitting ? (isEdit ? "Saving..." : "Creating...") : isEdit ? "Save Rule" : "Add Rule"}
				</button>
			</div>
		</form>
	);
}
