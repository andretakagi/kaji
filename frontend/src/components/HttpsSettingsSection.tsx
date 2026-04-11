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
import { Toggle } from "./Toggle";

type ChallengeType = "http-01" | "cloudflare";

const httpsOptions = [
	{ value: "on", label: "On" },
	{ value: "disable_redirects", label: "No Redirects" },
	{ value: "off", label: "Off" },
] as const;

const challengeOptions = [
	{ value: "http-01", label: "HTTP-01" },
	{ value: "cloudflare", label: "Cloudflare DNS" },
] as const;

const httpsDescriptions: Record<GlobalToggles["auto_https"], string> = {
	on: "Automatic certificates and HTTP-to-HTTPS redirects for all routes",
	disable_redirects: "Automatic certificates, but no HTTP-to-HTTPS redirects",
	off: "No automatic HTTPS",
};

export default function HttpsSettingsSection() {
	const [globalToggles, setGlobalToggles] = useState<GlobalToggles | null>(null);
	const [dnsTokenTouched, setDnsTokenTouched] = useState(false);
	const [saveWarning, setSaveWarning] = useState("");
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
		challengeType: "http-01" as ChallengeType,
		dnsToken: "",
	});

	useEffect(() => {
		Promise.all([fetchGlobalToggles(), fetchACMEEmail(), fetchDNSProvider()]).then(
			([toggles, acmeResult, dnsResult]) => {
				setGlobalToggles(toggles);
				load({
					httpsValue: toggles.auto_https,
					acmeEmail: acmeResult.email,
					challengeType: dnsResult.enabled ? "cloudflare" : "http-01",
					dnsToken: dnsResult.api_token ?? "",
				});
			},
		);
	}, [load]);

	if (!loaded || !globalToggles) return null;

	const httpsDirty = values.httpsValue !== saved.httpsValue;
	const acmeDirty = values.acmeEmail !== saved.acmeEmail;
	const challengeDirty = values.challengeType !== saved.challengeType;
	const dnsDirty = challengeDirty || (dnsTokenTouched && values.dnsToken !== "");
	const dirty = fieldsDirty || dnsDirty;

	const handleSave = () => {
		setSaveWarning("");

		if (values.httpsValue !== "off" && !values.acmeEmail) {
			setSaveWarning("ACME email is required for HTTPS certificates");
			return;
		}
		if (values.challengeType === "cloudflare" && !dnsTokenTouched && !saved.dnsToken) {
			setSaveWarning("Cloudflare API token is required for DNS challenges");
			return;
		}

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
					enabled: v.challengeType === "cloudflare",
					api_token: dnsTokenTouched ? v.dnsToken : undefined,
				});
				setDnsTokenTouched(false);
			}
			return "Saved";
		});
	};

	return (
		<section className="settings-section">
			<h3>HTTPS</h3>
			<div className="settings-field">
				<span className="settings-label" id="auto-https-label">
					Auto HTTPS
				</span>
				<Toggle
					options={httpsOptions}
					value={values.httpsValue}
					onChange={(v: GlobalToggles["auto_https"]) => {
						setValues((prev) => ({ ...prev, httpsValue: v }));
						setSaveWarning("");
					}}
					disabled={saving}
					aria-label="Auto HTTPS"
				/>
				<span className="settings-toggle-desc">{httpsDescriptions[values.httpsValue]}</span>
			</div>

			<div className="settings-field settings-field-spaced">
				<label htmlFor="acme-email">ACME email</label>
				<input
					id="acme-email"
					type="email"
					value={values.acmeEmail}
					onChange={(e) => {
						setValues((v) => ({ ...v, acmeEmail: e.target.value }));
						setSaveWarning("");
					}}
					placeholder="you@example.com"
					maxLength={254}
					disabled={saving}
				/>
				<span className="settings-toggle-desc">
					Email for Let's Encrypt certificate notifications
				</span>
			</div>

			<div className="settings-field settings-field-spaced">
				<span className="settings-label" id="challenge-type-label">
					Challenge type
				</span>
				<Toggle
					options={challengeOptions}
					value={values.challengeType}
					onChange={(v: ChallengeType) => {
						setValues((prev) => ({ ...prev, challengeType: v }));
						setSaveWarning("");
					}}
					disabled={saving}
					aria-label="Challenge type"
				/>
				<span className="settings-toggle-desc">
					{values.challengeType === "http-01"
						? "Automatic via port 80"
						: "DNS-01 challenges for wildcard certs and domains where HTTP-01 isn't viable"}
				</span>
				{values.challengeType === "cloudflare" && (
					<input
						type="password"
						value={dnsTokenTouched ? values.dnsToken : saved.dnsToken}
						onChange={(e) => {
							setValues((v) => ({ ...v, dnsToken: e.target.value }));
							setDnsTokenTouched(true);
							setSaveWarning("");
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
			</div>

			{saveWarning && <span className="settings-toggle-desc warning">{saveWarning}</span>}

			{dirty && (
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving}
					onClick={handleSave}
				>
					{saving ? "Saving..." : "Save"}
				</button>
			)}
			<Feedback msg={feedback.msg} type={feedback.type} />
		</section>
	);
}
