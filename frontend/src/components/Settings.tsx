import { useCallback, useEffect, useState } from "react";
import {
	exportCaddyfile,
	fetchAdvancedSettings,
	fetchAPIKeyStatus,
	fetchAuthStatus,
	generateAPIKey,
	revokeAPIKey,
	updateAdvancedSettings,
} from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { validateCaddyAdminUrl } from "../utils/validate";
import AuthSection from "./AuthSection";
import Feedback from "./Feedback";

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
						className={`theme-pill${theme === "dark" ? " active" : ""}`}
						onClick={() => applyTheme("dark")}
						aria-pressed={theme === "dark"}
					>
						Dark
					</button>
					<button
						type="button"
						className={`theme-pill${theme === "light" ? " active" : ""}`}
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

function AdvancedSection({ initial }: { initial: { caddy_admin_url: string } }) {
	const [caddyAdminUrl, setCaddyAdminUrl] = useState(initial.caddy_admin_url);
	useEffect(() => {
		setCaddyAdminUrl(initial.caddy_admin_url);
	}, [initial.caddy_admin_url]);
	const { saving, feedback, run } = useAsyncAction();

	const handleSave = () =>
		run(async () => {
			const urlError = validateCaddyAdminUrl(caddyAdminUrl);
			if (urlError) throw new Error(urlError);
			await updateAdvancedSettings({
				caddy_admin_url: caddyAdminUrl,
			});
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

export default function Settings({
	onAuthChange,
	caddyRunning,
}: {
	onAuthChange: (enabled: boolean) => void;
	caddyRunning: boolean;
}) {
	const [authEnabled, setAuthEnabled] = useState(false);
	const [advanced, setAdvanced] = useState({
		caddy_admin_url: "http://localhost:2019",
	});
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const [failedSections, setFailedSections] = useState<Set<string>>(new Set());

	const load = useCallback(() => {
		setLoading(true);
		setError("");
		setFailedSections(new Set());

		Promise.allSettled([fetchAuthStatus(), fetchAdvancedSettings()])
			.then(([authResult, advancedResult]) => {
				const failed = new Set<string>();

				if (authResult.status === "fulfilled") {
					const auth = authResult.value as { auth_enabled: boolean; has_password: boolean };
					setAuthEnabled(auth.auth_enabled);
				} else {
					failed.add("auth");
				}
				if (advancedResult.status === "fulfilled") {
					setAdvanced(advancedResult.value as { caddy_admin_url: string });
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

	const [prevCaddyRunning, setPrevCaddyRunning] = useState(caddyRunning);
	if (caddyRunning !== prevCaddyRunning) {
		setPrevCaddyRunning(caddyRunning);
		if (caddyRunning && !loading) {
			load();
		}
	}

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
