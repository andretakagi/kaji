import { useCallback, useEffect, useState } from "react";
import { adaptCaddyfile, setupImportFull, submitSetup, updateDNSProvider } from "../api";
import { cn } from "../cn";
import {
	type AdaptCaddyfileResponse,
	DEFAULT_GLOBAL_TOGGLES,
	type GlobalToggles,
	type ImportReview,
	type SetupImportFullResponse,
	type SetupStatus,
} from "../types/api";
import type { CaddyConfig } from "../types/caddy";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validatePassword } from "../utils/validate";
import FileUploadButton from "./FileUploadButton";
import { Toggle } from "./Toggle";

const themeOptions = [
	{ value: "dark", label: "Dark" },
	{ value: "light", label: "Light" },
] as const;

type ChallengeType = "http-01" | "cloudflare";

interface WizardData {
	authEnabled: boolean;
	password: string;
	confirmPassword: string;
	importMode: "none" | "caddyfile" | "full";
	caddyfileText: string;
	adaptedConfig: CaddyConfig | null;
	importedSettings: AdaptCaddyfileResponse | null;
	backupSummary: SetupImportFullResponse | null;
	backupData: Record<string, unknown> | null;
	reviewData: ImportReview | null;
	acmeEmail: string;
	globalToggles: GlobalToggles;
	challengeType: ChallengeType;
	dnsToken: string;
	autoSnapshot: boolean;
	snapshotLimit: number;
}

