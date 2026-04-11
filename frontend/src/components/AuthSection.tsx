import { useEffect, useState } from "react";
import { changePassword, updateAuthEnabled } from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validatePassword } from "../utils/validate";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

interface AuthSectionProps {
	enabled: boolean;
	onChange: (v: boolean) => void;
}

export default function AuthSection({ enabled, onChange }: AuthSectionProps) {
	const [toggleValue, setToggleValue] = useState(enabled);
	useEffect(() => setToggleValue(enabled), [enabled]);

	const [saving, setSaving] = useState(false);
	const [error, setError] = useState("");
	const [newPw, setNewPw] = useState("");
	const [confirmPw, setConfirmPw] = useState("");

	const [cpNew, setCpNew] = useState("");
	const [cpConfirm, setCpConfirm] = useState("");
	const cp = useAsyncAction();

	const pendingDisable = enabled && !toggleValue;
	const pendingEnable = !enabled && toggleValue;
	const stayingOn = enabled && toggleValue;

	const handleToggle = () => {
		setToggleValue((v) => !v);
		setError("");
		setNewPw("");
		setConfirmPw("");
	};

	const handleDisable = async () => {
		setSaving(true);
		setError("");
		try {
			await updateAuthEnabled(false);
			onChange(false);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to disable auth"));
		} finally {
			setSaving(false);
		}
	};

	const handleEnable = async (e: React.SubmitEvent) => {
		e.preventDefault();
		setError("");

		const pwErr = validatePassword(newPw, confirmPw);
		if (pwErr) {
			setError(pwErr);
			return;
		}

		setSaving(true);
		try {
			await updateAuthEnabled(true, newPw);
			setNewPw("");
			setConfirmPw("");
			onChange(true);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to enable auth"));
		} finally {
			setSaving(false);
		}
	};

	const handleChangePassword = (e: React.SubmitEvent) => {
		e.preventDefault();

		const pwErr = validatePassword(cpNew, cpConfirm);
		if (pwErr) {
			cp.setFeedback({ msg: pwErr, type: "error" });
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
				<Toggle value={toggleValue} onChange={handleToggle} disabled={saving} />
			</div>
			{pendingDisable && (
				<div className="auth-password-form">
					<p className="settings-description">
						Authentication will be disabled. Anyone with access to this server will be able to use
						Kaji without a password.
					</p>
					<div className="auth-password-actions">
						<button
							type="button"
							className="btn btn-primary settings-save-btn"
							disabled={saving}
							onClick={handleDisable}
						>
							{saving ? "Saving..." : "Save"}
						</button>
						<button
							type="button"
							className="btn btn-ghost settings-cancel-btn"
							disabled={saving}
							onClick={() => {
								setToggleValue(true);
								setError("");
							}}
						>
							Cancel
						</button>
					</div>
				</div>
			)}
			{pendingEnable && (
				<form className="auth-password-form" onSubmit={handleEnable}>
					<p className="settings-description">Set a password to enable authentication.</p>
					<div className="settings-field">
						<label htmlFor="auth-pw">Password</label>
						<input
							id="auth-pw"
							type="password"
							autoComplete="new-password"
							maxLength={512}
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
							maxLength={512}
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
							onClick={() => {
								setToggleValue(false);
								setNewPw("");
								setConfirmPw("");
								setError("");
							}}
							disabled={saving}
						>
							Cancel
						</button>
					</div>
				</form>
			)}
			{error && <Feedback msg={error} type="error" className="settings-feedback" />}
			{stayingOn && (
				<form className="auth-password-form" onSubmit={handleChangePassword}>
					<h4 className="settings-subsection-title">Change Password</h4>
					<div className="settings-field">
						<label htmlFor="pw-new">New Password</label>
						<input
							id="pw-new"
							type="password"
							autoComplete="new-password"
							maxLength={512}
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
							maxLength={512}
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
