import { useCallback, useEffect, useRef, useState } from "react";
import { fetchLokiConfig, fetchLokiStatus, testLokiConnection, updateLokiConfig } from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { useSettingsSection } from "../hooks/useSettingsSection";
import type { LokiConfig, LokiStatus } from "../types/logs";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

type LabelRow = { id: number; key: string; value: string };

const defaultValues: LokiConfig = {
	enabled: false,
	endpoint: "http://loki:3100",
	bearer_token: "",
	tenant_id: "",
	labels: { job: "kaji" },
	batch_size: 1048576,
	flush_interval_seconds: 5,
	sinks: [],
};

export function LokiSettings() {
	const { values, setValues, dirty, loaded, load, markLoaded, save, saving, feedback } =
		useSettingsSection(defaultValues);
	const { saving: testing, feedback: testFeedback, run: runTest } = useAsyncAction();
	const [status, setStatus] = useState<LokiStatus | null>(null);
	const [pollFailures, setPollFailures] = useState(0);
	const [labelRows, setLabelRows] = useState<LabelRow[]>([]);
	const nextLabelId = useRef(0);

	const labelsToRows = useCallback((labels: Record<string, string>): LabelRow[] => {
		return Object.entries(labels).map(([key, value]) => ({
			id: nextLabelId.current++,
			key,
			value,
		}));
	}, []);

	const rowsToLabels = useCallback((rows: LabelRow[]): Record<string, string> => {
		const labels: Record<string, string> = {};
		for (const row of rows) {
			if (row.key.trim()) {
				labels[row.key.trim()] = row.value;
			}
		}
		return labels;
	}, []);

	useEffect(() => {
		fetchLokiConfig()
			.then((cfg) => {
				load(cfg);
				setLabelRows(labelsToRows(cfg.labels ?? {}));
			})
			.catch(markLoaded);
	}, [load, markLoaded, labelsToRows]);

	useEffect(() => {
		if (!values.enabled) {
			setStatus(null);
			setPollFailures(0);
			return;
		}
		let cancelled = false;
		const poll = () => {
			fetchLokiStatus()
				.then((s) => {
					if (!cancelled) {
						setStatus(s);
						setPollFailures(0);
					}
				})
				.catch(() => {
					if (!cancelled) {
						setPollFailures((n) => n + 1);
					}
				});
		};
		poll();
		const id = setInterval(poll, 10000);
		return () => {
			cancelled = true;
			clearInterval(id);
		};
	}, [values.enabled]);

	const updateLabelRows = (rows: LabelRow[]) => {
		setLabelRows(rows);
		setValues((v) => ({ ...v, labels: rowsToLabels(rows) }));
	};

	const handleSave = () =>
		save(async (v) => {
			const dupes = new Set<string>();
			const seen = new Set<string>();
			for (const row of labelRows) {
				const k = row.key.trim();
				if (!k) continue;
				if (seen.has(k)) dupes.add(k);
				seen.add(k);
			}
			if (dupes.size > 0) {
				throw new Error(`Duplicate label key: ${[...dupes].join(", ")}`);
			}
			const withJob = { ...v, labels: { ...v.labels, job: "kaji" } };
			await updateLokiConfig(withJob);
			setLabelRows(labelsToRows(withJob.labels));
			return "Saved";
		});

	const handleTest = () =>
		runTest(async () => {
			const result = await testLokiConnection();
			if (!result.success) throw new Error(result.message);
			return result.message || "Connection successful";
		});

	if (!loaded) return null;

	return (
		<section className="settings-section">
			<h3>Loki Integration</h3>
			<div className="settings-toggle-row">
				<span>Enable Loki push</span>
				<Toggle
					inline
					small
					id="loki-enabled"
					value={values.enabled}
					onChange={(checked) => setValues((v) => ({ ...v, enabled: checked }))}
					disabled={saving}
				/>
			</div>
			{values.enabled && (
				<div className="loki-config-fields">
					<div className="settings-field">
						<label htmlFor="loki-endpoint">Endpoint URL</label>
						<input
							id="loki-endpoint"
							type="text"
							value={values.endpoint}
							onChange={(e) => setValues((v) => ({ ...v, endpoint: e.target.value }))}
							placeholder="http://loki:3100"
							disabled={saving}
						/>
					</div>
					<div className="settings-field">
						<label htmlFor="loki-bearer-token">Bearer token</label>
						<input
							id="loki-bearer-token"
							type="password"
							value={values.bearer_token}
							onChange={(e) => setValues((v) => ({ ...v, bearer_token: e.target.value }))}
							disabled={saving}
						/>
					</div>
					<div className="settings-field">
						<label htmlFor="loki-tenant-id">Tenant ID</label>
						<input
							id="loki-tenant-id"
							type="text"
							value={values.tenant_id}
							onChange={(e) => setValues((v) => ({ ...v, tenant_id: e.target.value }))}
							disabled={saving}
						/>
						<span className="settings-field-hint">
							Required for multi-tenant Loki setups. Leave empty for single-tenant.
						</span>
					</div>
					<div className="settings-field">
						<label htmlFor="loki-flush-interval">Push logs every</label>
						<div className="loki-flush-row">
							<div className="input-with-unit">
								<input
									id="loki-flush-interval"
									type="number"
									min={1}
									max={60}
									value={values.flush_interval_seconds}
									onChange={(e) =>
										setValues((v) => ({
											...v,
											flush_interval_seconds: Math.max(1, Number(e.target.value) || 1),
										}))
									}
									disabled={saving}
								/>
								<span className="input-unit">sec</span>
							</div>
							<span className="loki-flush-or">or</span>
							<select
								id="loki-batch-size"
								value={values.batch_size}
								onChange={(e) => setValues((v) => ({ ...v, batch_size: Number(e.target.value) }))}
								disabled={saving}
							>
								<option value={512000}>500 KB</option>
								<option value={1048576}>1 MB</option>
								<option value={2097152}>2 MB</option>
								<option value={3145728}>3 MB</option>
							</select>
						</div>
					</div>
					<div className="settings-field">
						<span className="settings-subsection-title">Labels</span>
						<div className="loki-labels">
							<div className="loki-label-row loki-label-fixed">
								<input type="text" value="job" disabled />
								<input type="text" value="kaji" disabled />
							</div>
							{labelRows
								.filter((row) => row.key.trim() !== "job")
								.map((row) => {
									const isDupe =
										row.key.trim() !== "" &&
										labelRows.some((r) => r.id !== row.id && r.key.trim() === row.key.trim());
									return (
										<div key={row.id} className="loki-label-row">
											<input
												type="text"
												placeholder="key"
												value={row.key}
												className={isDupe ? "input-error" : ""}
												onChange={(e) =>
													updateLabelRows(
														labelRows.map((r) =>
															r.id === row.id ? { ...r, key: e.target.value } : r,
														),
													)
												}
												disabled={saving}
											/>
											<input
												type="text"
												placeholder="value"
												value={row.value}
												onChange={(e) =>
													updateLabelRows(
														labelRows.map((r) =>
															r.id === row.id ? { ...r, value: e.target.value } : r,
														),
													)
												}
												disabled={saving}
											/>
											<button
												type="button"
												className="btn btn-ghost btn-sm"
												onClick={() => updateLabelRows(labelRows.filter((r) => r.id !== row.id))}
												disabled={saving}
											>
												Remove
											</button>
										</div>
									);
								})}
							<button
								type="button"
								className="btn btn-ghost btn-sm"
								onClick={() =>
									updateLabelRows([...labelRows, { id: nextLabelId.current++, key: "", value: "" }])
								}
								disabled={saving}
							>
								Add label
							</button>
						</div>
					</div>
					<div className="loki-test-section">
						<button
							type="button"
							className="btn btn-ghost settings-save-btn"
							style={{ marginTop: 0 }}
							disabled={testing || !values.endpoint}
							onClick={handleTest}
						>
							{testing ? "Testing..." : "Test Connection"}
						</button>
						<Feedback msg={testFeedback.msg} type={testFeedback.type} />
					</div>
					{pollFailures >= 3 && (
						<Feedback msg="Unable to reach the Loki status endpoint" type="error" />
					)}
					{status?.running && (
						<div className="loki-status">
							<h4>Pipeline status</h4>
							{Object.entries(status.sinks).map(([sink, s]) => (
								<div key={sink} className="loki-status-sink">
									<span className="loki-status-sink-name">{sink}</span>
									{s.entries_pushed > 0 && (
										<span className="loki-status-detail">
											{s.entries_pushed.toLocaleString()} entries pushed
										</span>
									)}
									{s.last_push_at && (
										<span className="loki-status-detail">
											last push {new Date(s.last_push_at).toLocaleTimeString()}
										</span>
									)}
									{s.last_error && (
										<span className="loki-status-detail loki-status-error">{s.last_error}</span>
									)}
								</div>
							))}
						</div>
					)}
				</div>
			)}
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