function Setup({
	onComplete,
	fetchSetupStatus,
}: {
	onComplete: () => void;
	fetchSetupStatus: () => Promise<SetupStatus>;
}) {
	const [caddyReady, setCaddyReady] = useState<boolean | null>(null);
	const [step, setStep] = useState(0);
	const [error, setError] = useState("");
	const [dnsError, setDnsError] = useState("");
	const [setupWarnings, setSetupWarnings] = useState<string[]>([]);
	const [submitting, setSubmitting] = useState(false);
	const [setupDone, setSetupDone] = useState(false);
	const [data, setData] = useState<WizardData>({
		authEnabled: true,
		password: "",
		confirmPassword: "",
		importMode: "none",
		caddyfileText: "",
		adaptedConfig: null,
		importedSettings: null,
		backupSummary: null,
		backupData: null,
		reviewData: null,
		acmeEmail: "",
		globalToggles: { ...DEFAULT_GLOBAL_TOGGLES },
		challengeType: "http-01",
		dnsToken: "",
		autoSnapshot: false,
		snapshotLimit: 10,
	});

	const update = useCallback(<K extends keyof WizardData>(key: K, value: WizardData[K]) => {
		setData((prev) => ({ ...prev, [key]: value }));
	}, []);

	const hasReview = data.importMode !== "none" && data.reviewData !== null;
	const stepLabels = hasReview
		? ["Auth", "Import", "Review", "Settings"]
		: ["Auth", "Import", "Settings"];
	const lastStep = stepLabels.length - 1;

	useEffect(() => {
		let active = true;
		const check = () => {
			fetchSetupStatus()
				.then((res) => {
					if (active) setCaddyReady(res.caddy_running);
				})
				.catch(() => {
					if (active) setCaddyReady(false);
				});
		};
		check();
		const id = setInterval(check, 3000);
		return () => {
			active = false;
			clearInterval(id);
		};
	}, [fetchSetupStatus]);

	const validateStep = (): boolean => {
		setError("");
		if (step === 0 && data.authEnabled) {
			const pwErr = validatePassword(data.password, data.confirmPassword);
			if (pwErr) {
				setError(pwErr);
				return false;
			}
		}
		return true;
	};

	const handleNext = () => {
		if (!validateStep()) return;
		setError("");
		setStep((s) => s + 1);
	};

	const handleBack = () => {
		setError("");
		setStep((s) => s - 1);
	};

	const handleSubmit = async () => {
		if (!validateStep()) return;
		setSubmitting(true);
		setError("");
		setDnsError("");
		try {
			if (setupDone) {
				if (data.challengeType === "cloudflare" && data.dnsToken) {
					await updateDNSProvider({ enabled: true, api_token: data.dnsToken });
				}
				onComplete();
				return;
			}

			const adminListen = data.importedSettings?.admin_listen;
			const res = await submitSetup({
				password: data.authEnabled ? data.password : undefined,
				caddy_admin_url: adminListen ? `http://${adminListen}` : undefined,
				acme_email: data.acmeEmail || undefined,
				global_toggles: data.globalToggles,
				caddyfile_json:
					data.importMode === "caddyfile" ? data.adaptedConfig || undefined : undefined,
				dns_challenge_token:
					data.challengeType === "cloudflare" && data.dnsToken ? data.dnsToken : undefined,
				auto_snapshot_enabled: data.autoSnapshot,
				auto_snapshot_limit: data.snapshotLimit,
				backup_data: data.importMode === "full" ? data.backupData || undefined : undefined,
			});

			setSetupDone(true);
			const warnings = [...(res.warnings || [])];

			if (res.dns_error) {
				setDnsError(res.dns_error);
				return;
			}

			if (warnings.length) {
				setSetupWarnings(warnings);
				return;
			}

			onComplete();
		} catch (err) {
			const msg = getErrorMessage(err, "Setup failed.");
			if (msg === "setup already completed") {
				onComplete();
				return;
			}
			if (setupDone) {
				setDnsError(getErrorMessage(err, "Could not configure DNS challenge"));
				return;
			}
			setError(msg);
		} finally {
			setSubmitting(false);
		}
	};

	const stepContent = hasReview
		? [
				<StepAuth key="auth" data={data} update={update} />,
				<StepImport key="import" data={data} update={update} error={error} setError={setError} />,
				<StepReview key="review" data={data} />,
				<StepSettings key="settings" data={data} update={update} dnsError={dnsError} />,
			]
		: [
				<StepAuth key="auth" data={data} update={update} />,
				<StepImport key="import" data={data} update={update} error={error} setError={setError} />,
				<StepSettings key="settings" data={data} update={update} dnsError={dnsError} />,
			];

	if (!caddyReady) {
		return (
			<div className="auth-wrapper">
				<div className="auth-card auth-card-narrow setup-caddy-gate">
					<h1>Kaji</h1>
					<div className="setup-caddy-gate-status">
						<span className="status-beacon stopped" />
						<span>Waiting for Caddy</span>
					</div>
					<p>
						Caddy must be running before setup can continue.
						{caddyReady === false && " Retrying every few seconds..."}
					</p>
				</div>
			</div>
		);
	}

	if (setupWarnings.length > 0) {
		return (
			<div className="auth-wrapper">
				<div className="auth-card">
					<h1>Kaji</h1>
					<p>Setup completed with warnings:</p>
					<ul className="setup-warnings">
						{setupWarnings.map((w) => (
							<li key={w}>{w}</li>
						))}
					</ul>
					<p>These can be configured later from the settings page.</p>
					<button type="button" className="btn btn-primary" onClick={onComplete}>
						Continue
					</button>
				</div>
			</div>
		);
	}

	return (
		<div className="auth-wrapper">
			<div
				className={`auth-card${step === lastStep || (hasReview && step === 2) ? " auth-card-wide" : ""}`}
			>
				<h1>Kaji</h1>
				<StepIndicator current={step} labels={stepLabels} />

				{error && step !== 1 && (
					<div className="inline-error auth-error" role="alert">
						{error}
					</div>
				)}

				<div className="setup-step-content">{stepContent[step]}</div>

				<div className="setup-nav">
					{step > 0 && !setupDone && (
						<button type="button" className="btn btn-ghost" onClick={handleBack}>
							Back
						</button>
					)}
					{setupDone && (
						<button type="button" className="btn btn-ghost" onClick={onComplete}>
							Skip
						</button>
					)}
					{step === 0 && !setupDone && <span />}
					{step < lastStep ? (
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleNext}
							disabled={
								step === 1 &&
								((data.importMode === "caddyfile" && !data.importedSettings) ||
									(data.importMode === "full" && !data.backupSummary))
							}
						>
							Next
						</button>
					) : (
						<button
							type="button"
							className="btn btn-primary auth-submit"
							disabled={submitting}
							onClick={handleSubmit}
						>
							{submitting ? "Setting up..." : setupDone ? "Retry" : "Complete Setup"}
						</button>
					)}
				</div>
			</div>
		</div>
	);
}

