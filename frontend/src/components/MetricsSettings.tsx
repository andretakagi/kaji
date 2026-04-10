import { useEffect, useRef, useState } from "react";
import { fetchGlobalToggles, updateGlobalToggles } from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import type { GlobalToggles } from "../types/api";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

export function MetricsSettings({ caddyRunning }: { caddyRunning: boolean }) {
	const [prometheus, setPrometheus] = useState(false);
	const [perHost, setPerHost] = useState(false);
	const [savedPrometheus, setSavedPrometheus] = useState(false);
	const [savedPerHost, setSavedPerHost] = useState(false);
	const [loaded, setLoaded] = useState(false);
	const { saving, feedback, run } = useAsyncAction();
	const togglesRef = useRef<GlobalToggles | null>(null);

	useEffect(() => {
		if (!caddyRunning) return;
		fetchGlobalToggles()
			.then((t) => {
				togglesRef.current = t;
				setPrometheus(t.prometheus_metrics);
				setPerHost(t.per_host_metrics);
				setSavedPrometheus(t.prometheus_metrics);
				setSavedPerHost(t.per_host_metrics);
				setLoaded(true);
			})
			.catch(() => setLoaded(true));
	}, [caddyRunning]);

	const dirty = prometheus !== savedPrometheus || perHost !== savedPerHost;

	const handleSave = () =>
		run(async () => {
			if (!togglesRef.current) throw new Error("Toggles not loaded");
			const updated = {
				...togglesRef.current,
				prometheus_metrics: prometheus,
				per_host_metrics: perHost,
			};
			await updateGlobalToggles(updated);
			togglesRef.current = updated;
			setSavedPrometheus(prometheus);
			setSavedPerHost(perHost);
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
						checked={prometheus}
						onChange={(checked) => {
							setPrometheus(checked);
							if (!checked) setPerHost(false);
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
						checked={perHost}
						onChange={setPerHost}
						disabled={saving || !prometheus}
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
