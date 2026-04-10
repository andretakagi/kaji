import { updateACMEEmail, updateDNSProvider, updateGlobalToggles } from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import type { GlobalToggles } from "../types/api";
import Feedback from "./Feedback";

export interface RouteSettingsProps {
	globalToggles: GlobalToggles;
	httpsValue: GlobalToggles["auto_https"];
	setHttpsValue: (v: GlobalToggles["auto_https"]) => void;
	acmeEmail: string;
	setAcmeEmail: (v: string) => void;
	initialAcmeEmail: string;
	onTogglesSaved: (toggles: GlobalToggles) => void;
	onAcmeSaved: () => void;
	dnsEnabled: boolean;
	setDnsEnabled: (v: boolean) => void;
	dnsToken: string;
	setDnsToken: (v: string) => void;
	initialDnsEnabled: boolean;
	initialDnsToken: string;
	dnsTokenTouched: boolean;
	setDnsTokenTouched: (v: boolean) => void;
	onDnsSaved: () => void;
}

export default function RouteSettingsSection({
	globalToggles,
	httpsValue,
	setHttpsValue,
	acmeEmail,
	setAcmeEmail,
	initialAcmeEmail,
	onTogglesSaved,
	onAcmeSaved,
	dnsEnabled,
	setDnsEnabled,
	dnsToken,
	setDnsToken,
	initialDnsEnabled,
	initialDnsToken,
	dnsTokenTouched,
	setDnsTokenTouched,
	onDnsSaved,
}: RouteSettingsProps) {
	const { saving, feedback, run } = useAsyncAction();

	const httpsDirty = httpsValue !== globalToggles.auto_https;
	const acmeDirty = acmeEmail !== initialAcmeEmail;
	const dnsDirty = dnsEnabled !== initialDnsEnabled || (dnsTokenTouched && dnsToken !== "");
	const dirty = httpsDirty || acmeDirty || dnsDirty;
	const httpsOn = httpsValue !== "off";

	const descriptions: Record<GlobalToggles["auto_https"], string> = {
		on: "Automatic certificates and HTTP-to-HTTPS redirects for all routes",
		disable_redirects: "Automatic certificates, but no HTTP-to-HTTPS redirects",
		off: "No automatic HTTPS",
	};

	const handleSave = () =>
		run(async () => {
			if (httpsDirty) {
				const updated = { ...globalToggles, auto_https: httpsValue };
				await updateGlobalToggles(updated);
				onTogglesSaved(updated);
			}
			if (acmeDirty) {
				await updateACMEEmail(acmeEmail);
				onAcmeSaved();
			}
			if (dnsDirty) {
				await updateDNSProvider({
					enabled: dnsEnabled,
					api_token: dnsTokenTouched ? dnsToken : undefined,
				});
				onDnsSaved();
			}
			return "Saved";
		});

	return (
		<section className="settings-section">
			<h3>Route Settings</h3>
			<div className="settings-field">
				<label htmlFor="acme-email">ACME email</label>
				<input
					id="acme-email"
					type="email"
					value={acmeEmail}
					onChange={(e) => setAcmeEmail(e.target.value)}
					placeholder="you@example.com"
					disabled={saving}
				/>
				<span className="settings-toggle-desc">
					Email for Let's Encrypt certificate notifications
				</span>
				{httpsOn && !acmeEmail && !acmeDirty && (
					<span className="settings-toggle-desc" style={{ color: "var(--yellow)" }}>
						No ACME email set - you won't receive certificate expiry warnings
					</span>
				)}
			</div>

			<div className="settings-field" style={{ marginTop: "0.75rem" }}>
				<label htmlFor="global-https">Auto HTTPS</label>
				<select
					id="global-https"
					value={httpsValue}
					onChange={(e) => setHttpsValue(e.target.value as GlobalToggles["auto_https"])}
					disabled={saving}
				>
					<option value="on">On</option>
					<option value="off">Off</option>
					<option value="disable_redirects">On without redirects</option>
				</select>
				<span className="settings-toggle-desc">{descriptions[httpsValue]}</span>
			</div>

			<div className="settings-field" style={{ marginTop: "0.75rem" }}>
				<label className="toggle-label">
					<input
						type="checkbox"
						checked={dnsEnabled}
						onChange={(e) => setDnsEnabled(e.target.checked)}
						disabled={saving}
					/>
					Cloudflare DNS challenge
				</label>
				<span className="settings-toggle-desc">
					Use DNS-01 challenges for wildcard certs and domains where HTTP-01 isn't viable
				</span>
				{dnsEnabled && (
					<input
						type="password"
						value={dnsTokenTouched ? dnsToken : initialDnsToken}
						onChange={(e) => {
							setDnsToken(e.target.value);
							setDnsTokenTouched(true);
						}}
						onFocus={() => {
							if (!dnsTokenTouched) {
								setDnsToken("");
								setDnsTokenTouched(true);
							}
						}}
						onBlur={() => {
							if (dnsTokenTouched && dnsToken === "") {
								setDnsTokenTouched(false);
							}
						}}
						placeholder="Cloudflare API token"
						disabled={saving}
						autoComplete="off"
						style={{ marginTop: "0.5rem" }}
					/>
				)}
				{dnsEnabled && !acmeEmail && !acmeDirty && (
					<span className="settings-toggle-desc" style={{ color: "var(--yellow)" }}>
						DNS challenge requires an ACME email - set one above
					</span>
				)}
			</div>
			{dirty && (
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving || (acmeDirty && !acmeEmail)}
					onClick={handleSave}
				>
					{saving ? "Saving..." : "Save"}
				</button>
			)}
			<Feedback msg={feedback.msg} type={feedback.type} />
		</section>
	);
}