function StepIndicator({ current, labels }: { current: number; labels: string[] }) {
	return (
		<div className="setup-steps">
			{labels.map((label, i) => {
				let cls = "setup-step";
				if (i === current) cls += " active";
				else if (i < current) cls += " completed";
				return (
					<div key={label} className={cls}>
						{i > 0 && <div className="setup-step-connector" />}
						<div className="setup-step-number">
							{i < current ? (
								<svg
									width="12"
									height="12"
									viewBox="0 0 12 12"
									fill="none"
									role="img"
									aria-label="Completed"
								>
									<path
										d="M2 6L5 9L10 3"
										stroke="currentColor"
										strokeWidth="2"
										strokeLinecap="round"
										strokeLinejoin="round"
									/>
								</svg>
							) : (
								i + 1
							)}
						</div>
						<div className="setup-step-label">{label}</div>
					</div>
				);
			})}
		</div>
	);
}

function StepTheme() {
	const [theme, setTheme] = useState<"dark" | "light">(
		() => (localStorage.getItem("theme") as "dark" | "light") || "dark",
	);

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
		<Toggle
			options={themeOptions}
			value={theme}
			onChange={(v: "dark" | "light") => applyTheme(v)}
			aria-label="Theme"
		/>
	);
}

function StepAuth({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<>
			<p className="setup-step-description">Set up admin authentication.</p>
			<div className="setup-auth-toggle">
				<div className="setup-toggle-row">
					<span>Require password</span>
					<Toggle
						value={data.authEnabled}
						onChange={() => update("authEnabled", !data.authEnabled)}
					/>
				</div>
				{!data.authEnabled && (
					<div className="field-hint">You can enable authentication later from Settings.</div>
				)}
			</div>

			{data.authEnabled && (
				<>
					<div className="auth-field">
						<label htmlFor="setup-password">Admin Password</label>
						<input
							id="setup-password"
							type="password"
							autoComplete="new-password"
							minLength={8}
							maxLength={512}
							value={data.password}
							onChange={(e) => update("password", e.target.value)}
							placeholder="Choose a password"
						/>
					</div>
					<div className="auth-field">
						<label htmlFor="setup-confirm">Confirm Password</label>
						<input
							id="setup-confirm"
							type="password"
							autoComplete="new-password"
							minLength={8}
							maxLength={512}
							value={data.confirmPassword}
							onChange={(e) => update("confirmPassword", e.target.value)}
							placeholder="Repeat your password"
						/>
					</div>
				</>
			)}
		</>
	);
}

