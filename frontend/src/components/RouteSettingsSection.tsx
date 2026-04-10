import { useEffect, useState } from "react";
import {
	fetchACMEEmail,
	fetchDNSProvider,
	fetchGlobalToggles,
	updateACMEEmail,
	updateDNSProvider,
	updateGlobalToggles,
} from "../api";
import { useSettingsSection } from "../hooks/useSettingsSection";
import type { GlobalToggles } from "../types/api";
import Feedback from "./Feedback";

export default function RouteSettingsSection() {
	const [globalToggles, setGlobalToggles] = useState<GlobalToggles | null>(null);
	const [dnsTokenTouched, setDnsTokenTouched] = useState(false);
	const {
		values,
		setValues,
		saved,
		dirty: fieldsDirty,
		loaded,
		load,
		save,
		saving,
		feedback,
	} = useSettingsSection({
		httpsValue: "on" as GlobalToggles["auto_https"],
		acmeEmail: "",
		dnsEnabled: false,
		dnsToken: "",
	});

	useEffect(() => {
		Promise.all([fetchGlobalToggles(), fetchACMEEmail(), fetchDNSProvider()]).then(
			([toggles, acmeResult, dnsResult]) => {
				setGlobalToggles(toggles);
				load({
					httpsValue: toggles.auto_https,
					acmeEmail: acmeResult.email,
					dnsEnabled: dnsResult.enabled,
					dnsToken: dnsResult.api_token ?? "",
				});
			},
		);
	}, [load]);

	if (!loaded || !globalToggles) return null;

	const httpsDirty = values.httpsValue !== saved.httpsValue;
	const acmeDirty = values.acmeEmail !== saved.acmeEmail;
	const dnsDirty =
		values.dnsEnabled !== saved.dnsEnabled || (dnsTokenTouched && values.dnsToken !== "");
	const dirty = fieldsDirty || dnsDirty;
	const httpsOn = values.httpsValue !== "off";

	const descriptions: Record<GlobalToggles["auto_https"], string> = {
		on: "Automatic certificates and HTTP-to-HTTPS redirects for all routes",
		disable_redirects: "Automatic certificates, but no HTTP-to-HTTPS redirects",
		off: "No automatic HTTPS",
	};

	const handleSave = () =>
		save(async (v) => {
			if (httpsDirty) {
				const updated = { ...globalToggles, auto_https: v.httpsValue };
				await updateGlobalToggles(updated);
				setGlobalToggles(updated);
			}
			if (acmeDirty) {
				await updateACMEEmail(v.acmeEmail);
			}
			if (dnsDirty) {
				await updateDNSProvider({
					enabled: v.dnsEnabled,
					api_token: dnsTokenTouched ? v.dnsToken : undefined,
				});
				setDnsTokenTouched(false);
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
					value={values.acmeEmail}
					onChange={(e) => setValues((v) => ({ ...v, acmeEmail: e.target.value }))}
					placeholder="you@example.com"
					maxLength={254}
					disabled={saving}
				/>
				<span className="settings-toggle-desc">
					Email for Let's Encrypt certificate notifications
				</span>
				{httpsOn && !values.acmeEmail && !acmeDirty && (
					<span className="settings-toggle-desc warning">
						No ACME email set - you won't receive certificate expiry warnings
					</span>
				)}
			</div>

			<div className="settings-field settings-field-spaced">
				<label htmlFor="global-https">Auto HTTPS</label>
				<select
					id="global-https"
					value={values.httpsValue}
					onChange={(e) =>
						setValues((v) => ({
							...v,
							httpsValue: e.target.value as GlobalToggles["auto_https"],
						}))
					}
					disabled={saving}
				>
					<option value="on">On</option>
					<option value="off">Off</option>
					<option value="disable_redirects">On without redirects</option>
				</select>
				<span className="settings-toggle-desc">{descriptions[values.httpsValue]}</span>
			</div>

			<div className="settings-field settings-field-spaced">
				<label className="toggle-label">
					<input
						type="checkbox"
						checked={values.dnsEnabled}
						onChange={(e) => setValues((v) => ({ ...v, dnsEnabled: e.target.checked }))}
						disabled={saving}
					/>
					Cloudflare DNS challenge
				</label>
				<span className="settings-toggle-desc">
					Use DNS-01 challenges for wildcard certs and domains where HTTP-01 isn't viable
				</span>
				{values.dnsEnabled && (
					<input
						type="password"
						value={dnsTokenTouched ? values.dnsToken : saved.dnsToken}
						onChange={(e) => {
							setValues((v) => ({ ...v, dnsToken: e.target.value }));
							setDnsTokenTouched(true);
						}}
						onFocus={() => {
							if (!dnsTokenTouched) {
								setValues((v) => ({ ...v, dnsToken: "" }));
								setDnsTokenTouched(true);
							}
						}}
						onBlur={() => {
							if (dnsTokenTouched && values.dnsToken === "") {
								setDnsTokenTouched(false);
							}
						}}
						placeholder="Cloudflare API token"
						disabled={saving}
						autoComplete="off"
						className="settings-field-nested"
					/>
				)}
				{values.dnsEnabled && !values.acmeEmail && !acmeDirty && (
					<span className="settings-toggle-desc warning">
						DNS challenge requires an ACME email - set one above
					</span>
				)}
			</div>
			{dirty && (
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving || (acmeDirty && !values.acmeEmail)}
					onClick={handleSave}
				>
					{saving ? "Saving..." : "Save"}
				</button>
			)}
			<Feedback msg={feedback.msg} type={feedback.type} />
		</section>
	);
}
