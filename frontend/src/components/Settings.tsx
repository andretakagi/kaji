import { useCallback, useEffect, useRef, useState } from "react";
import {
	exportCaddyfile,
	exportFull,
	fetchAdvancedSettings,
	fetchAPIKeyStatus,
	fetchAuthStatus,
	fetchCaddyDataDir,
	fetchPorts,
	generateAPIKey,
	importCaddyfile,
	importFull,
	revokeAPIKey,
	updateAdvancedSettings,
	updateCaddyDataDir,
	updatePorts,
} from "../api";
import { useCaddyStatus } from "../contexts/CaddyContext";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { validateCaddyAdminUrl } from "../utils/validate";
import AuthSection from "./AuthSection";
import Feedback from "./Feedback";
import FileUploadButton from "./FileUploadButton";
import HttpsSettingsSection from "./HttpsSettingsSection";
import { LokiSettings } from "./LokiSettings";
import { MetricsSettings } from "./MetricsSettings";
import { SnapshotSettings } from "./SnapshotSettings";
import { Toggle } from "./Toggle";

function AppearanceSection() {
	const [theme, setTheme] = useState(() => localStorage.getItem("theme") || "dark");

	const applyTheme = (t: "dark" | "light") => {
		setTheme(t);
		document.documentElement.setAttribute("data-theme", t);
		localStorage.setItem("theme", t);
		const cs = document.querySelector('meta[name="color-scheme"]');
		if (cs) cs.setAttribute("content", t);
		const tc = document.querySelector('meta[name="theme-color"]');
		if (tc) tc.setAttribute("content", t === "light" ? "#f0edf4" : "#1a1d28");
	};

	return (
		<section className="settings-section">
			<h3>Appearance</h3>
			<div className="settings-toggle-row">
				<span>Theme</span>
				<Toggle
					options={["Dark", "Light"] as const}
					value={theme === "light" ? "Light" : "Dark"}
					onChange={(v: string) => applyTheme(v === "Light" ? "light" : "dark")}
				/>
			</div>
		</section>
	);
}

function CaddyOffSection({ title }: { title: string }) {
	return (
		<section className="settings-section settings-section-failed">
			<h3>{title}</h3>
			<p className="settings-section-error">Caddy is not running</p>
		</section>
	);
}

function APIKeySection() {
	const [hasKey, setHasKey] = useState(false);
	const [visibleKey, setVisibleKey] = useState("");
	const [loading, setLoading] = useState(true);
	const { saving, feedback, setFeedback, run } = useAsyncAction();

	useEffect(() => {
		fetchAPIKeyStatus()
			.then((res) => setHasKey(res.has_api_key))
			.finally(() => setLoading(false));
	}, []);

	const handleGenerate = () =>
		run(async () => {
			const res = await generateAPIKey();
			setVisibleKey(res.api_key);
			setHasKey(true);
			return "Copy this key now - it won't be shown again.";
		});

	const handleRevoke = () =>
		run(async () => {
			await revokeAPIKey();
			setHasKey(false);
			setVisibleKey("");
			return "API key revoked";
		});

	const handleCopy = async () => {
		try {
			await navigator.clipboard.writeText(visibleKey);
			setFeedback({ msg: "Copied to clipboard", type: "success" });
		} catch {
			setFeedback({ msg: "Failed to copy - clipboard access denied", type: "error" });
		}
	};

	if (loading) return null;

	return (
		<section className="settings-section">
			<h3>API Key</h3>
			<p className="settings-description">
				Use a Bearer token to authenticate API requests from scripts and automation.
			</p>
			{visibleKey && (
				<div className="api-key-display">
					<input type="text" readOnly value={visibleKey} aria-label="Generated API key" />
					<button type="button" className="btn btn-primary settings-save-btn" onClick={handleCopy}>
						Copy
					</button>
				</div>
			)}
			<div className="api-key-actions">
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving}
					onClick={handleGenerate}
				>
					{saving ? "Generating..." : hasKey ? "Regenerate" : "Generate"}
				</button>
				{hasKey && (
					<button
						type="button"
						className="btn btn-danger settings-danger-btn"
						disabled={saving}
						onClick={handleRevoke}
					>
						Revoke
					</button>
				)}
			</div>
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}