function StepImport({
	data,
	update,
	error,
	setError,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
	error: string;
	setError: (msg: string) => void;
}) {
	const [parsing, setParsing] = useState(false);

	const handleModeChange = (mode: "none" | "caddyfile" | "full") => {
		update("importMode", mode);
		setError("");
		update("reviewData", null);
		if (mode !== "caddyfile") {
			update("adaptedConfig", null);
			update("importedSettings", null);
		}
		if (mode !== "full") {
			update("backupSummary", null);
			update("backupData", null);
		}
	};

	const handleCaddyfileUpload = async (file: File) => {
		setParsing(true);
		setError("");
		try {
			const text = await file.text();
			const result = await adaptCaddyfile(text);
			update("caddyfileText", text);
			update("adaptedConfig", result.adapted_config);
			update("importedSettings", result);
			update("reviewData", { routes: result.routes ?? [] });
			if (result.acme_email) update("acmeEmail", result.acme_email);
			update("globalToggles", result.global_toggles);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to parse Caddyfile"));
			update("adaptedConfig", null);
			update("importedSettings", null);
		} finally {
			setParsing(false);
		}
	};

	const handleBackupUpload = async (file: File) => {
		setParsing(true);
		setError("");
		try {
			const result = await setupImportFull(file);
			update("backupSummary", result);
			update("backupData", result.backup_data);
			if (result.review) {
				update("reviewData", result.review);
			}
			if (result.acme_email) update("acmeEmail", result.acme_email);
			if (result.global_toggles) update("globalToggles", result.global_toggles);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to parse backup file"));
			update("backupSummary", null);
			update("backupData", null);
		} finally {
			setParsing(false);
		}
	};

	return (
		<>
			<p className="setup-step-description">Import an existing configuration or start fresh.</p>

			<div className="setup-import-modes">
				<button
					type="button"
					className={cn("setup-import-mode-btn", data.importMode === "none" && "active")}
					onClick={() => handleModeChange("none")}
				>
					<strong>Start Fresh</strong>
					<span>Begin with a clean configuration</span>
				</button>
				<button
					type="button"
					className={cn("setup-import-mode-btn", data.importMode === "caddyfile" && "active")}
					onClick={() => handleModeChange("caddyfile")}
				>
					<strong>Import Caddyfile</strong>
					<span>Bring in routes from an existing Caddyfile</span>
				</button>
				<button
					type="button"
					className={cn("setup-import-mode-btn", data.importMode === "full" && "active")}
					onClick={() => handleModeChange("full")}
				>
					<strong>Import Full Backup</strong>
					<span>Restore a complete Kaji backup</span>
				</button>
			</div>

			{error && (
				<div className="inline-error auth-error" role="alert">
					{error}
				</div>
			)}

			{data.importMode === "caddyfile" && (
				<>
					<div className="setup-import-actions">
						<FileUploadButton disabled={parsing} onChange={handleCaddyfileUpload}>
							{parsing ? "Parsing..." : "Choose Caddyfile"}
						</FileUploadButton>
					</div>
					{data.importedSettings && (
						<div className="setup-import-result">
							<div className="setup-import-result-item">
								<span>Routes found</span>
								<strong>{data.importedSettings.route_count}</strong>
							</div>
							{data.importedSettings.acme_email && (
								<div className="setup-import-result-item">
									<span>ACME email</span>
									<strong>{data.importedSettings.acme_email}</strong>
								</div>
							)}
							<div className="setup-import-result-item">
								<span>Auto HTTPS</span>
								<strong>{data.importedSettings.global_toggles.auto_https}</strong>
							</div>
						</div>
					)}
				</>
			)}

			{data.importMode === "full" && (
				<>
					<div className="setup-import-actions">
						<FileUploadButton accept=".zip" disabled={parsing} onChange={handleBackupUpload}>
							{parsing ? "Reading backup..." : "Choose Backup File"}
						</FileUploadButton>
					</div>
					{data.backupSummary && (
						<div className="setup-import-result">
							<div className="setup-import-result-item">
								<span>Caddy config</span>
								<strong>Included</strong>
							</div>
							<div className="setup-import-result-item">
								<span>IP lists</span>
								<strong>{data.backupSummary.summary.ip_lists}</strong>
							</div>
							<div className="setup-import-result-item">
								<span>Disabled routes</span>
								<strong>{data.backupSummary.summary.disabled_routes}</strong>
							</div>
							<div className="setup-import-result-item">
								<span>Snapshots</span>
								<strong>{data.backupSummary.summary.snapshot_count}</strong>
							</div>
							{data.backupSummary.summary.loki_enabled && (
								<div className="setup-import-result-item">
									<span>Loki</span>
									<strong>Configured</strong>
								</div>
							)}
							{data.backupSummary.migration_log && data.backupSummary.migration_log.length > 0 && (
								<div className="setup-import-migration">
									<span className="setup-import-migration-label">
										Migrated from {data.backupSummary.migrated_from}
									</span>
									<ul className="setup-import-migration-list">
										{data.backupSummary.migration_log.map((change) => (
											<li key={change}>{change}</li>
										))}
									</ul>
								</div>
							)}
						</div>
					)}
				</>
			)}
		</>
	);
}

