import { useCallback, useEffect, useState } from "react";
import {
	changePassword,
	exportCaddyfile,
	fetchACMEEmail,
	fetchAdvancedSettings,
	fetchAPIKeyStatus,
	fetchAuthStatus,
	fetchGlobalToggles,
	generateAPIKey,
	revokeAPIKey,
	updateACMEEmail,
	updateAdvancedSettings,
	updateAuthEnabled,
	updateGlobalToggles,
} from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { DEFAULT_GLOBAL_TOGGLES, type GlobalToggles } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validateCaddyAdminUrl } from "../utils/validate";
import Feedback from "./Feedback";
import { GlobalTogglesForm } from "./GlobalTogglesForm";

function CaddyOffSection({ title }: { title: string }) {
	return (
		<section className="settings-section settings-section-failed">
			<h3>{title}</h3>
			<p className="settings-section-error">Caddy is not running</p>
		</section>
	);
}

function ACMEEmailSection({ initial }: { initial: string }) {
	const [email, setEmail] = useState(initial);
	useEffect(() => setEmail(initial), [initial]);
	const { saving, feedback, run } = useAsyncAction();

	const handleSave = () =>
		run(async () => {
			await updateACMEEmail(email);
			return "Saved";
		});

	return (
		<section className="settings-section">
			<h3>ACME Email</h3>
			<div className="settings-field">
				<label htmlFor="acme-email">Email for Let's Encrypt certificates</label>
				<input
					id="acme-email"
					type="email"
					value={email}
					onChange={(e) => setEmail(e.target.value)}
					placeholder="you@example.com"
				/>
			</div>
			<button
				type="button"
				className="btn btn-primary settings-save-btn"
				disabled={saving || !email}
				onClick={handleSave}
			>
				{saving ? "Saving..." : "Save"}
			</button>
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}

function AuthSection({
	enabled,
	hasPassword,
	onChange,
}: {
	enabled: boolean;
	hasPassword: boolean;
	onChange: (v: boolean, nowHasPassword: boolean) => void;
}) {
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState("");
	const [showPasswordForm, setShowPasswordForm] = useState(false);
	const [newPw, setNewPw] = useState("");
	const [confirmPw, setConfirmPw] = useState("");

	const [cpNew, setCpNew] = useState("");
	const [cpConfirm, setCpConfirm] = useState("");
	const cp = useAsyncAction();

	const handleToggle = async () => {
		if (!enabled && !hasPassword) {
			setShowPasswordForm(true);
			return;
		}

		setSaving(true);
		setError("");
		try {
			await updateAuthEnabled(!enabled);
			onChange(!enabled, hasPassword);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to update"));
		} finally {
			setSaving(false);
		}
	};

	const handleSetPassword = async (e: React.FormEvent) => {
		e.preventDefault();
		setError("");

		if (newPw.length < 8) {
			setError("Password must be at least 8 characters");
			return;
		}
		if (newPw !== confirmPw) {
			setError("Passwords do not match");
			return;
		}

		setSaving(true);
		try {
			await updateAuthEnabled(true, newPw);
			setShowPasswordForm(false);
			setNewPw("");
			setConfirmPw("");
			onChange(true, true);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to enable auth"));
		} finally {
			setSaving(false);
		}
	};

	const handleCancel = () => {
		setShowPasswordForm(false);
		setNewPw("");
		setConfirmPw("");
		setError("");
	};

	const handleChangePassword = (e: React.FormEvent) => {
		e.preventDefault();

		if (cpNew.length < 8) {
			cp.setFeedback({ msg: "New password must be at least 8 characters", type: "error" });
			return;
		}
		if (cpNew !== cpConfirm) {
			cp.setFeedback({ msg: "Passwords do not match", type: "error" });
			return;
		}

		cp.run(async () => {
			await changePassword({ new_password: cpNew });
			setCpNew("");
			setCpConfirm("");
			return "Password changed";
		});
	};

	return (
		<section className="settings-section">
			<h3>Authentication</h3>
			<div className="settings-toggle-row">
				<span>Require password to access Kaji</span>
				<label className="toggle-switch">
					<input type="checkbox" checked={enabled} onChange={handleToggle} disabled={saving} />
					<span className="toggle-slider" />
				</label>
			</div>
			{showPasswordForm && (
				<form className="auth-password-form" onSubmit={handleSetPassword}>
					<p className="settings-description">Set a password to enable authentication.</p>
					<div className="settings-field">
						<label htmlFor="auth-pw">Password</label>
						<input
							id="auth-pw"
							type="password"
							autoComplete="new-password"
							value={newPw}
							onChange={(e) => setNewPw(e.target.value)}
							required
						/>
					</div>
					<div className="settings-field">
						<label htmlFor="auth-pw-confirm">Confirm Password</label>
						<input
							id="auth-pw-confirm"
							type="password"
							autoComplete="new-password"
							value={confirmPw}
							onChange={(e) => setConfirmPw(e.target.value)}
							required
						/>
					</div>
					<div className="auth-password-actions">
						<button type="submit" className="btn btn-primary settings-save-btn" disabled={saving}>
							{saving ? "Enabling..." : "Enable Authentication"}
						</button>
						<button
							type="button"
							className="btn btn-ghost settings-cancel-btn"
							onClick={handleCancel}
							disabled={saving}
						>
							Cancel
						</button>
					</div>
				</form>
			)}
			{error && <Feedback msg={error} type="error" className="settings-feedback" />}
			{enabled && (
				<form className="auth-password-form" onSubmit={handleChangePassword}>
					<h4 className="settings-subsection-title">Change Password</h4>
					<div className="settings-field">
						<label htmlFor="pw-new">New Password</label>
						<input
							id="pw-new"
							type="password"
							autoComplete="new-password"
							value={cpNew}
							onChange={(e) => setCpNew(e.target.value)}
							required
						/>
					</div>
					<div className="settings-field">
						<label htmlFor="pw-confirm">Confirm New Password</label>
						<input
							id="pw-confirm"
							type="password"
							autoComplete="new-password"
							value={cpConfirm}
							onChange={(e) => setCpConfirm(e.target.value)}
							required
						/>
					</div>
					<button type="submit" className="btn btn-primary settings-save-btn" disabled={cp.saving}>
						{cp.saving ? "Saving..." : "Change Password"}
					</button>
					<Feedback msg={cp.feedback.msg} type={cp.feedback.type} className="settings-feedback" />
				</form>
			)}
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

function GlobalTogglesSection({ initial }: { initial: GlobalToggles }) {
	const [toggles, setToggles] = useState<GlobalToggles>(initial);
	useEffect(() => setToggles(initial), [initial]);
	const { saving, feedback, run } = useAsyncAction();

	const set = <K extends keyof GlobalToggles>(key: K, value: GlobalToggles[K]) => {
		setToggles((prev) => ({ ...prev, [key]: value }));
	};

	const handleSave = () =>
		run(async () => {
			await updateGlobalToggles(toggles);
			return "Saved";
		});

	return (
		<section className="settings-section">
			<h3>Global Toggles</h3>
			<GlobalTogglesForm toggles={toggles} onChange={set} selectClassName="settings-field" />
			<button
				type="button"
				className="btn btn-primary settings-save-btn"
				disabled={saving}
				onClick={handleSave}
			>
				{saving ? "Saving..." : "Save"}
			</button>
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}

function ExportCaddyfileSection() {
	const { saving, feedback, run } = useAsyncAction();

	const handleExport = () =>
		run(async () => {
			const content = await exportCaddyfile();
			const blob = new Blob([content], { type: "text/plain" });
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
	const [globalToggles, setGlobalToggles] = useState<GlobalToggles>(DEFAULT_GLOBAL_TOGGLES);
	const [acmeEmail, setAcmeEmail] = useState("");
	const [authEnabled, setAuthEnabled] = useState(false);
	const [hasPassword, setHasPassword] = useState(false);
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

		Promise.allSettled([
			fetchGlobalToggles(),
			fetchACMEEmail(),
			fetchAuthStatus(),
			fetchAdvancedSettings(),
		])
			.then(([togglesResult, emailResult, authResult, advancedResult]) => {
				const failed = new Set<string>();

				if (togglesResult.status === "fulfilled") {
					setGlobalToggles(togglesResult.value as GlobalToggles);
				} else {
					failed.add("toggles");
				}
				if (emailResult.status === "fulfilled") {
					setAcmeEmail((emailResult.value as { email: string }).email);
				} else {
					failed.add("acme");
				}
				if (authResult.status === "fulfilled") {
					const auth = authResult.value as { auth_enabled: boolean; has_password: boolean };
					setAuthEnabled(auth.auth_enabled);
					setHasPassword(auth.has_password);
				} else {
					failed.add("auth");
				}
				if (advancedResult.status === "fulfilled") {
					setAdvanced(advancedResult.value as { caddy_admin_url: string });
				} else {
					failed.add("advanced");
				}

				if (failed.has("auth") && failed.has("toggles") && failed.has("acme")) {
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

			{failedSections.has("auth") ? (
				<section className="settings-section settings-section-failed">
					<h3>Authentication</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<>
					<AuthSection
						enabled={authEnabled}
						hasPassword={hasPassword}
						onChange={(enabled, nowHasPassword) => {
							setAuthEnabled(enabled);
							setHasPassword(nowHasPassword);
							onAuthChange(enabled);
						}}
					/>
					<APIKeySection />
				</>
			)}

			{!caddyRunning ? (
				<CaddyOffSection title="ACME Email" />
			) : failedSections.has("acme") ? (
				<section className="settings-section settings-section-failed">
					<h3>ACME Email</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<ACMEEmailSection initial={acmeEmail} />
			)}

			{!caddyRunning ? (
				<CaddyOffSection title="Global Toggles" />
			) : failedSections.has("toggles") ? (
				<section className="settings-section settings-section-failed">
					<h3>Global Toggles</h3>
					<p className="settings-section-error">Failed to load</p>
				</section>
			) : (
				<GlobalTogglesSection initial={globalToggles} />
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
