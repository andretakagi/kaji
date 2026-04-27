import { useId, useState } from "react";
import type {
	CreatePathRequest,
	DomainToggles,
	Path,
	PathMatch,
	Rule,
	UpdatePathRequest,
} from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { pathMatchWarning, validateRule } from "../utils/validate";
import { DomainToggleGrid } from "./DomainToggleGrid";
import RuleEditor from "./RuleEditor";

interface PathFormProps {
	domainName: string;
	parentToggles: DomainToggles;
	initial?: Path;
	inline?: boolean;
	onSubmit: (req: CreatePathRequest | UpdatePathRequest) => Promise<void>;
	onCancel: () => void;
}

const pathMatchOptions: { value: PathMatch; label: string }[] = [
	{ value: "prefix", label: "Prefix" },
	{ value: "exact", label: "Exact" },
	{ value: "regex", label: "Regex" },
];

export default function PathForm({
	domainName,
	parentToggles,
	initial,
	inline,
	onSubmit,
	onCancel,
}: PathFormProps) {
	const formId = useId();
	const isEdit = !!initial;

	const [pathMatch, setPathMatch] = useState<PathMatch>(initial?.path_match ?? "prefix");
	const [matchValue, setMatchValue] = useState(initial?.match_value ?? "");
	const [rule, setRule] = useState<Rule>(
		initial?.rule ?? {
			handler_type: "reverse_proxy",
			handler_config: { ...defaultReverseProxyConfig },
			advanced_headers: false,
		},
	);
	const [overridesOpen, setOverridesOpen] = useState(initial?.toggle_overrides != null);
	const [toggleOverrides, setToggleOverrides] = useState<DomainToggles>(
		initial?.toggle_overrides ?? parentToggles ?? { ...defaultDomainToggles },
	);
	const [submitting, setSubmitting] = useState(false);
	const [formError, setFormError] = useState<string | null>(null);

	async function handleSubmit(e: React.SubmitEvent) {
		e.preventDefault();
		setFormError(null);

		if (!matchValue.trim()) {
			setFormError("Path value is required");
			return;
		}

		const ruleError = validateRule(rule);
		if (ruleError) {
			setFormError(ruleError);
			return;
		}

		const req: CreatePathRequest = {
			path_match: pathMatch,
			match_value: matchValue.trim(),
			rule: {
				handler_type: rule.handler_type,
				handler_config: rule.handler_config,
				advanced_headers: rule.advanced_headers,
			},
			toggle_overrides: overridesOpen ? toggleOverrides : null,
		};

		setSubmitting(true);
		try {
			await onSubmit(req);
		} catch (err) {
			setFormError(
				getErrorMessage(err, isEdit ? "Failed to update path" : "Failed to create path"),
			);
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<form
			className={inline ? "add-domain-form add-domain-form-inline" : "add-domain-form"}
			onSubmit={handleSubmit}
		>
			<div className="form-row">
				<div className="form-field">
					<label htmlFor={`path-match-${formId}`}>Path Match</label>
					<select
						id={`path-match-${formId}`}
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
					<label htmlFor={`path-value-${formId}`}>Path</label>
					<input
						id={`path-value-${formId}`}
						type="text"
						placeholder="/api/*"
						value={matchValue}
						onChange={(e) => setMatchValue(e.target.value)}
						maxLength={253}
						required
						disabled={submitting}
					/>
					{pathMatchWarning(pathMatch, matchValue) && (
						<span className="field-warning">{pathMatchWarning(pathMatch, matchValue)}</span>
					)}
				</div>
			</div>

			<RuleEditor
				allowNone={false}
				value={rule}
				onChange={setRule}
				idPrefix={`path-rule-${formId}`}
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
							onUpdate={(key: keyof DomainToggles, value: DomainToggles[keyof DomainToggles]) =>
								setToggleOverrides((prev) => ({ ...prev, [key]: value }))
							}
							idPrefix={`path-override-${formId}`}
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
				<button type="submit" className="btn btn-primary submit-btn" disabled={submitting}>
					{submitting ? (isEdit ? "Saving..." : "Creating...") : isEdit ? "Save Path" : "Add Path"}
				</button>
			</div>
		</form>
	);
}
