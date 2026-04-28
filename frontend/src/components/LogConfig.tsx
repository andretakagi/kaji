import { memo, useCallback, useEffect, useRef, useState } from "react";
import {
	fetchAccessDomains,
	fetchLogConfig,
	fetchLogSkipRules,
	fetchLokiConfig,
	updateLogConfig,
	updateLokiConfig,
} from "../api";
import { useCaddyStatus } from "../contexts/CaddyContext";
import { deepEqual } from "../deepEqual";
import type { Feedback } from "../hooks/useAsyncAction";
import type { CaddyLoggingConfig, CaddyLogSink, LogSkipConfig, LokiConfig } from "../types/logs";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import LoadingState from "./LoadingState";
import { LogSkipRulesEditor } from "./LogSkipRulesEditor";
import { SectionHeader } from "./SectionHeader";
import { Toggle } from "./Toggle";

const NS_PER_DAY = 24 * 3600 * 1e9;

function LogSinkEditor({
	name,
	sink,
	savedSink,
	onSave,
	onChange,
	logDir,
	lokiConfig,
	lokiSinks,
	onLokiToggle,
}: {
	name: string;
	sink: CaddyLogSink;
	savedSink: CaddyLogSink | undefined;
	onSave: () => Promise<void>;
	onChange: (sink: CaddyLogSink) => void;
	logDir: string;
	lokiConfig: LokiConfig | null;
	lokiSinks: string[];
	onLokiToggle: (sink: string, enabled: boolean) => Promise<void>;
}) {
	const output = sink.writer?.output ?? "stdout";
	const isFile = output === "file";
	const roll =
		(sink.writer?.roll_size_mb ?? 0) > 0 ||
		(sink.writer?.roll_keep ?? 0) > 0 ||
		(sink.writer?.roll_keep_for ?? 0) > 0;
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<Feedback | null>(null);
	const [lokiError, setLokiError] = useState("");

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
							<span className="log-config-filepath-prefix">{logDir}</span>
							<input
								id={`${name}-filepath`}
								type="text"
								placeholder="access.log"
								maxLength={255}
								value={sink.writer?.filename ?? ""}
								onChange={(e) => updateWriter({ filename: e.target.value })}
							/>
						</label>
					</div>
				)}
				{isFile && (
					<>
						<div className="log-config-toggle-row">
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
							<div className="log-config-loki-toggle-group">
								<label className="log-config-rotation-toggle">
									<input
										type="checkbox"
										checked={lokiSinks.includes(name)}
										onChange={(e) => {
											setLokiError("");
											onLokiToggle(name, e.target.checked).catch((err) => {
												setLokiError(getErrorMessage(err, "Failed to update Loki config"));
												setTimeout(() => setLokiError(""), 4000);
											});
										}}
										disabled={!lokiConfig?.enabled}
									/>
									Push to Loki
								</label>
								{!lokiConfig?.enabled && (
									<span className="log-config-loki-hint">Configure Loki in Settings to enable</span>
								)}
								{lokiError && <span className="feedback error">{lokiError}</span>}
							</div>
						</div>
						{roll && (
							<div className="log-config-rotation">
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-size`}>Max size (MB)</label>
									<input
										id={`${name}-roll-size`}
										type="number"
										min={1}
										max={10240}
										value={sink.writer?.roll_size_mb ?? 100}
										onChange={(e) =>
											updateWriter({
												roll_size_mb: Math.min(10240, Math.max(1, Number(e.target.value) || 1)),
											})
										}
									/>
								</div>
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-keep`}>Keep files</label>
									<input
										id={`${name}-roll-keep`}
										type="number"
										min={1}
										max={1000}
										value={sink.writer?.roll_keep ?? 5}
										onChange={(e) =>
											updateWriter({
												roll_keep: Math.min(1000, Math.max(1, Number(e.target.value) || 1)),
											})
										}
									/>
								</div>
								<div className="log-config-field">
									<label htmlFor={`${name}-roll-days`}>Keep days</label>
									<input
										id={`${name}-roll-days`}
										type="number"
										min={1}
										max={3650}
										value={Math.round((sink.writer?.roll_keep_for ?? 30 * NS_PER_DAY) / NS_PER_DAY)}
										onChange={(e) =>
											updateWriter({
												roll_keep_for:
													Math.min(3650, Math.max(1, Number(e.target.value) || 1)) * NS_PER_DAY,
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
	logDir,
	lokiConfig,
	lokiSinks,
	onLokiToggle,
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
	logDir: string;
	lokiConfig: LokiConfig | null;
	lokiSinks: string[];
	onLokiToggle: (sink: string, enabled: boolean) => Promise<void>;
}) {
	const [removeError, setRemoveError] = useState("");
	const [skipRules, setSkipRules] = useState<LogSkipConfig | null>(null);
	const [skipRulesError, setSkipRulesError] = useState("");
	const isAccessLog = name === "kaji_access";
	const isDiscard = sink.writer?.output === "discard";
	const showSkipRules = !isDefault && !isDiscard;

	useEffect(() => {
		if (!showSkipRules) return;
		let cancelled = false;
		fetchLogSkipRules(name)
			.then((r) => {
				if (!cancelled) setSkipRules(r);
			})
			.catch((err) => {
				if (!cancelled) setSkipRulesError(getErrorMessage(err, "Failed to load skip rules"));
			});
		return () => {
			cancelled = true;
		};
	}, [name, showSkipRules]);

	const [confirmDisable, setConfirmDisable] = useState(false);
	const hasDomains = isAccessLog && accessDomains && accessDomains.length > 0;

	const handleAccessToggle = (checked: boolean) => {
		if (!checked && hasDomains) {
			setConfirmDisable(true);
			return;
		}
		onToggle?.(checked);
	};

	const actions =
		isDefault || isAccessLog ? (
			<Toggle
				small
				stopPropagation
				value={!isDiscard}
				onChange={(val) => (isAccessLog ? handleAccessToggle(val) : onToggle?.(val))}
			/>
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
			forceExpanded={confirmDisable}
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
				logDir={logDir}
				lokiConfig={lokiConfig}
				lokiSinks={lokiSinks}
				onLokiToggle={onLokiToggle}
			/>
			{showSkipRules && skipRulesError && <div className="feedback error">{skipRulesError}</div>}
			{showSkipRules && skipRules && (
				<LogSkipRulesEditor sinkName={name} initialRules={skipRules} />
			)}
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
						<p className="access-log-domains-empty">No domains using this sink.</p>
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

export function LogConfigList() {
	const { caddyRunning } = useCaddyStatus();
	const [editConfig, setEditConfig] = useState<CaddyLoggingConfig>({ logs: {} });
	const [savedConfig, setSavedConfig] = useState<CaddyLoggingConfig>({ logs: {} });
	const editConfigRef = useRef(editConfig);
	editConfigRef.current = editConfig;
	const [loaded, setLoaded] = useState(false);
	const [loadError, setLoadError] = useState("");
	const [accessDomains, setAccessDomains] = useState<Record<string, Record<string, string>>>({});
	const [lokiConfig, setLokiConfig] = useState<LokiConfig | null>(null);
	const [lokiSinks, setLokiSinks] = useState<string[]>([]);

	useEffect(() => {
		if (!caddyRunning) return;
		Promise.all([fetchLogConfig(), fetchAccessDomains(), fetchLokiConfig()])
			.then(([logData, domainData, loki]) => {
				const normalized = logData?.logs ? logData : { logs: {} };
				setEditConfig(structuredClone(normalized));
				setSavedConfig(structuredClone(normalized));
				setAccessDomains(domainData);
				setLokiConfig(loki);
				setLokiSinks(loki.sinks ?? []);
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
		const currentWriter = current?.writer;
		let updated: CaddyLogSink;
		if (enabled) {
			const output = currentWriter?.filename ? "file" : "stderr";
			updated = {
				...current,
				writer: { ...currentWriter, output },
			};
		} else {
			updated = {
				...current,
				writer: { ...currentWriter, output: "discard" },
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
		const currentWriter = current?.writer;
		let updated: CaddyLogSink;
		if (enabled) {
			const output = currentWriter?.filename ? "file" : "stdout";
			updated = {
				...current,
				writer: { ...currentWriter, output },
			};
		} else {
			updated = {
				...current,
				writer: { ...currentWriter, output: "discard" },
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

	const handleLokiToggle = useCallback(
		async (sink: string, enabled: boolean) => {
			if (!lokiConfig) return;
			const nextSinks = enabled ? [...lokiSinks, sink] : lokiSinks.filter((s) => s !== sink);
			setLokiSinks(nextSinks);
			try {
				await updateLokiConfig({ ...lokiConfig, sinks: nextSinks });
			} catch (err) {
				setLokiSinks(lokiSinks);
				throw err;
			}
		},
		[lokiConfig, lokiSinks],
	);

	const sinkEntries = Object.entries(editConfig.logs ?? {});
	const sortedEntries = [...sinkEntries].sort(([a], [b]) => {
		if (a === "default") return -1;
		if (b === "default") return 1;
		if (a === "kaji_access") return -1;
		if (b === "kaji_access") return 1;
		return a.localeCompare(b);
	});
	if (!loaded) {
		return <LoadingState label="log config" />;
	}

	if (loadError) {
		return <div className="empty-state">{loadError}</div>;
	}

	return (
		<>
			<SectionHeader title="Log Configuration" />

			{sinkEntries.length === 0 ? (
				<div className="empty-state">
					No log outputs configured. Enable access logging on a domain to create one.
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
							logDir={editConfig.log_dir ?? "/var/log/caddy/"}
							lokiConfig={lokiConfig}
							lokiSinks={lokiSinks}
							onLokiToggle={handleLokiToggle}
						/>
					))}
				</div>
			)}
		</>
	);
}
