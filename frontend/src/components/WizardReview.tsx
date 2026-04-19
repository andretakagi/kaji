import type { DomainToggles, ReverseProxyConfig, StaticResponseConfig } from "../types/domain";
import type { WizardData, WizardRule } from "./DomainWizard";

interface Props {
	data: WizardData;
	onEditStep: (step: number) => void;
}

function toggleSummary(toggles: DomainToggles): string[] {
	const active: string[] = [];
	if (toggles.force_https) active.push("Force HTTPS");
	if (toggles.compression) active.push("Compression");
	if (toggles.headers.response.enabled) active.push("Response Headers");
	if (toggles.basic_auth.enabled) active.push("Basic Auth");
	if (toggles.access_log) active.push("Access Log");
	if (toggles.ip_filtering.enabled) active.push("IP Filtering");
	return active;
}

function ruleMatchLabel(rule: WizardRule, domainName: string): string {
	if (rule.matchType === "subdomain") {
		return `${rule.matchValue}.${domainName}`;
	}
	return `${domainName}${rule.matchValue}`;
}

function staticResponseSummary(config: StaticResponseConfig): string {
	if (config.close) return "Close connection";
	const parts: string[] = [];
	if (config.status_code) parts.push(config.status_code);
	if (config.body) {
		const preview = config.body.length > 40 ? `${config.body.slice(0, 40)}...` : config.body;
		parts.push(preview);
	}
	return parts.join(" - ") || "Empty response";
}

export default function WizardReview({ data, onEditStep }: Props) {
	const activeTags = toggleSummary(data.toggles);

	const rootHandlerLabel =
		data.rootRule.handlerType !== "none" ? data.rootRule.handlerType.replace("_", " ") : null;

	const rootUpstream =
		data.rootRule.handlerType === "reverse_proxy"
			? (data.rootRule.handlerConfig as ReverseProxyConfig).upstream
			: null;

	const rootStaticSummary =
		data.rootRule.handlerType === "static_response"
			? staticResponseSummary(data.rootRule.handlerConfig as StaticResponseConfig)
			: null;

	return (
		<div className="wizard-review">
			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Domain</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(0)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">{data.name}</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Toggles</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(1)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{activeTags.length > 0 ? (
						<div className="wizard-review-tags">
							{activeTags.map((tag) => (
								<span key={tag} className="wizard-review-tag">
									{tag}
								</span>
							))}
						</div>
					) : (
						<span className="text-muted">None enabled</span>
					)}
				</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Root Rule</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(2)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{rootHandlerLabel ? (
						<div className="wizard-review-rule-detail">
							<span className={`rule-card-handler-badge handler-${data.rootRule.handlerType}`}>
								{rootHandlerLabel}
							</span>
							{rootUpstream && <span className="wizard-review-upstream">{rootUpstream}</span>}
							{rootStaticSummary && (
								<span className="wizard-review-upstream">{rootStaticSummary}</span>
							)}
						</div>
					) : (
						<span className="text-muted">None</span>
					)}
				</div>
			</div>

			<div className="wizard-review-section">
				<div className="wizard-review-header">
					<h4>Rules</h4>
					<button type="button" className="btn btn-ghost btn-sm" onClick={() => onEditStep(3)}>
						Edit
					</button>
				</div>
				<div className="wizard-review-value">
					{data.rules.length > 0 ? (
						<div className="wizard-review-rule-list">
							{data.rules.map((rule) => (
								<div key={rule.key} className="wizard-review-rule-item">
									<span className="wizard-review-rule-match">
										{ruleMatchLabel(rule, data.name)}
									</span>
									<span className={`rule-card-handler-badge handler-${rule.handlerType}`}>
										{rule.handlerType.replace("_", " ")}
									</span>
									{rule.handlerType === "reverse_proxy" && (
										<span className="wizard-review-upstream">
											{(rule.handlerConfig as ReverseProxyConfig).upstream}
										</span>
									)}
									{rule.handlerType === "static_response" && (
										<span className="wizard-review-upstream">
											{staticResponseSummary(rule.handlerConfig as StaticResponseConfig)}
										</span>
									)}
									{rule.toggleOverrides && <span className="wizard-review-tag">overrides</span>}
								</div>
							))}
						</div>
					) : (
						<span className="text-muted">No additional rules</span>
					)}
				</div>
			</div>
		</div>
	);
}
