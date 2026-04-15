import { useCallback, useEffect, useState } from "react";
import {
	exportCaddyfile,
	fetchAdvancedSettings,
	fetchAPIKeyStatus,
	fetchAuthStatus,
	fetchCaddyDataDir,
	generateAPIKey,
	revokeAPIKey,
	updateAdvancedSettings,
	updateCaddyDataDir,
} from "../api";
import { cn } from "../cn";
import { useCaddyStatus } from "../contexts/CaddyContext";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { validateCaddyAdminUrl } from "../utils/validate";
import AuthSection from "./AuthSection";
import Feedback from "./Feedback";
import HttpsSettingsSection from "./HttpsSettingsSection";
import { LokiSettings } from "./LokiSettings";
import { MetricsSettings } from "./MetricsSettings";
import { SnapshotSettings } from "./SnapshotSettings";

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
				<div className="theme-switcher">
					<button
						type="button"
						className={cn("theme-pill", theme === "dark" && "active")}
						onClick={() => applyTheme("dark")}
						aria-pressed={theme === "dark"}
					>
						Dark
					</button>
					<button
						type="button"
						className={cn("theme-pill", theme === "light" && "active")}
						onClick={() => applyTheme("light")}
						aria-pressed={theme === "light"}
					>
						Light
					</button>
				</div>
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

function ExportCaddyfileSection() {
	const { saving, feedback, run } = useAsyncAction();

	const handleExport = () =>
		run(async () => {
			const content = await exportCaddyfile();
			const blob = new Blob([content], { type: "application/octet-stream" });
			const url = URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			a.download = "Caddyfile";
			a.click();
			URL.revokeObjectURL(url);
			return "Downloaded";
		});

	return (
		<section className="settings-section">
			<h3>Export Caddyfile</h3>
			<p className="settings-description">
				Download the current Caddy configuration as a Caddyfile.
			</p>
			<button
				type="button"
				className="btn btn-primary settings-save-btn"
				disabled={saving}
				onClick={handleExport}
			>
				{saving ? "Exporting..." : "Export"}
			</button>
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}

function AdvancedSection({
	initial,
}: {
	initial: {
		caddy_admin_url: string;
		caddy_data_dir: string;
		caddy_data_dir_placeholder: string;
	};
}) {
	const [caddyAdminUrl, setCaddyAdminUrl] = useState(initial.caddy_admin_url);
	const [caddyDataDir, setCaddyDataDir] = useState(initial.caddy_data_dir);
	useEffect(() => {
		setCaddyAdminUrl(initial.caddy_admin_url);
		setCaddyDataDir(initial.caddy_data_dir);
	}, [initial.caddy_admin_url, initial.caddy_data_dir]);
	const { saving, feedback, run } = useAsyncAction();

	const handleSave = () =>
		run(async () => {
			const urlError = validateCaddyAdminUrl(caddyAdminUrl);
			if (urlError) throw new Error(urlError);
			await updateAdvancedSettings({ caddy_admin_url: caddyAdminUrl });
			await updateCaddyDataDir(caddyDataDir);
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

	useEffect(() => {
		if (caddyRunning && !loading) {
			load();
		}
	}, [caddyRunning, load, loading]);

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

			{!caddyRunning ? <CaddyOffSection title="Export Caddyfile" /> : <ExportCaddyfileSection />}

			{failedSections.has("advanced") ? (
				<section className="settings-section settings-section-failed">
					<h3>Advanced</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<AdvancedSection initial={advanced} />
			)}
		</div>
	);
}
