import type { GlobalToggles } from "../types/api";

interface GlobalTogglesFormProps {
	toggles: GlobalToggles;
	onChange: <K extends keyof GlobalToggles>(key: K, value: GlobalToggles[K]) => void;
	idPrefix?: string;
	selectClassName?: string;
}

export function GlobalTogglesForm({
	toggles,
	onChange,
	idPrefix = "",
	selectClassName = "auth-field",
}: GlobalTogglesFormProps) {
	const selectId = `${idPrefix}auto-https`;

	return (
		<>
			<div className={selectClassName}>
				<label htmlFor={selectId}>Auto HTTPS</label>
				<select
					id={selectId}
					value={toggles.auto_https}
					onChange={(e) => onChange("auto_https", e.target.value as GlobalToggles["auto_https"])}
				>
					<option value="on">On</option>
					<option value="off">Off</option>
					<option value="disable_redirects">On (no redirects)</option>
				</select>
			</div>

			<div className="settings-toggle-grid">
				<label className="settings-toggle-item">
					<span>HTTP to HTTPS redirect</span>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={toggles.http_to_https_redirect}
							onChange={(e) => onChange("http_to_https_redirect", e.target.checked)}
						/>
						<span className="toggle-slider" />
					</span>
				</label>
				<label className="settings-toggle-item">
					<span>Debug logging</span>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={toggles.debug_logging}
							onChange={(e) => onChange("debug_logging", e.target.checked)}
						/>
						<span className="toggle-slider" />
					</span>
				</label>
				<label className="settings-toggle-item">
					<span>Prometheus metrics</span>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={toggles.prometheus_metrics}
							onChange={(e) => onChange("prometheus_metrics", e.target.checked)}
						/>
						<span className="toggle-slider" />
					</span>
				</label>
				<label className="settings-toggle-item">
					<span>Per-host metrics</span>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={toggles.per_host_metrics}
							onChange={(e) => onChange("per_host_metrics", e.target.checked)}
							disabled={!toggles.prometheus_metrics}
						/>
						<span className="toggle-slider" />
					</span>
				</label>
			</div>
		</>
	);
}