function ExportImportSection({ onImport }: { onImport: () => void }) {
	const { saving: exporting, feedback: exportFeedback, run: runExport } = useAsyncAction();
	const { saving: importing, feedback: importFeedback, run: runImport } = useAsyncAction();
	const [confirmAction, setConfirmAction] = useState<null | "caddyfile" | "full">(null);
	const [pendingFile, setPendingFile] = useState<File | null>(null);

	const handleExportCaddyfile = () =>
		runExport(async () => {
			const content = await exportCaddyfile();
			const blob = new Blob([content], { type: "application/octet-stream" });
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = "Caddyfile";
			a.click();
			URL.revokeObjectURL(url);
			return "Downloaded Caddyfile";
		});

	const handleExportFull = () =>
		runExport(async () => {
			const blob = await exportFull();
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = `kaji-export-${new Date().toISOString().slice(0, 10)}.zip`;
			a.click();
			URL.revokeObjectURL(url);
			return "Downloaded full backup";
		});

	const handleCaddyfileFileChange = (file: File) => {
		setPendingFile(file);
		setConfirmAction("caddyfile");
	};

	const handleFullFileChange = (file: File) => {
		setPendingFile(file);
		setConfirmAction("full");
	};

	const handleConfirmImport = () => {
		const action = confirmAction;
		const file = pendingFile;
		setConfirmAction(null);
		setPendingFile(null);
		if (!file || !action) return;

		runImport(async () => {
			if (action === "caddyfile") {
				const text = await file.text();
				await importCaddyfile(text);
				onImport();
				return "Caddyfile imported successfully";
			}
			const result = await importFull(file);
			onImport();
			const parts = ["Full backup imported"];
			if (result.domain_count !== undefined) {
				parts.push(`${result.domain_count} ${result.domain_count === 1 ? "domain" : "domains"}`);
			}
			if (result.snapshot_count) {
				parts.push(
					`${result.snapshot_count} ${result.snapshot_count === 1 ? "snapshot" : "snapshots"}`,
				);
			}
			let message =
				parts.length > 1
					? `${parts[0]}: ${parts.slice(1).join(", ")}. Reload to see changes.`
					: `${parts[0]} successfully. Reload to see changes.`;
			if (result.migrated_from && result.migration_log?.length) {
				message += `\n\nMigrated from ${result.migrated_from}:\n${result.migration_log.map((c) => `  - ${c}`).join("\n")}`;
			}
			if (result.warnings?.length) {
				message += `\n\nPath adjustments:\n${result.warnings.map((w) => `  - ${w}`).join("\n")}`;
			}
			return message;
		});
	};

	const handleCancelImport = () => {
		setConfirmAction(null);
		setPendingFile(null);
	};

	return (
		<section className="settings-section">
			<h3>Export & Import</h3>

			<div className="export-import-group">
				<p className="settings-description">Export your configuration for backup or migration.</p>
				<div className="export-import-actions">
					<button
						type="button"
						className="btn btn-primary"
						disabled={exporting}
						onClick={handleExportCaddyfile}
					>
						{exporting ? "Exporting..." : "Export Caddyfile"}
					</button>
					<button
						type="button"
						className="btn btn-primary"
						disabled={exporting}
						onClick={handleExportFull}
					>
						{exporting ? "Exporting..." : "Export Full Backup"}
					</button>
				</div>
				<Feedback
					msg={exportFeedback.msg}
					type={exportFeedback.type}
					className="settings-feedback"
				/>
			</div>

			<div className="export-import-group">
				<p className="settings-description">
					Import a Caddyfile or full backup. This will override your current configuration. A
					snapshot will be created automatically before importing.
				</p>
				<div className="export-import-actions">
					<FileUploadButton
						className="btn btn-ghost"
						disabled={importing}
						onChange={handleCaddyfileFileChange}
					>
						Import Caddyfile
					</FileUploadButton>
					<FileUploadButton
						accept=".zip"
						className="btn btn-ghost"
						disabled={importing}
						onChange={handleFullFileChange}
					>
						Import Full Backup
					</FileUploadButton>
				</div>
				<Feedback
					msg={importFeedback.msg}
					type={importFeedback.type}
					className="settings-feedback"
				/>
			</div>

			{confirmAction && (
				<div className="confirm-dialog-overlay">
					<div
						className="confirm-dialog"
						role="dialog"
						aria-modal="true"
						aria-labelledby="confirm-import-title"
					>
						<h4 id="confirm-import-title">Confirm Import</h4>
						<p>
							{confirmAction === "full"
								? "This will replace all current settings, domains, and configuration with the backup. A snapshot of the current state will be created first."
								: "This will load the Caddyfile into Caddy, replacing the current domain configuration. A snapshot of the current state will be created first."}
						</p>
						<div className="confirm-dialog-actions">
							<button type="button" className="btn btn-ghost" onClick={handleCancelImport}>
								Cancel
							</button>
							<button type="button" className="btn btn-danger" onClick={handleConfirmImport}>
								{importing ? "Importing..." : "Import"}
							</button>
						</div>
					</div>
				</div>
			)}
		</section>
	);
}

