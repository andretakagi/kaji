import { useState } from "react";
import type { CreateDomainRequest, HandlerType, ReverseProxyConfig } from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";
import { Toggle } from "./Toggle";

interface Props {
	onCreate: (req: CreateDomainRequest) => Promise<void>;
	onCancel: () => void;
}

type HandlerSelection = "none" | HandlerType;

const handlerOptions: readonly { value: HandlerSelection; label: string }[] = [
	{ value: "none", label: "None" },
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
] as const;

export default function DomainForm({ onCreate, onCancel }: Props) {
	const [name, setName] = useState("");
	const [handlerType, setHandlerType] = useState<HandlerSelection>("none");
	const [upstream, setUpstream] = useState("");
	const [submitting, setSubmitting] = useState(false);
	const [formError, setFormError] = useState<string | null>(null);

	const supported = handlerType === "none" || handlerType === "reverse_proxy";

	async function handleSubmit(e: React.SubmitEvent) {
		e.preventDefault();
		setFormError(null);

		if (!name.trim()) {
			setFormError("Domain name is required");
			return;
		}

		if (handlerType === "reverse_proxy" && !upstream.trim()) {
			setFormError("Upstream is required");
			return;
		}

		let handlerConfig: ReverseProxyConfig | Record<string, unknown>;
		if (handlerType === "reverse_proxy") {
			handlerConfig = { ...defaultReverseProxyConfig, upstream: upstream.trim() };
		} else {
			handlerConfig = {};
		}

		const effectiveHandler: HandlerType = handlerType === "none" ? "reverse_proxy" : handlerType;

		const req: CreateDomainRequest = {
			name: name.trim(),
			toggles: defaultDomainToggles,
			first_rule: {
				match_type: "",
				handler_type: effectiveHandler,
				handler_config: handlerConfig,
			},
		};

		setSubmitting(true);
		try {
			await onCreate(req);
		} catch (err) {
			setFormError(getErrorMessage(err, "Failed to create domain"));
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<form className="add-route-form" onSubmit={handleSubmit}>
			<div className="form-row">
				<div className="form-field">
					<label htmlFor="domain-name">Domain</label>
					<input
						id="domain-name"
						type="text"
						placeholder="example.com"
						value={name}
						onChange={(e) => setName(e.target.value)}
						maxLength={253}
						required
						disabled={submitting}
					/>
				</div>
			</div>

			<div className="form-row">
				<div className="form-field">
					<label>Handler</label>
					<Toggle
						options={handlerOptions}
						value={handlerType}
						onChange={setHandlerType}
						disabled={submitting}
					/>
				</div>
			</div>

			{handlerType !== "none" && handlerType !== "reverse_proxy" && (
				<div className="alert-warning" role="status">
					This handler type is not yet supported. Only reverse proxy is available for now.
				</div>
			)}

			{handlerType === "reverse_proxy" && (
				<div className="form-row">
					<div className="form-field">
						<label htmlFor="domain-upstream">Upstream</label>
						<input
							id="domain-upstream"
							type="text"
							placeholder="localhost:3000"
							value={upstream}
							onChange={(e) => setUpstream(e.target.value)}
							maxLength={260}
							required
							disabled={submitting}
						/>
					</div>
				</div>
			)}

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
					{submitting ? "Creating..." : "Create Domain"}
				</button>
			</div>
		</form>
	);
}
