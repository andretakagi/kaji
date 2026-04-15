import { useEffect, useRef } from "react";
import { fetchGlobalToggles, updateGlobalToggles } from "../api";
import { useCaddyStatus } from "../contexts/CaddyContext";
import { useSettingsSection } from "../hooks/useSettingsSection";
import type { GlobalToggles } from "../types/api";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

export function MetricsSettings() {
	const { caddyRunning } = useCaddyStatus();
	const { values, setValues, dirty, loaded, load, markLoaded, save, saving, feedback } =
		useSettingsSection({ prometheus: false, perHost: false });
	const togglesRef = useRef<GlobalToggles | null>(null);

	useEffect(() => {
		if (!caddyRunning) return;
		fetchGlobalToggles()
			.then((t) => {
				togglesRef.current = t;
				load({ prometheus: t.prometheus_metrics, perHost: t.per_host_metrics });
			})
			.catch(markLoaded);
	}, [caddyRunning, load, markLoaded]);

	const handleSave = () =>
		save(async (v) => {
			if (!togglesRef.current) throw new Error("Toggles not loaded");
			const updated = {
				...togglesRef.current,
				prometheus_metrics: v.prometheus,
				per_host_metrics: v.perHost,
			};
			await updateGlobalToggles(updated);
			togglesRef.current = updated;
			return "Saved";
		});

	if (!loaded) return null;

	return (
		<section className="settings-section">
			<h3>Metrics</h3>
			<div className="settings-toggle-grid">
				<label className="settings-toggle-item" htmlFor="metrics-prometheus">
					<div className="settings-toggle-label">
						<span>Prometheus metrics</span>
						<span className="settings-toggle-desc">
							Expose a /metrics endpoint for Prometheus to scrape. Provides request counts, latency
							percentiles, and other server stats for graphing in Grafana or similar tools.
						</span>
					</div>
					<Toggle
						inline
						small
						id="metrics-prometheus"
						value={values.prometheus}
						onChange={(checked) => {
							setValues((v) => ({
								...v,
								prometheus: checked,
								perHost: checked ? v.perHost : false,
							}));
						}}
						disabled={saving}
					/>
				</label>
				<label className="settings-toggle-item" htmlFor="metrics-per-host">
					<div className="settings-toggle-label">
						<span>Per-host metrics</span>
						<span className="settings-toggle-desc">
							Break down metrics by hostname instead of aggregating across all sites. Useful for
							per-site dashboards, but increases cardinality with many hosts.
						</span>
					</div>
					<Toggle
						inline
						small
						id="metrics-per-host"
						value={values.perHost}
						onChange={(checked) => setValues((v) => ({ ...v, perHost: checked }))}
						disabled={saving || !values.prometheus}
					/>
				</label>
			</div>
			{dirty && (
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving}
					onClick={handleSave}
				>
					{saving ? "Saving..." : "Save"}
				</button>
			)}
			<Feedback msg={feedback.msg} type={feedback.type} />
		</section>
	);
}