function AdvancedSection({
	initial,
	caddyRunning,
}: {
	initial: {
		caddy_admin_url: string;
		caddy_data_dir: string;
		caddy_data_dir_placeholder: string;
	};
	caddyRunning: boolean;
}) {
	const [caddyAdminUrl, setCaddyAdminUrl] = useState(initial.caddy_admin_url);
	const [caddyDataDir, setCaddyDataDir] = useState(initial.caddy_data_dir);
	const [httpPort, setHttpPort] = useState(80);
	const [httpsPort, setHttpsPort] = useState(443);
	const [portsLoaded, setPortsLoaded] = useState(false);

	useEffect(() => {
		setCaddyAdminUrl(initial.caddy_admin_url);
		setCaddyDataDir(initial.caddy_data_dir);
	}, [initial.caddy_admin_url, initial.caddy_data_dir]);

	useEffect(() => {
		if (!caddyRunning) {
			setPortsLoaded(false);
			return;
		}
		fetchPorts()
			.then((ports) => {
				setHttpPort(ports.http_port);
				setHttpsPort(ports.https_port);
				setPortsLoaded(true);
			})
			.catch(() => setPortsLoaded(false));
	}, [caddyRunning]);

	const { saving, feedback, run } = useAsyncAction();

	const handleSave = () =>
		run(async () => {
			const urlError = validateCaddyAdminUrl(caddyAdminUrl);
			if (urlError) throw new Error(urlError);

			if (portsLoaded) {
				if (httpPort < 1 || httpPort > 65535)
					throw new Error("HTTP port must be between 1 and 65535");
				if (httpsPort < 1 || httpsPort > 65535)
					throw new Error("HTTPS port must be between 1 and 65535");
				if (httpPort === httpsPort) throw new Error("HTTP and HTTPS ports must be different");
			}

			await updateAdvancedSettings({ caddy_admin_url: caddyAdminUrl });
			await updateCaddyDataDir(caddyDataDir);
			if (portsLoaded) {
				await updatePorts({ http_port: httpPort, https_port: httpsPort });
			}
			return "Saved";
		});

	return (
		<section className="settings-section">
			<h3>Advanced</h3>
			<div className="settings-field">
				<label htmlFor="caddy-admin-url">Caddy admin URL</label>
				<input
					id="caddy-admin-url"
					type="text"
					value={caddyAdminUrl}
					onChange={(e) => setCaddyAdminUrl(e.target.value)}
					placeholder="http://localhost:2019"
					maxLength={2048}
				/>
			</div>
			<div className="settings-field">
				<label htmlFor="caddy-data-dir">Caddy data directory</label>
				<input
					id="caddy-data-dir"
					type="text"
					value={caddyDataDir}
					onChange={(e) => setCaddyDataDir(e.target.value)}
					placeholder={initial.caddy_data_dir_placeholder}
					maxLength={2048}
				/>
			</div>
			{portsLoaded && (
				<>
					<h4 className="settings-subsection-heading">Ports</h4>
					<div className="settings-field">
						<label htmlFor="http-port">HTTP port</label>
						<input
							id="http-port"
							type="number"
							value={httpPort}
							onChange={(e) => setHttpPort(Number(e.target.value))}
							min={1}
							max={65535}
						/>
					</div>
					<div className="settings-field">
						<label htmlFor="https-port">HTTPS port</label>
						<input
							id="https-port"
							type="number"
							value={httpsPort}
							onChange={(e) => setHttpsPort(Number(e.target.value))}
							min={1}
							max={65535}
						/>
					</div>
				</>
			)}
			<button
				type="button"
				className="btn btn-primary settings-save-btn"
				disabled={saving || !caddyAdminUrl}
				onClick={handleSave}
			>
				{saving ? "Saving..." : "Save"}
			</button>
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}

export default function Settings({ onAuthChange }: { onAuthChange: (enabled: boolean) => void }) {
	const { caddyRunning } = useCaddyStatus();
	const [authEnabled, setAuthEnabled] = useState(false);
	const [advanced, setAdvanced] = useState({
		caddy_admin_url: "http://localhost:2019",
		caddy_data_dir: "",
		caddy_data_dir_placeholder: "/data/caddy",
	});
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [failedSections, setFailedSections] = useState<Set<string>>(new Set());

	const load = useCallback(() => {
		setLoading(true);
		setError("");
		setFailedSections(new Set());

		Promise.allSettled([fetchAuthStatus(), fetchAdvancedSettings(), fetchCaddyDataDir()])
			.then(([authResult, advancedResult, dataDirResult]) => {
				const failed = new Set<string>();

				if (authResult.status === "fulfilled") {
					const auth = authResult.value as { auth_enabled: boolean; has_password: boolean };
					setAuthEnabled(auth.auth_enabled);
				} else {
					failed.add("auth");
				}

				let dataDirOverride = "";
				let dataDirResolved = "/data/caddy";
				if (dataDirResult.status === "fulfilled") {
					const dd = dataDirResult.value as {
						caddy_data_dir: string;
						is_override: string;
					};
					dataDirResolved = dd.caddy_data_dir;
					dataDirOverride = dd.is_override === "true" ? dd.caddy_data_dir : "";
				}

				if (advancedResult.status === "fulfilled") {
					const adv = advancedResult.value as { caddy_admin_url: string };
					setAdvanced({
						caddy_admin_url: adv.caddy_admin_url,
						caddy_data_dir: dataDirOverride,
						caddy_data_dir_placeholder: dataDirResolved,
					});
				} else {
					failed.add("advanced");
				}

				if (failed.has("auth")) {
					setError("critical");
					return;
				}
				setFailedSections(failed);
			})
			.finally(() => setLoading(false));
	}, []);

	useEffect(() => {
		load();
	}, [load]);

	const prevCaddyRunning = useRef(caddyRunning);
	useEffect(() => {
		if (caddyRunning && !prevCaddyRunning.current) {
			load();
		}
		prevCaddyRunning.current = caddyRunning;
	}, [caddyRunning, load]);

	if (loading) {
		return <div className="empty-state settings-loading">Loading settings...</div>;
	}

	if (error) {
		const errorMessage = caddyRunning
			? "Could not load settings from the backend."
			: "Caddy is not running. Start Caddy to manage all settings.";
		return (
			<div className="empty-state settings-error">
				<p>{errorMessage}</p>
				<button type="button" className="btn btn-primary settings-save-btn" onClick={load}>
					Retry
				</button>
			</div>
		);
	}

	return (
		<div className="settings">
			<h2 className="sr-only">Settings</h2>
			{failedSections.size > 0 && caddyRunning && (
				<div className="settings-partial-error" role="alert">
					Some settings could not be loaded.
					<button type="button" className="settings-retry-link" onClick={load}>
						Retry
					</button>
				</div>
			)}

			<AppearanceSection />

			{failedSections.has("auth") ? (
				<section className="settings-section settings-section-failed">
					<h3>Authentication</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<>
					<AuthSection
						enabled={authEnabled}
						onChange={(enabled) => {
							setAuthEnabled(enabled);
							onAuthChange(enabled);
						}}
					/>
					<APIKeySection />
				</>
			)}

			{!caddyRunning ? <CaddyOffSection title="HTTPS" /> : <HttpsSettingsSection />}

			{!caddyRunning ? <CaddyOffSection title="Metrics" /> : <MetricsSettings />}

			{!caddyRunning ? <CaddyOffSection title="Loki Integration" /> : <LokiSettings />}

			<SnapshotSettings />

			{!caddyRunning ? (
				<CaddyOffSection title="Export & Import" />
			) : (
				<ExportImportSection onImport={load} />
			)}

			{failedSections.has("advanced") ? (
				<section className="settings-section settings-section-failed">
					<h3>Advanced</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<AdvancedSection initial={advanced} caddyRunning={caddyRunning} />
			)}
		</div>
	);
}
