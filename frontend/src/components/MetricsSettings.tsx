import { useEffect, useRef, useState } from "react";
import { fetchGlobalToggles, updateGlobalToggles } from "../api";
import type { GlobalToggles } from "../types/api";
import Feedback from "./Feedback";

export function MetricsSettings({ caddyRunning }: { caddyRunning: boolean }) {
	const [prometheus, setPrometheus] = useState(false);
	const [perHost, setPerHost] = useState(false);
	const [savedPrometheus, setSavedPrometheus] = useState(false);
	const [savedPerHost, setSavedPerHost] = useState(false);
	const [loaded, setLoaded] = useState(false);
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<{ msg: string; type: "success" | "error" }>({
		msg: "",
		type: "success",
	});
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

	const handleSave = async () => {
		if (!togglesRef.current) return;
		setSaving(true);
		setFeedback({ msg: "", type: "success" });
		try {
			const updated = {
				...togglesRef.current,
				prometheus_metrics: prometheus,
				per_host_metrics: perHost,
			};
			await updateGlobalToggles(updated);
			togglesRef.current = updated;
			setSavedPrometheus(prometheus);
			setSavedPerHost(perHost);
			setFeedback({ msg: "Saved", type: "success" });
		} catch {
			setFeedback({ msg: "Failed to save", type: "error" });
		} finally {
			setSaving(false);
		}
	};

	if (!loaded) return null;

	return (
		<section className="settings-section">
			<h3>Metrics</h3>
			<div className="settings-toggle-grid">
				<label className="settings-toggle-item">
					<div className="settings-toggle-label">
						<span>Prometheus metrics</span>
						<span className="settings-toggle-desc">
							Expose a /metrics endpoint for Prometheus to scrape. Provides request counts, latency
							percentiles, and other server stats for graphing in Grafana or similar tools.
						</span>
					</div>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={prometheus}
							onChange={(e) => {
								setPrometheus(e.target.checked);
								if (!e.target.checked) setPerHost(false);
							}}
							disabled={saving}
						/>
						<span className="toggle-slider" />
					</span>
				</label>
				<label className="settings-toggle-item">
					<div className="settings-toggle-label">
						<span>Per-host metrics</span>
						<span className="settings-toggle-desc">
							Break down metrics by hostname instead of aggregating across all sites. Useful for
							per-site dashboards, but increases cardinality with many hosts.
						</span>
					</div>
					<span className="toggle-switch small">
						<input
							type="checkbox"
							checked={perHost}
							onChange={(e) => setPerHost(e.target.checked)}
							disabled={saving || !prometheus}
						/>
						<span className="toggle-slider" />
					</span>
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
