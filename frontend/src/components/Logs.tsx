import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	fetchAccessDomains,
	fetchConfig,
	fetchGlobalToggles,
	fetchLogConfig,
	fetchLogs,
	POLL_INTERVAL,
	updateGlobalToggles,
	updateLogConfig,
} from "../api";
import { deepEqual } from "../deepEqual";
import type { GlobalToggles } from "../types/api";
import type {
	CaddyLogEntry,
	CaddyLoggingConfig,
	CaddyLogSink,
	LogQueryParams,
} from "../types/logs";
import { getErrorMessage } from "../utils/getErrorMessage";
import Autocomplete from "./Autocomplete";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import Feedback from "./Feedback";

const NS_PER_DAY = 24 * 3600 * 1e9;

const PAGE_SIZE = 50;

const STATUS_RANGES: Record<string, { min?: number; max?: number }> = {
	all: {},
	"2xx": { min: 200, max: 299 },
	"3xx": { min: 300, max: 399 },
	"4xx": { min: 400, max: 499 },
	"5xx": { min: 500, max: 599 },
};

function formatTime(ts: number): string {
	const d = new Date(ts * 1000);
	const now = new Date();
	const sameDay =
		d.getFullYear() === now.getFullYear() &&
		d.getMonth() === now.getMonth() &&
		d.getDate() === now.getDate();

	if (sameDay) {
		return d.toLocaleTimeString([], {
			hour: "2-digit",
			minute: "2-digit",
			second: "2-digit",
		});
	}
	return d.toLocaleDateString([], {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

function formatDuration(seconds: number | undefined): string {
	if (seconds == null) return "--";
	const ms = seconds * 1000;
	if (ms < 1) return "<1ms";
	if (ms < 1000) return `${Math.round(ms)}ms`;
	return `${(ms / 1000).toFixed(1)}s`;
}

function statusClass(code: number): string {
	if (code >= 200 && code < 300) return "s2xx";
	if (code >= 300 && code < 400) return "s3xx";
	if (code >= 400 && code < 500) return "s4xx";
	return "s5xx";
}

function LogSinkEditor({
	name,
	sink,
	savedSink,
	onSave,
	onChange,
}: {
	name: string;
	sink: CaddyLogSink;
	savedSink: CaddyLogSink | undefined;
	onSave: () => Promise<void>;
	onChange: (sink: CaddyLogSink) => void;
}) {
	const output = sink.writer?.output ?? "stdout";
	const isFile = output === "file";
	const roll =
		(sink.writer?.roll_size_mb ?? 0) > 0 ||
		(sink.writer?.roll_keep ?? 0) > 0 ||
		(sink.writer?.roll_keep_for ?? 0) > 0;
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<{ msg: string; type: "success" | "error" } | null>(null);

	const dirty = !deepEqual(sink, savedSink);

	async function handleSave() {
		if (output === "file") {
			const filename = sink.writer?.filename?.trim() ?? "";
			if (!filename) {
				setFeedback({ msg: "File name is required", type: "error" });
				return;
			}
			if (filename.includes("/")) {
				setFeedback({ msg: "Enter a file name only, not a path", type: "error" });
				return;
			}
			const ext = filename.split(".").pop()?.toLowerCase();
			const logExtensions = ["log", "txt", "json", "jsonl"];
			if (ext && !logExtensions.includes(ext)) {
				setFeedback({
					msg: `Unexpected file extension ".${ext}" - expected .log, .txt, .json, or .jsonl`,
					type: "error",
				});
				return;
			}
		}
		setSaving(true);
		setFeedback(null);
		try {
			await onSave();
			setFeedback({ msg: "Saved", type: "success" });
			setTimeout(() => setFeedback(null), 2000);
		} catch (err) {
			setFeedback({ msg: getErrorMessage(err, "Failed to save"), type: "error" });
		} finally {
			setSaving(false);
		}
	}

	function updateWriter(patch: Partial<NonNullable<CaddyLogSink["writer"]>>) {
		onChange({ ...sink, writer: { ...sink.writer, output, ...patch } });
	}

	function setOutput(newOutput: string) {
		if (newOutput === "file") {
			updateWriter({ output: "file" });
		} else {
			onChange({
				...sink,
				writer: { output: newOutput },
			});
		}
	}

	return (
		<div className="log-config-sink">
			<div className="log-config-fields">
				<div className="log-config-field">
					<label htmlFor={`${name}-level`}>Level</label>
					<select
						id={`${name}-level`}
						value={sink.level ?? "INFO"}
						onChange={(e) => onChange({ ...sink, level: e.target.value })}
					>
						<option value="DEBUG">DEBUG</option>
						<option value="INFO">INFO</option>
						<option value="WARN">WARN</option>
						<option value="ERROR">ERROR</option>
					</select>
				</div>
				<div className="log-config-field">
					<label htmlFor={`${name}-output`}>Output</label>
					<select id={`${name}-output`} value={output} onChange={(e) => setOutput(e.target.value)}>
						<option value="stdout">stdout</option>
						<option value="stderr">stderr</option>
						<option value="file">file</option>
					</select>
				</div>
				<div className="log-config-field">
					<label htmlFor={`${name}-encoder`}>Encoder</label>
					<select
						id={`${name}-encoder`}
						value={sink.encoder?.format ?? "console"}
						onChange={(e) => onChange({ ...sink, encoder: { format: e.target.value } })}
					>
						<option value="console">console</option>
						<option value="json">json</option>
					</select>
				</div>
				{isFile && (
					<div className="log-config-field">
						<label htmlFor={`${name}-filepath`}>File name</label>
						<label className="log-config-filepath" htmlFor={`${name}-filepath`}>
							<span className="log-config-filepath-prefix">/var/log/caddy/</span>
							<input
								id={`${name}-filepath`}
								type="text"
								placeholder="access.log"
								value={sink.writer?.filename ?? ""}
								onChange={(e) => updateWriter({ filename: e.target.value })}
							/>
						</label>
					</div>
				)}
				{isFile && (
					<>
						<label className="log-config-rotation-toggle">
							<input
								type="checkbox"
								checked={roll}
								onChange={(e) => {
									if (e.target.checked) {
										updateWriter({
											roll_size_mb: 100,
											roll_keep: 5,
											roll_keep_for: 30 * NS_PER_DAY,
										});
									} else {
										const { roll_size_mb, roll_keep, roll_keep_for, ...rest } = sink.writer ?? {
											output,
										};
										onChange({ ...sink, writer: rest as CaddyLogSink["writer"] });
									}
								}}
							/>
							Enable log rotation
						</label>
						{roll && (
							<div className="log-config-rotation">
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-size`}>Max size (MB)</label>
									<input
										id={`${name}-roll-size`}
										type="number"
										min={1}
										value={sink.writer?.roll_size_mb ?? 100}
										onChange={(e) =>
											updateWriter({ roll_size_mb: Math.max(1, Number(e.target.value) || 1) })
										}
									/>
								</div>
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-keep`}>Keep files</label>
									<input
										id={`${name}-roll-keep`}
										type="number"
										min={1}
										value={sink.writer?.roll_keep ?? 5}
										onChange={(e) =>
											updateWriter({ roll_keep: Math.max(1, Number(e.target.value) || 1) })
										}
									/>
								</div>
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-days`}>Keep days</label>
									<input
										id={`${name}-roll-days`}
										type="number"
										min={1}
										value={Math.round((sink.writer?.roll_keep_for ?? 30 * NS_PER_DAY) / NS_PER_DAY)}
										onChange={(e) =>
											updateWriter({
												roll_keep_for: Math.max(1, Number(e.target.value) || 1) * NS_PER_DAY,
											})
										}
									/>
								</div>
							</div>
						)}
					</>
				)}
			</div>
			{(dirty || feedback) && (
				<div className="log-config-sink-footer">
					{dirty && (
						<button
							type="button"
							className="btn btn-primary log-config-save-btn"
							disabled={saving}
							onClick={handleSave}
						>
							{saving ? "Saving..." : "Save"}
						</button>
					)}
					{feedback && (
						<span className={`feedback log-config-feedback ${feedback.type}`}>{feedback.msg}</span>
					)}
				</div>
			)}
		</div>
	);
}

const LogConfigCard = memo(function LogConfigCard({
	name,
	sink,
	savedSink,
	onSave,
	onChange,
	onRemove,
	onToggle,
	accessDomains,
	isDefault,
}: {
	name: string;
	sink: CaddyLogSink;
	savedSink: CaddyLogSink | undefined;
	onSave: (name: string) => Promise<void>;
	onChange: (name: string, sink: CaddyLogSink) => void;
	onRemove: (name: string) => Promise<void>;
	onToggle?: (enabled: boolean) => void;
	accessDomains?: string[];
	isDefault?: boolean;
}) {
	const [removeError, setRemoveError] = useState("");
	const isAccessLog = name === "kaji_access";
	const isDiscard = sink.writer?.output === "discard";

	const [confirmDisable, setConfirmDisable] = useState(false);
	const hasRoutes = isAccessLog && accessDomains && accessDomains.length > 0;

	const handleAccessToggle = (checked: boolean) => {
		if (!checked && hasRoutes) {
			setConfirmDisable(true);
			return;
		}
		onToggle?.(checked);
	};

	const actions =
		isDefault || isAccessLog ? (
			<label
				className="toggle-switch small"
				onClick={(e) => e.stopPropagation()}
				onKeyDown={(e) => e.stopPropagation()}
			>
				<input
					type="checkbox"
					checked={!isDiscard}
					onChange={(e) =>
						isAccessLog ? handleAccessToggle(e.target.checked) : onToggle?.(e.target.checked)
					}
				/>
				<span className="toggle-slider" />
			</label>
		) : (
			<ConfirmDeleteButton
				onConfirm={async () => {
					try {
						await onRemove(name);
					} catch (err) {
						setRemoveError(getErrorMessage(err, "Failed to remove"));
						throw err;
					}
				}}
				label="Remove log sink"
			/>
		);

	const title = isDefault ? (
		"Caddy System Log"
	) : isAccessLog ? (
		<>
			{name} <span className="access-log-badge">Access Log</span>
		</>
	) : (
		name
	);

	return (
		<CollapsibleCard
			title={title}
			actions={actions}
			ariaLabel={name}
			disabled={(isDefault || isAccessLog) && isDiscard}
		>
			{removeError && <div className="feedback error">{removeError}</div>}
			{confirmDisable && (
				<div className="access-log-disable-confirm">
					<p>
						This will disable access logging on{" "}
						{accessDomains?.map((d, i) => (
							<span key={d}>
								{i > 0 && ", "}
								<strong>{d}</strong>
							</span>
						))}
					</p>
					<div className="access-log-disable-actions">
						<button
							type="button"
							className="btn btn-danger btn-sm"
							onClick={() => {
								setConfirmDisable(false);
								onToggle?.(false);
							}}
						>
							Disable
						</button>
						<button
							type="button"
							className="btn btn-ghost btn-sm"
							onClick={() => setConfirmDisable(false)}
						>
							Cancel
						</button>
					</div>
				</div>
			)}
			<LogSinkEditor
				name={name}
				sink={sink}
				savedSink={savedSink}
				onSave={() => onSave(name)}
				onChange={(s) => onChange(name, s)}
			/>
			{isAccessLog && !isDiscard && (
				<div className="access-log-domains">
					{accessDomains && accessDomains.length > 0 ? (
						<div className="access-log-domain-list">
							{accessDomains.map((domain) => (
								<span key={domain} className="access-log-domain-chip">
									{domain}
								</span>
							))}
						</div>
					) : (
						<p className="access-log-domains-empty">No routes using this sink.</p>
					)}
				</div>
			)}
		</CollapsibleCard>
	);
});

function domainsForSink(
	accessDomains: Record<string, Record<string, string>>,
	sinkName: string,
): string[] {
	const result: string[] = [];
	for (const serverDomains of Object.values(accessDomains)) {
		for (const [domain, logger] of Object.entries(serverDomains)) {
			if (logger === sinkName) result.push(domain);
		}
	}
	return result;
}

function MetricsSettings({ caddyRunning }: { caddyRunning: boolean }) {
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

function LogConfigList({ caddyRunning }: { caddyRunning: boolean }) {
	const [editConfig, setEditConfig] = useState<CaddyLoggingConfig>({ logs: {} });
	const [savedConfig, setSavedConfig] = useState<CaddyLoggingConfig>({ logs: {} });
	const editConfigRef = useRef(editConfig);
	editConfigRef.current = editConfig;
	const [loaded, setLoaded] = useState(false);
	const [loadError, setLoadError] = useState("");
	const [accessDomains, setAccessDomains] = useState<Record<string, Record<string, string>>>({});

	useEffect(() => {
		if (!caddyRunning) return;
		Promise.all([fetchLogConfig(), fetchAccessDomains()])
			.then(([logData, domainData]) => {
				const normalized = logData?.logs ? logData : { logs: {} };
				setEditConfig(structuredClone(normalized));
				setSavedConfig(structuredClone(normalized));
				setAccessDomains(domainData);
				setLoaded(true);
			})
			.catch((err) => {
				setLoadError(getErrorMessage(err, "Failed to load config"));
				setLoaded(true);
			});
	}, [caddyRunning]);

	const updateSink = useCallback((name: string, sink: CaddyLogSink) => {
		setEditConfig((prev) => ({
			...prev,
			logs: { ...prev.logs, [name]: sink },
		}));
	}, []);

	const saveSink = useCallback(async (_name: string) => {
		const fullConfig: CaddyLoggingConfig = {
			...editConfigRef.current,
			logs: { ...editConfigRef.current.logs },
		};
		await updateLogConfig(fullConfig);
		setSavedConfig(structuredClone(fullConfig));
		fetchAccessDomains()
			.then(setAccessDomains)
			.catch(() => setAccessDomains({}));
	}, []);

	const removeSink = useCallback(async (name: string) => {
		const nextLogs = { ...editConfigRef.current.logs };
		delete nextLogs[name];
		const fullConfig: CaddyLoggingConfig = { ...editConfigRef.current, logs: nextLogs };
		await updateLogConfig(fullConfig);
		setEditConfig(fullConfig);
		setSavedConfig(structuredClone(fullConfig));
		fetchAccessDomains()
			.then(setAccessDomains)
			.catch(() => setAccessDomains({}));
	}, []);

	const toggleDefaultLogger = useCallback(async (enabled: boolean) => {
		const current = editConfigRef.current.logs?.default;
		let updated: CaddyLogSink;
		if (enabled) {
			updated = {
				...current,
				writer: { output: "stderr" },
				encoder: current?.encoder ?? { format: "console" },
			};
		} else {
			updated = {
				...current,
				writer: { output: "discard" },
			};
		}
		const nextConfig: CaddyLoggingConfig = {
			...editConfigRef.current,
			logs: { ...editConfigRef.current.logs, default: updated },
		};
		try {
			await updateLogConfig(nextConfig);
			setEditConfig(nextConfig);
			setSavedConfig(structuredClone(nextConfig));
		} catch {
			// revert on failure - state remains unchanged
		}
	}, []);

	const toggleAccessLogger = useCallback(async (enabled: boolean) => {
		const current = editConfigRef.current.logs?.kaji_access;
		let updated: CaddyLogSink;
		if (enabled) {
			updated = {
				...current,
				writer: { output: "stdout" },
				include: current?.include ?? ["http.log.access.*"],
			};
		} else {
			updated = {
				...current,
				writer: { output: "discard" },
			};
		}
		const nextConfig: CaddyLoggingConfig = {
			...editConfigRef.current,
			logs: { ...editConfigRef.current.logs, kaji_access: updated },
		};
		try {
			await updateLogConfig(nextConfig);
			setEditConfig(nextConfig);
			setSavedConfig(structuredClone(nextConfig));
			fetchAccessDomains()
				.then(setAccessDomains)
				.catch(() => setAccessDomains({}));
		} catch {
			// revert on failure - state remains unchanged
		}
	}, []);

	const sinkEntries = Object.entries(editConfig.logs ?? {});
	const sortedEntries = [...sinkEntries].sort(([a], [b]) => {
		if (a === "default") return -1;
		if (b === "default") return 1;
		if (a === "kaji_access") return -1;
		if (b === "kaji_access") return 1;
		return a.localeCompare(b);
	});
	if (!loaded) {
		return <div className="empty-state">Loading log config...</div>;
	}

	if (loadError) {
		return <div className="empty-state">{loadError}</div>;
	}

	return (
		<>
			<div className="section-header">
				<h2>Log Configuration</h2>
			</div>

			{sinkEntries.length === 0 ? (
				<div className="empty-state">
					No log outputs configured. Enable access logging on a route to create one.
				</div>
			) : (
				<div className="log-config-list">
					{sortedEntries.map(([name, sink]) => (
						<LogConfigCard
							key={name}
							name={name}
							sink={sink}
							savedSink={savedConfig.logs?.[name]}
							onSave={saveSink}
							onChange={updateSink}
							onRemove={removeSink}
							onToggle={
								name === "default"
									? toggleDefaultLogger
									: name === "kaji_access"
										? toggleAccessLogger
										: undefined
							}
							accessDomains={domainsForSink(accessDomains, name)}
							isDefault={name === "default"}
						/>
					))}
				</div>
			)}
		</>
	);
}

export default function Logs({ caddyRunning }: { caddyRunning: boolean }) {
	const [entries, setEntries] = useState<CaddyLogEntry[]>([]);
	const [hasMore, setHasMore] = useState(false);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [level, setLevel] = useState("");
	const [host, setHost] = useState("");
	const [hosts, setHosts] = useState<string[]>([]);
	const [statusRange, setStatusRange] = useState("all");
	const [page, setPage] = useState(0);
	const [streaming, setStreaming] = useState(false);
	const [streamDisconnected, setStreamDisconnected] = useState(false);
	const [debouncedHost, setDebouncedHost] = useState("");
	const [streamEntries, setStreamEntries] = useState<CaddyLogEntry[]>([]);
	const eventSourceRef = useRef<EventSource | null>(null);
	const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

	useEffect(() => {
		if (debounceRef.current) clearTimeout(debounceRef.current);
		debounceRef.current = setTimeout(() => {
			setDebouncedHost(host);
			setPage(0);
		}, 300);
		return () => {
			if (debounceRef.current) clearTimeout(debounceRef.current);
		};
	}, [host]);

	useEffect(() => {
		if (!caddyRunning) return;
		fetchConfig()
			.then((config) => {
				const found = new Set<string>();
				for (const server of Object.values(config.apps?.http?.servers ?? {})) {
					for (const route of server.routes ?? []) {
						for (const match of route.match ?? []) {
							for (const h of match.host ?? []) {
								found.add(h);
							}
						}
					}
				}
				setHosts(Array.from(found).sort());
			})
			.catch(() => {});
	}, [caddyRunning]);

	const loadLogs = useCallback(async () => {
		try {
			const range = STATUS_RANGES[statusRange] ?? {};
			const params: LogQueryParams = {
				limit: PAGE_SIZE,
				offset: page * PAGE_SIZE,
			};
			if (level) params.level = level as LogQueryParams["level"];
			if (debouncedHost) params.host = debouncedHost;
			if (range.min) params.status_min = range.min;
			if (range.max) params.status_max = range.max;

			const res = await fetchLogs(params);
			setEntries(res.entries ?? []);
			setHasMore(res.has_more);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load logs"));
		} finally {
			setLoading(false);
		}
	}, [level, debouncedHost, statusRange, page]);

	// Stop streaming if Caddy goes offline
	useEffect(() => {
		if (!caddyRunning && streaming) {
			setStreaming(false);
			setStreamEntries([]);
			setStreamDisconnected(false);
		}
	}, [caddyRunning, streaming]);

	useEffect(() => {
		if (streaming || !caddyRunning) return;
		loadLogs();
		const id = setInterval(loadLogs, POLL_INTERVAL);
		return () => clearInterval(id);
	}, [loadLogs, streaming, caddyRunning]);

	useEffect(() => {
		if (eventSourceRef.current) {
			eventSourceRef.current.close();
			eventSourceRef.current = null;
		}

		if (!streaming) return;

		let reconnectTimer: ReturnType<typeof setTimeout>;

		function connect() {
			const es = new EventSource("/api/logs/stream");
			eventSourceRef.current = es;

			es.onmessage = (event) => {
				try {
					const entry: CaddyLogEntry = JSON.parse(event.data);
					setStreamDisconnected(false);
					setStreamEntries((prev) => {
						const next = [entry, ...prev];
						return next.length > 200 ? next.slice(0, 200) : next;
					});
				} catch {
					// non-JSON line, skip
				}
			};

			es.onerror = () => {
				setStreamDisconnected(true);
				es.close();
				eventSourceRef.current = null;
				reconnectTimer = setTimeout(connect, 3000);
			};
		}

		connect();

		return () => {
			clearTimeout(reconnectTimer);
			if (eventSourceRef.current) {
				eventSourceRef.current.close();
				eventSourceRef.current = null;
			}
		};
	}, [streaming]);

	const filteredStreamEntries = useMemo(() => {
		if (!streaming) return [];
		return streamEntries.filter((e) => {
			if (level && e.level !== level) return false;
			if (debouncedHost && e.request?.host && !e.request.host.includes(debouncedHost)) return false;
			const range = STATUS_RANGES[statusRange];
			if (range?.min != null && (e.status == null || e.status < range.min)) return false;
			if (range?.max != null && (e.status == null || e.status > range.max)) return false;
			return true;
		});
	}, [streaming, streamEntries, level, debouncedHost, statusRange]);

	const displayEntries = streaming ? filteredStreamEntries : entries;

	if (!caddyRunning) {
		return (
			<div className="logs">
				<div className="section-header">
					<h2>Logs</h2>
				</div>
				<div className="caddy-offline" role="status">
					Caddy is not running. Start it to view logs.
				</div>
			</div>
		);
	}

	if (loading && !streaming) {
		return <div className="empty-state">Loading logs...</div>;
	}

	const hasFilters = level || debouncedHost || statusRange !== "all";

	return (
		<div className="logs">
			<MetricsSettings caddyRunning={caddyRunning} />
			<LogConfigList caddyRunning={caddyRunning} />

			<div className="section-header">
				<h2>Log Viewer</h2>
			</div>

			<div className="logs-mode-bar">
				<div className="logs-mode-toggle" role="tablist" aria-label="Log viewing mode">
					<button
						type="button"
						role="tab"
						aria-selected={!streaming}
						className={`logs-mode-tab ${!streaming ? "active" : ""}`}
						onClick={() => {
							if (streaming) {
								setStreaming(false);
								setStreamEntries([]);
								setStreamDisconnected(false);
							}
						}}
					>
						History
					</button>
					<button
						type="button"
						role="tab"
						aria-selected={streaming}
						className={`logs-mode-tab ${streaming ? "active" : ""}`}
						onClick={() => {
							if (!streaming) {
								setStreaming(true);
								setStreamEntries([]);
								setStreamDisconnected(false);
							}
						}}
					>
						Live
						{streaming && !streamDisconnected && <span className="live-dot" />}
					</button>
				</div>
				<p className="logs-mode-hint">
					{streaming
						? streamDisconnected
							? "Connection lost. Reconnecting..."
							: "Showing new entries as they arrive."
						: "Browse and search past log entries."}
				</p>
			</div>

			<div className="logs-toolbar">
				<div className="logs-filters">
					<div className="logs-filter">
						<label htmlFor="log-level">Level</label>
						<select
							id="log-level"
							value={level}
							onChange={(e) => {
								setLevel(e.target.value);
								setPage(0);
							}}
						>
							<option value="">All</option>
							<option value="debug">DEBUG</option>
							<option value="info">INFO</option>
							<option value="warn">WARN</option>
							<option value="error">ERROR</option>
						</select>
					</div>
					<div className="logs-filter">
						<label htmlFor="log-host">Host</label>
						<Autocomplete
							id="log-host"
							value={host}
							onChange={setHost}
							options={hosts}
							placeholder="Filter by host"
							minChars={1}
						/>
					</div>
					<div className="logs-filter">
						<label htmlFor="log-status">Status</label>
						<select
							id="log-status"
							value={statusRange}
							onChange={(e) => {
								setStatusRange(e.target.value);
								setPage(0);
							}}
						>
							<option value="all">All</option>
							<option value="2xx">2xx</option>
							<option value="3xx">3xx</option>
							<option value="4xx">4xx</option>
							<option value="5xx">5xx</option>
						</select>
					</div>
				</div>
			</div>

			{error && (
				<div className="alert-error" role="alert">
					{error}
					<button type="button" onClick={() => setError("")}>
						Dismiss
					</button>
				</div>
			)}

			{displayEntries.length === 0 ? (
				<div className="empty-state">
					{streaming
						? "Waiting for new log entries..."
						: hasFilters
							? "No entries match the current filters."
							: "No log entries found. Logs will appear here once Caddy processes requests."}
				</div>
			) : (
				<>
					<div className="log-list" role="log" aria-label="Log entries">
						{displayEntries.map((entry, i) => {
							const isHttp = entry.request != null;
							const key = `${i}-${entry.ts}-${entry.status ?? entry.level}-${entry.request?.host ?? ""}-${entry.request?.uri ?? entry.msg}`;
							return (
								<div className="log-entry" key={key}>
									<span className="log-time">{formatTime(entry.ts)}</span>
									{isHttp && entry.status != null ? (
										<span className={`log-status ${statusClass(entry.status)}`}>
											{entry.status}
										</span>
									) : (
										<span className={`log-level level-${entry.level}`}>{entry.level}</span>
									)}
									{isHttp ? (
										<>
											<span className="log-method">{entry.request?.method}</span>
											<span className="log-path">
												<span className="log-path-host">{entry.request?.host}</span>
												<span className="log-path-uri">{entry.request?.uri}</span>
											</span>
											<span className="log-duration">{formatDuration(entry.duration)}</span>
										</>
									) : (
										<span className="log-msg">
											{entry.logger ? `${entry.logger}: ` : ""}
											{entry.msg}
											{entry.extra && Object.keys(entry.extra).length > 0 && (
												<span className="log-extra">
													{Object.entries(entry.extra).map(([k, v]) => (
														<span key={k} className="log-extra-field">
															<span className="log-extra-key">{k}</span>
															<span className="log-extra-value">
																{typeof v === "string" ? v : JSON.stringify(v)}
															</span>
														</span>
													))}
												</span>
											)}
										</span>
									)}
								</div>
							);
						})}
					</div>

					{!streaming && (
						<div className="logs-pagination">
							<button type="button" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
								Newer
							</button>
							<span>Page {page + 1}</span>
							<button type="button" disabled={!hasMore} onClick={() => setPage((p) => p + 1)}>
								Older
							</button>
						</div>
					)}
				</>
			)}
		</div>
	);
}