function StepReview({ data }: { data: WizardData }) {
	const review = data.reviewData;
	if (!review) return null;

	const isFull = data.importMode === "full";

	return (
		<div className="setup-review">
			{review.routes.length > 0 && (
				<div className="setup-review-section">
					<h3 className="setup-review-heading">Routes</h3>
					<div className="setup-review-table">
						<div className="setup-review-table-header">
							<span>Domain</span>
							<span>Upstream</span>
							<span>Status</span>
						</div>
						{review.routes.map((route) => (
							<div
								key={`${route.domain}-${route.enabled}`}
								className={cn("setup-review-table-row", !route.enabled && "disabled")}
							>
								<span className="setup-review-domain">{route.domain}</span>
								<span className="setup-review-upstream">{route.upstream}</span>
								<span className={cn("setup-review-status", route.enabled ? "on" : "off")}>
									{route.enabled ? "Enabled" : "Disabled"}
								</span>
							</div>
						))}
					</div>
				</div>
			)}

			{isFull && review.logging && (
				<div className="setup-review-section">
					<h3 className="setup-review-heading">Logging</h3>
					<div className="setup-review-list">
						{review.logging.log_file && (
							<div className="setup-review-list-item">
								<span>Log file</span>
								<span className="setup-review-value">{review.logging.log_file}</span>
							</div>
						)}
						{review.logging.log_dir && (
							<div className="setup-review-list-item">
								<span>Log directory</span>
								<span className="setup-review-value">{review.logging.log_dir}</span>
							</div>
						)}
						<div className="setup-review-list-item">
							<span>Loki</span>
							<span className="setup-review-value">
								{review.logging.loki_enabled ? review.logging.loki_endpoint : "Not configured"}
							</span>
						</div>
					</div>
				</div>
			)}

			{isFull && review.ip_lists && review.ip_lists.length > 0 && (
				<div className="setup-review-section">
					<h3 className="setup-review-heading">IP Lists</h3>
					<div className="setup-review-table">
						<div className="setup-review-table-header">
							<span>Name</span>
							<span>Type</span>
							<span>Entries</span>
						</div>
						{review.ip_lists.map((list) => (
							<div key={list.name} className="setup-review-table-row">
								<span>{list.name}</span>
								<span className="setup-review-badge">{list.type}</span>
								<span>{list.entry_count}</span>
							</div>
						))}
					</div>
				</div>
			)}

			{isFull && review.snapshots && review.snapshots.length > 0 && (
				<div className="setup-review-section">
					<h3 className="setup-review-heading">Snapshots</h3>
					<div className="setup-review-table">
						<div className="setup-review-table-header">
							<span>Name</span>
							<span>Type</span>
							<span>Date</span>
						</div>
						{review.snapshots.map((snap) => (
							<div key={snap.name + snap.created_at} className="setup-review-table-row">
								<span>{snap.name}</span>
								<span className="setup-review-badge">{snap.type}</span>
								<span>
									{new Date(snap.created_at).toLocaleDateString(undefined, {
										year: "numeric",
										month: "short",
										day: "numeric",
									})}
								</span>
							</div>
						))}
					</div>
				</div>
			)}
		</div>
	);
}

const challengeOptions = [
	{ value: "http-01", label: "HTTP-01" },
	{ value: "cloudflare", label: "Cloudflare DNS" },
] as const;

