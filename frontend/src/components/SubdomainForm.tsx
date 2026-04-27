import { useId, useState } from "react";
import type { CreateSubdomainRequest, Rule } from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import RuleEditor from "./RuleEditor";

interface SubdomainFormProps {
	onSubmit: (req: CreateSubdomainRequest) => Promise<void>;
	onCancel: () => void;
}

export default function SubdomainForm({ onSubmit, onCancel }: SubdomainFormProps) {
	const formId = useId();

	const [name, setName] = useState("");
	const [rule, setRule] = useState<Rule>({
		handler_type: "reverse_proxy",
		handler_config: { ...defaultReverseProxyConfig },
		advanced_headers: false,
		enabled: true,
	});
	const [submitting, setSubmitting] = useState(false);
	const [formError, setFormError] = useState<string | null>(null);

	async function handleSubmit(e: React.SubmitEvent) {
		e.preventDefault();
		setFormError(null);

		const trimmed = name.trim();
		if (!trimmed) {
			setFormError("Subdomain name is required");
			return;
		}

		const req: CreateSubdomainRequest = {
			name: trimmed,
			rule: {
				handler_type: rule.handler_type,
				handler_config: rule.handler_config,
				advanced_headers: rule.advanced_headers,
			},
			toggles: { ...defaultDomainToggles },
		};

		setSubmitting(true);
		try {
			await onSubmit(req);
		} catch (err) {
			setFormError(getErrorMessage(err, "Failed to create subdomain"));
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<form className="add-domain-form add-domain-form-inline" onSubmit={handleSubmit}>
			<div className="form-row">
				<div className="form-field">
					<label htmlFor={`subdomain-name-${formId}`}>Name</label>
					<input
						id={`subdomain-name-${formId}`}
						type="text"
						placeholder="api"
						value={name}
						onChange={(e) => setName(e.target.value)}
						maxLength={253}
						required
						disabled={submitting}
					/>
				</div>
			</div>

			<RuleEditor allowNone value={rule} onChange={setRule} idPrefix={`subdomain-rule-${formId}`} />

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
					{submitting ? "Creating..." : "Add Subdomain"}
				</button>
			</div>
		</form>
	);
}
