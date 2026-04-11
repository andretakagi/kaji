import { useCallback, useEffect, useRef, useState } from "react";
import {
	adaptCaddyfile,
	fetchDefaultCaddyfile,
	submitSetup,
	updateDNSProvider,
	updateSnapshotSettings,
} from "../api";
import {
	type AdaptCaddyfileResponse,
	DEFAULT_GLOBAL_TOGGLES,
	type GlobalToggles,
} from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validatePassword } from "../utils/validate";
import { Toggle } from "./Toggle";

const themeOptions = [
	{ value: "dark", label: "Dark" },
	{ value: "light", label: "Light" },
] as const;

const STEP_LABELS = ["Theme", "Auth", "Import", "HTTPS", "Metrics", "Snapshots"];
const LAST_STEP = STEP_LABELS.length - 1;

type ChallengeType = "http-01" | "cloudflare";

interface WizardData {
	authEnabled: boolean;
	password: string;
	confirmPassword: string;
	caddyfileText: string;
	adaptedConfig: Record<string, unknown> | null;
	importedSettings: AdaptCaddyfileResponse | null;
	acmeEmail: string;
	globalToggles: GlobalToggles;
	challengeType: ChallengeType;
	dnsToken: string;
	autoSnapshot: boolean;
	snapshotLimit: number;
}

