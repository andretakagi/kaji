import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchConfig, fetchLogs } from "../api";
import { cn } from "../cn";
import { RequireCaddy, useCaddyStatus } from "../contexts/CaddyContext";
import { formatTime } from "../formatTime";
import type { CaddyLogEntry, LogQueryParams } from "../types/logs";
import { getErrorMessage } from "../utils/getErrorMessage";
import Autocomplete from "./Autocomplete";
import { ErrorAlert } from "./ErrorAlert";
import LoadingState from "./LoadingState";
import { LogConfigList } from "./LogConfig";

const SHOW_OPTIONS = [50, 100, 250, 500, 1000];

const STATUS_RANGES: Record<string, { min?: number; max?: number }> = {
	all: {},
	"2xx": { min: 200, max: 299 },
	"3xx": { min: 300, max: 399 },
	"4xx": { min: 400, max: 499 },
	"5xx": { min: 500, max: 599 },
};

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

export default function Logs() {
	const { caddyRunning } = useCaddyStatus();
	const [historyEntries, setHistoryEntries] = useState<CaddyLogEntry[]>([]);
	const [streamEntries, setStreamEntries] = useState<CaddyLogEntry[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [showCount, setShowCount] = useState(100);
	const [level, setLevel] = useState("");
	const [host, setHost] = useState("");
	const [hosts, setHosts] = useState<string[]>([]);
	const [statusRange, setStatusRange] = useState("all");
	const [streamDisconnected, setStreamDisconnected] = useState(false);
	const [debouncedHost, setDebouncedHost] = useState("");
	const eventSourceRef = useRef<EventSource | null>(null);
	const reconnectTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
	const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);

	useEffect(() => {
		if (debounceRef.current) clearTimeout(debounceRef.current);
		debounceRef.current = setTimeout(() => {
			setDebouncedHost(host);
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

	const fetchHistory = useCallback(async () => {
		if (!caddyRunning) return;
		setLoading(true);
		try {
			const range = STATUS_RANGES[statusRange] ?? {};
			const params: LogQueryParams = { limit: showCount };
			if (level) params.level = level as LogQueryParams["level"];
			if (debouncedHost) params.host = debouncedHost;
			if (range.min) params.status_min = range.min;
			if (range.max) params.status_max = range.max;

			const res = await fetchLogs(params);
			setHistoryEntries(res.entries ?? []);
			setError("");
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load logs"));
		} finally {
			setLoading(false);
		}
	}, [caddyRunning, showCount, level, debouncedHost, statusRange]);

	useEffect(() => {
		fetchHistory();
	}, [fetchHistory]);

	useEffect(() => {
		if (!caddyRunning) {
			setStreamEntries([]);
			setStreamDisconnected(false);
			if (eventSourceRef.current) {
				eventSourceRef.current.close();
				eventSourceRef.current = null;
			}
			clearTimeout(reconnectTimerRef.current);
			return;
		}

		function connect() {
			const es = new EventSource("/api/logs/stream");
			eventSourceRef.current = es;

			es.onopen = () => {
				setStreamDisconnected(false);
			};

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
				reconnectTimerRef.current = setTimeout(connect, 3000);
			};
		}

		connect();

		return () => {
			clearTimeout(reconnectTimerRef.current);
			if (eventSourceRef.current) {
				eventSourceRef.current.close();
				eventSourceRef.current = null;
			}
		};
	}, [caddyRunning]);

	const displayEntries = useMemo(() => {
		const filteredStream = streamEntries.filter((e) => {
			if (level && e.level !== level) return false;
			if (debouncedHost && e.request?.host && !e.request.host.includes(debouncedHost)) return false;
			const range = STATUS_RANGES[statusRange];
			if (range?.min != null && (e.status == null || e.status < range.min)) return false;
			if (range?.max != null && (e.status == null || e.status > range.max)) return false;
			return true;
		});

		const historyKeys = new Set(historyEntries.map((e) => `${e.ts}:${e.msg}`));
		const dedupedStream = filteredStream.filter((e) => !historyKeys.has(`${e.ts}:${e.msg}`));

		return [...dedupedStream, ...historyEntries];
	}, [streamEntries, historyEntries, level, debouncedHost, statusRange]);

	if (!caddyRunning) {
		return (
			<div className="logs">
				<div className="section-header">
					<h2>Logs</h2>
				</div>
				<RequireCaddy message="Start it to view logs." />
			</div>
		);
	}

	if (loading && historyEntries.length === 0 && streamEntries.length === 0) {
		return <LoadingState label="logs" />;
	}

	const hasFilters = level || debouncedHost || statusRange !== "all";

	return (
		<div className="logs">
			<LogConfigList />

			<div className="section-header">
				<h2>
					Log Viewer
					<span className={cn("logs-connection", streamDisconnected && "disconnected")}>
						{streamDisconnected ? "Reconnecting..." : <span className="live-dot" />}
					</span>
				</h2>
			</div>

			<div className="logs-toolbar">
				<div className="logs-filters">
					<div className="logs-filter">
						<label htmlFor="log-show">Show last</label>
						<select
							id="log-show"
							value={showCount}
							onChange={(e) => setShowCount(Number(e.target.value))}
						>
							{SHOW_OPTIONS.map((n) => (
								<option key={n} value={n}>
									{n}
								</option>
							))}
						</select>
					</div>
					<div className="logs-filter">
						<label htmlFor="log-level">Level</label>
						<select id="log-level" value={level} onChange={(e) => setLevel(e.target.value)}>
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
							onChange={(e) => setStatusRange(e.target.value)}
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

			<ErrorAlert message={error} onDismiss={() => setError("")} />

			{displayEntries.length === 0 ? (
				<div className="empty-state">
					{hasFilters
						? "No entries match the current filters."
						: "No log entries found. Logs will appear here once Caddy processes requests."}
				</div>
			) : (
				<div className="log-list" role="log" aria-label="Log entries">
					{displayEntries.map((entry, i) => {
						const isHttp = entry.request != null;
						const key = `${i}-${entry.ts}-${entry.status ?? entry.level}-${entry.request?.host ?? ""}-${entry.request?.uri ?? entry.msg}`;
						return (
							<div className="log-entry" key={key}>
								<span className="log-time">{formatTime(entry.ts, { seconds: true })}</span>
								{isHttp && entry.status != null ? (
									<span className={`log-status ${statusClass(entry.status)}`}>{entry.status}</span>
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
			)}
		</div>
	);
}
