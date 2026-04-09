import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchConfig, fetchLogs, POLL_INTERVAL } from "../api";
import type { CaddyLogEntry, LogQueryParams } from "../types/logs";
import { getErrorMessage } from "../utils/getErrorMessage";
import Autocomplete from "./Autocomplete";
import { LogConfigList } from "./LogConfig";
import { MetricsSettings } from "./MetricsSettings";

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
