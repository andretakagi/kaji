import { useState } from "react";
import type {
	CreateDomainRequest,
	HandlerType,
	MatchType,
	PathMatch,
	ReverseProxyConfig,
} from "../types/domain";
import { defaultDomainToggles, defaultReverseProxyConfig } from "../types/domain";
import { getErrorMessage } from "../utils/getErrorMessage";

interface Props {
	onCreate: (req: CreateDomainRequest) => Promise<void>;
	onCancel: () => void;
	saving: boolean;
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

export default function DomainForm({ onCreate, onCancel, saving }: Props) {
	const [name, setName] = useState("");
	const [matchType, setMatchType] = useState<MatchType>("");
	const [pathMatch, setPathMatch] = useState<PathMatch>("prefix");
	const [matchValue, setMatchValue] = useState("");
	const [handlerType, setHandlerType] = useState<HandlerType>("reverse_proxy");
	const [upstream, setUpstream] = useState("");
	const [formError, setFormError] = useState<string | null>(null);

	const supported = handlerType === "reverse_proxy";

	async function handleSubmit(e: React.FormEvent) {
		e.preventDefault();
		setFormError(null);

		if (!name.trim()) {
			setFormError("Domain name is required");
			return;
		}

		if (matchType === "subdomain" && !matchValue.trim()) {
			setFormError("Subdomain value is required");
			return;
		}

		if (matchType === "path" && !matchValue.trim()) {
			setFormError("Path value is required");
			return;
		}

		if (supported && !upstream.trim()) {
			setFormError("Upstream is required");
			return;
		}

		let handlerConfig: ReverseProxyConfig | Record<string, unknown>;
		if (handlerType === "reverse_proxy") {
			handlerConfig = { ...defaultReverseProxyConfig, upstream: upstream.trim() };
		} else {
			handlerConfig = {};
		}

		const req: CreateDomainRequest = {
			name: name.trim(),
			toggles: defaultDomainToggles,
			first_rule: {
				match_type: matchType,
				...(matchType === "path" ? { path_match: pathMatch } : {}),
				...(matchType !== "" ? { match_value: matchValue.trim() } : {}),
				handler_type: handlerType,
				handler_config: handlerConfig,
			},
		};

		try {
			await onCreate(req);
		} catch (err) {
			setFormError(getErrorMessage(err, "Failed to create domain"));
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
						disabled={saving}
					/>
				</div>
			</div>

			<div className="form-row">
				<div className="form-field">
					<label htmlFor="domain-match-type">Match Type</label>
					<select
						id="domain-match-type"
						value={matchType}
						onChange={(e) => setMatchType(e.target.value as MatchType)}
						disabled={saving}
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
						<label htmlFor="domain-match-value">Subdomain</label>
						<input
							id="domain-match-value"
							type="text"
							placeholder="api"
							value={matchValue}
							onChange={(e) => setMatchValue(e.target.value)}
							maxLength={63}
							required
							disabled={saving}
						/>
					</div>
				)}

				{matchType === "path" && (
					<>
						<div className="form-field">
							<label htmlFor="domain-path-match">Path Match</label>
							<select
								id="domain-path-match"
								value={pathMatch}
								onChange={(e) => setPathMatch(e.target.value as PathMatch)}
								disabled={saving}
							>
								{pathMatchOptions.map((o) => (
									<option key={o.value} value={o.value}>
										{o.label}
									</option>
								))}
							</select>
						</div>
						<div className="form-field">
							<label htmlFor="domain-path-value">Path</label>
							<input
								id="domain-path-value"
								type="text"
								placeholder="/api/*"
								value={matchValue}
								onChange={(e) => setMatchValue(e.target.value)}
								maxLength={253}
								required
								disabled={saving}
							/>
						</div>
					</>
				)}
			</div>

			<div className="form-row">
				<div className="form-field">
					<label htmlFor="domain-handler-type">Handler Type</label>
					<select
						id="domain-handler-type"
						value={handlerType}
						onChange={(e) => setHandlerType(e.target.value as HandlerType)}
						disabled={saving}
					>
						{handlerOptions.map((o) => (
							<option key={o.value} value={o.value}>
								{o.label}
							</option>
						))}
					</select>
				</div>

				{supported && (
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
							disabled={saving}
						/>
					</div>
				)}
			</div>

			{!supported && (
				<div className="alert-warning" role="status">
					This handler type is not yet supported. Only reverse proxy is available for now.
				</div>
			)}

			{formError && (
				<div className="inline-error" role="alert">
					{formError}
				</div>
			)}

			<div className="form-row" style={{ justifyContent: "flex-end", gap: "0.5rem" }}>
				<button type="button" className="btn btn-ghost" onClick={onCancel} disabled={saving}>
					Cancel
				</button>
				<button
					type="submit"
					className="btn btn-primary submit-btn"
					disabled={saving || !supported}
				>
					{saving ? "Creating..." : "Create Domain"}
				</button>
			</div>
		</form>
	);
}
