import { useState } from "react";
import { submitLogin } from "../api";
import { getErrorMessage } from "../utils/getErrorMessage";

function Login({ onSuccess }: { onSuccess: () => void }) {
	const [password, setPassword] = useState("");
	const [error, setError] = useState("");
	const [submitting, setSubmitting] = useState(false);

	const handleSubmit = async (e: React.SubmitEvent) => {
		e.preventDefault();
		setError("");
		setSubmitting(true);

		try {
			await submitLogin({ password });
			onSuccess();
		} catch (err) {
			setError(getErrorMessage(err, "Login failed."));
		} finally {
			setSubmitting(false);
		}
	};

	return (
		<div className="auth-wrapper">
			<form className="auth-card auth-card-narrow" onSubmit={handleSubmit}>
				<h1>Kaji</h1>
				<p>Enter your password to continue.</p>

				{error && (
					<div id="auth-error" className="inline-error auth-error" role="alert">
						{error}
					</div>
				)}

				<div className="auth-field">
					<label htmlFor="login-password">Password</label>
					<input
						id="login-password"
						type="password"
						autoComplete="current-password"
						value={password}
						onChange={(e) => setPassword(e.target.value)}
						placeholder="Admin password"
						required
						aria-invalid={error ? true : undefined}
						aria-describedby={error ? "auth-error" : undefined}
					/>
				</div>

				<button type="submit" className="btn btn-primary auth-submit" disabled={submitting}>
					{submitting ? "Signing in..." : "Sign In"}
				</button>
			</form>
		</div>
	);
}

export default Login;