function StepHTTPSContent({
	data,
	update,
	dnsError,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
	dnsError: string;
}) {
	return (
		<>
			<div className="auth-field">
				<label htmlFor="setup-auto-https">Auto HTTPS</label>
				<select
					id="setup-auto-https"
					value={data.globalToggles.auto_https}
					onChange={(e) =>
						update("globalToggles", {
							...data.globalToggles,
							auto_https: e.target.value as GlobalToggles["auto_https"],
						})
					}
				>
					<option value="on">On</option>
					<option value="off">Off</option>
					<option value="disable_redirects">On without redirects</option>
				</select>
			</div>
			<div className="auth-field">
				<label htmlFor="setup-acme-email">ACME Email</label>
				<input
					id="setup-acme-email"
					type="email"
					value={data.acmeEmail}
					onChange={(e) => update("acmeEmail", e.target.value)}
					placeholder="you@example.com"
					maxLength={254}
				/>
				<div className="field-hint">
					Used by Let's Encrypt for certificate expiry notices and account recovery.
				</div>
			</div>
			<div className="auth-field">
				<span className="settings-label" id="setup-challenge-label">
					Challenge type
				</span>
				<Toggle
					options={challengeOptions}
					value={data.challengeType}
					onChange={(v: ChallengeType) => update("challengeType", v)}
					aria-label="Challenge type"
				/>
				{data.challengeType === "cloudflare" && (
					<>
						<input
							type="password"
							className={dnsError ? "input-error" : ""}
							style={{ marginTop: "0.5rem" }}
							value={data.dnsToken}
							onChange={(e) => update("dnsToken", e.target.value)}
							placeholder="Cloudflare API token"
							autoComplete="off"
						/>
						{dnsError && (
							<div className="inline-error" role="alert">
								{dnsError}
							</div>
						)}
					</>
				)}
			</div>
		</>
	);
}

function StepMetricsContent({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<div className="settings-toggle-grid setup-toggle-grid-stacked">
			<div className="settings-toggle-item">
				<div className="settings-toggle-label">
					<span>Prometheus metrics</span>
					<span className="settings-toggle-desc">
						Expose a /metrics endpoint for Prometheus to scrape.
					</span>
				</div>
				<Toggle
					inline
					small
					value={data.globalToggles.prometheus_metrics}
					onChange={() =>
						update("globalToggles", {
							...data.globalToggles,
							prometheus_metrics: !data.globalToggles.prometheus_metrics,
							per_host_metrics: !data.globalToggles.prometheus_metrics
								? data.globalToggles.per_host_metrics
								: false,
						})
					}
				/>
			</div>
			{data.globalToggles.prometheus_metrics && (
				<div className="settings-toggle-item">
					<div className="settings-toggle-label">
						<span>Per-host metrics</span>
						<span className="settings-toggle-desc">
							Break down metrics by hostname. Increases cardinality with many hosts.
						</span>
					</div>
					<Toggle
						inline
						small
						value={data.globalToggles.per_host_metrics}
						onChange={() =>
							update("globalToggles", {
								...data.globalToggles,
								per_host_metrics: !data.globalToggles.per_host_metrics,
							})
						}
					/>
				</div>
			)}
		</div>
	);
}

function StepSnapshotsContent({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<>
			<div className="setup-toggle-row">
				<span>Auto snapshots</span>
				<Toggle
					value={data.autoSnapshot}
					onChange={() => update("autoSnapshot", !data.autoSnapshot)}
				/>
			</div>
			{data.autoSnapshot && (
				<div className="snapshot-settings-limit">
					<span>Keep last</span>
					<input
						type="number"
						min={1}
						max={100}
						value={data.snapshotLimit}
						onChange={(e) =>
							update(
								"snapshotLimit",
								Math.min(100, Math.max(1, Number.parseInt(e.target.value, 10) || 1)),
							)
						}
						className="snapshot-limit-input"
					/>
					<span>auto snapshots</span>
				</div>
			)}
		</>
	);
}

function StepSettings({
	data,
	update,
	dnsError,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
	dnsError: string;
}) {
	return (
		<>
			<p className="setup-step-description">
				Configure your preferences. All of these can be changed later in Settings.
			</p>

			<div className="setup-settings-grid">
				<div className="setup-settings-section">
					<h3 className="setup-settings-heading">HTTPS</h3>
					<StepHTTPSContent data={data} update={update} dnsError={dnsError} />
				</div>

				<div className="setup-settings-section">
					<h3 className="setup-settings-heading">Metrics</h3>
					<StepMetricsContent data={data} update={update} />
				</div>

				<div className="setup-settings-section">
					<h3 className="setup-settings-heading">Snapshots</h3>
					<StepSnapshotsContent data={data} update={update} />
				</div>

				<div className="setup-settings-section">
					<h3 className="setup-settings-heading">Theme</h3>
					<StepTheme />
				</div>
			</div>
		</>
	);
}

export default Setup;