function Setup({ onComplete }: { onComplete: () => void }) {
	const [step, setStep] = useState(0);
	const [error, setError] = useState("");
	const [setupWarnings, setSetupWarnings] = useState<string[]>([]);
	const [submitting, setSubmitting] = useState(false);
	const [data, setData] = useState<WizardData>({
		authEnabled: true,
		password: "",
		confirmPassword: "",
		caddyfileText: "",
		adaptedConfig: null,
		importedSettings: null,
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

	const validateStep = (): boolean => {
		setError("");
		if (step === 1 && data.authEnabled) {
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
		try {
			const res = await submitSetup({
				password: data.authEnabled ? data.password : undefined,
				acme_email: data.acmeEmail || undefined,
				global_toggles: data.globalToggles,
				caddyfile_json: data.adaptedConfig || undefined,
			});

			const warnings = [...(res.warnings || [])];

			try {
				if (data.challengeType === "cloudflare" && data.dnsToken) {
					await updateDNSProvider({ enabled: true, api_token: data.dnsToken });
				}
			} catch {
				warnings.push("Failed to configure DNS challenge provider");
			}

			try {
				await updateSnapshotSettings({
					auto_snapshot_enabled: data.autoSnapshot,
					auto_snapshot_limit: data.snapshotLimit,
				});
			} catch {
				warnings.push("Failed to configure snapshot settings");
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
			setError(msg);
		} finally {
			setSubmitting(false);
		}
	};

	const stepContent = [
		<StepTheme key="theme" />,
		<StepAuth key="auth" data={data} update={update} />,
		<StepImport key="import" data={data} update={update} error={error} setError={setError} />,
		<StepHTTPS key="https" data={data} update={update} />,
		<StepMetrics key="metrics" data={data} update={update} />,
		<StepSnapshots key="snapshots" data={data} update={update} />,
	];

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
			<div className="auth-card">
				<h1>Kaji</h1>
				<StepIndicator current={step} />

				{error && step !== 2 && (
					<div className="inline-error auth-error" role="alert">
						{error}
					</div>
				)}

				<div className="setup-step-content">{stepContent[step]}</div>

				<div className="setup-nav">
					{step > 0 && (
						<button type="button" className="btn btn-ghost" onClick={handleBack}>
							Back
						</button>
					)}
					{step === 0 && <span />}
					{step < LAST_STEP ? (
						<button type="button" className="btn btn-primary" onClick={handleNext}>
							Next
						</button>
					) : (
						<button
							type="button"
							className="btn btn-primary auth-submit"
							disabled={submitting}
							onClick={handleSubmit}
						>
							{submitting ? "Setting up..." : "Complete Setup"}
						</button>
					)}
				</div>
			</div>
		</div>
	);
}

function StepIndicator({ current }: { current: number }) {
	return (
		<div className="setup-steps">
			{STEP_LABELS.map((label, i) => {
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
		<>
			<p className="setup-step-description">Choose your preferred theme.</p>
			<div className="auth-field">
				<span className="settings-label">Theme</span>
				<Toggle
					options={themeOptions}
					value={theme}
					onChange={(v: "dark" | "light") => applyTheme(v)}
					aria-label="Theme"
				/>
			</div>
		</>
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
	const [loadedDefault, setLoadedDefault] = useState(false);
	const fileInputRef = useRef<HTMLInputElement>(null);

	useEffect(() => {
		if (loadedDefault || data.caddyfileText) return;
		setLoadedDefault(true);
		fetchDefaultCaddyfile()
			.then((res) => {
				if (res.content) {
					update("caddyfileText", res.content);
				}
			})
			.catch(() => {});
	}, [loadedDefault, data.caddyfileText, update]);

	const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
		const file = e.target.files?.[0];
		if (!file) return;
		const reader = new FileReader();
		reader.onload = () => {
			update("caddyfileText", reader.result as string);
			update("adaptedConfig", null);
			update("importedSettings", null);
		};
		reader.onerror = () => {
			setError("Failed to read file.");
		};
		reader.readAsText(file);
	};

	const handleParse = async () => {
		if (!data.caddyfileText.trim()) return;
		setParsing(true);
		setError("");
		try {
			const result = await adaptCaddyfile(data.caddyfileText);
			update("adaptedConfig", result.adapted_config);
			update("importedSettings", result);
			if (result.acme_email) {
				update("acmeEmail", result.acme_email);
			}
			update("globalToggles", result.global_toggles);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to parse Caddyfile"));
			update("adaptedConfig", null);
			update("importedSettings", null);
		} finally {
			setParsing(false);
		}
	};

	const handleTextChange = (text: string) => {
		update("caddyfileText", text);
		if (data.importedSettings) {
			update("adaptedConfig", null);
			update("importedSettings", null);
		}
	};

	return (
		<>
			<p className="setup-step-description">
				Import an existing Caddyfile to bring in your routes and settings. This step is optional.
			</p>

			<div className="auth-field">
				<label htmlFor="setup-caddyfile">Caddyfile</label>
				<textarea
					id="setup-caddyfile"
					className="setup-import-textarea"
					value={data.caddyfileText}
					onChange={(e) => handleTextChange(e.target.value)}
					placeholder={"example.com {\n    reverse_proxy localhost:3000\n}"}
					spellCheck={false}
				/>
			</div>

			{error && (
				<div className="inline-error auth-error" role="alert">
					{error}
				</div>
			)}

			<div className="setup-import-actions">
				<input
					ref={fileInputRef}
					type="file"
					accept=".caddyfile,.Caddyfile,text/*"
					onChange={handleFileUpload}
					hidden
				/>
				<button
					type="button"
					className="btn btn-ghost"
					onClick={() => fileInputRef.current?.click()}
				>
					Upload File
				</button>
				<button
					type="button"
					className="btn btn-primary"
					onClick={handleParse}
					disabled={parsing || !data.caddyfileText.trim()}
				>
					{parsing ? "Parsing..." : "Parse Caddyfile"}
				</button>
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
	);
}

const challengeOptions = [
	{ value: "http-01", label: "HTTP-01" },
	{ value: "cloudflare", label: "Cloudflare DNS" },
] as const;

function StepHTTPS({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<>
			<p className="setup-step-description">Configure HTTPS certificate settings.</p>
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
					<input
						type="password"
						value={data.dnsToken}
						onChange={(e) => update("dnsToken", e.target.value)}
						placeholder="Cloudflare API token"
						autoComplete="off"
					/>
				)}
			</div>
		</>
	);
}

function StepMetrics({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<>
			<p className="setup-step-description">Configure Prometheus metrics collection.</p>
			<div className="setup-toggle-row">
				<div>
					<span>Prometheus metrics</span>
					<div className="field-hint">Expose a /metrics endpoint for Prometheus to scrape.</div>
				</div>
				<Toggle
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
				<div className="setup-toggle-row">
					<div>
						<span>Per-host metrics</span>
						<div className="field-hint">
							Break down metrics by hostname. Increases cardinality with many hosts.
						</div>
					</div>
					<Toggle
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
		</>
	);
}

function StepSnapshots({
	data,
	update,
}: {
	data: WizardData;
	update: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
}) {
	return (
		<>
			<p className="setup-step-description">
				Snapshots save a copy of your Caddy config before each change, so you can roll back if
				something goes wrong.
			</p>
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

export default Setup;
