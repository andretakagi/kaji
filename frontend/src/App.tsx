import type { ErrorInfo, ReactNode } from "react";
import { Component, useCallback, useEffect, useState } from "react";
import {
	fetchAuthStatus,
	fetchSetupStatus,
	fetchStatus,
	logout,
	POLL_INTERVAL,
	restartCaddy,
	startCaddy,
	stopCaddy,
} from "./api";
import { cn } from "./cn";
import { ErrorAlert } from "./components/ErrorAlert";
import IPLists from "./components/IPLists";
import Login from "./components/Login";
import Logs from "./components/Logs";
import Routes from "./components/Routes";
import Settings from "./components/Settings";
import Setup from "./components/Setup";
import Snapshots from "./components/Snapshots";
import { getErrorMessage } from "./utils/getErrorMessage";
import "./styles/App.css";

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
	state: { error: Error | null } = { error: null };

	static getDerivedStateFromError(error: Error) {
		return { error };
	}

	componentDidCatch(error: Error, info: ErrorInfo) {
		console.error("Uncaught error:", error, info.componentStack);
	}

	render() {
		if (this.state.error) {
			return (
				<div className="error-boundary">
					<h2>Something went wrong</h2>
					<p>{this.state.error.message}</p>
					<button
						type="button"
						className="btn btn-primary"
						onClick={() => {
							this.setState({ error: null });
							window.location.reload();
						}}
					>
						Reload
					</button>
				</div>
			);
		}
		return this.props.children;
	}
}

type View = "routes" | "ip-lists" | "logs" | "snapshots" | "settings";
type AppState = "loading" | "setup" | "login" | "ready";

function pathToView(pathname: string): View {
	const segment = pathname.replace(/^\//, "").split("/")[0];
	if (
		segment === "ip-lists" ||
		segment === "logs" ||
		segment === "snapshots" ||
		segment === "settings"
	)
		return segment;
	return "routes";
}

function App() {
	const [appState, setAppState] = useState<AppState>("loading");
	const [view, setView] = useState<View>(() => pathToView(window.location.pathname));
	const [authEnabled, setAuthEnabled] = useState(false);
	const [caddyRunning, setCaddyRunning] = useState<boolean | null>(null);
	const [serviceActing, setServiceActing] = useState(false);
	const [serviceError, setServiceError] = useState<string | null>(null);

	const navigate = useCallback((v: View) => {
		const path = v === "routes" ? "/" : `/${v}`;
		window.history.pushState(null, "", path);
		setView(v);
	}, []);

	useEffect(() => {
		const onPop = () => setView(pathToView(window.location.pathname));
		window.addEventListener("popstate", onPop);
		return () => window.removeEventListener("popstate", onPop);
	}, []);

	const checkAuth = useCallback(async () => {
		const auth = await fetchAuthStatus();
		setAuthEnabled(auth.auth_enabled);
		setAppState(auth.auth_enabled && !auth.authenticated ? "login" : "ready");
	}, []);

	useEffect(() => {
		fetchSetupStatus()
			.then((res) => {
				if (res.is_first_run) {
					setAppState("setup");
					if (window.location.pathname !== "/") {
						window.history.replaceState(null, "", "/");
					}
				} else {
					return checkAuth();
				}
			})
			.catch(() => checkAuth().catch(() => setAppState("ready")));
	}, [checkAuth]);

	useEffect(() => {
		if (appState !== "ready") return;
		let active = true;
		const poll = () => {
			fetchStatus()
				.then((s) => {
					if (active) setCaddyRunning(s.running);
				})
				.catch(() => {
					if (active) setCaddyRunning(false);
				});
		};
		poll();
		const id = setInterval(poll, POLL_INTERVAL);
		return () => {
			active = false;
			clearInterval(id);
		};
	}, [appState]);

	async function handleServiceAction(action: () => Promise<unknown>) {
		setServiceActing(true);
		setServiceError(null);
		try {
			await action();
			const s = await fetchStatus();
			setCaddyRunning(s.running);
		} catch (err) {
			setServiceError(getErrorMessage(err, "Action failed"));
		} finally {
			setServiceActing(false);
		}
	}

	if (appState === "loading") {
		return (
			<div className="app-loading" role="status">
				<span className="app-loading-text" aria-hidden="true">
					Kaji
				</span>
				<span className="sr-only">Loading...</span>
			</div>
		);
	}

	if (appState === "setup") {
		return (
			<main>
				<Setup
					onComplete={() => {
						window.history.replaceState(null, "", "/");
						setView("routes");
						checkAuth();
					}}
				/>
			</main>
		);
	}

	if (appState === "login") {
		return (
			<main>
				<Login onSuccess={() => setAppState("ready")} />
			</main>
		);
	}

	const handleLogout = async () => {
		try {
			await logout();
		} catch {
			// Still clear local session even if the server call fails
		}
		setAppState("login");
	};

	const navItems: { key: View; label: string }[] = [
		{ key: "routes", label: "Routes" },
		{ key: "ip-lists", label: "IP Lists" },
		{ key: "logs", label: "Logs" },
		{ key: "snapshots", label: "Snapshots" },
		{ key: "settings", label: "Settings" },
	];

	const running = caddyRunning ?? false;
	const statusKnown = caddyRunning !== null;

	return (
		<ErrorBoundary>
			<div className="app">
				<a href="#main-content" className="skip-link">
					Skip to content
				</a>
				<header className="app-header">
					<h1 className="sr-only">Kaji</h1>
					{statusKnown && (
						<div className="status-widget">
							<span className="status-widget-caddy">Caddy</span>
							<span
								className={cn("status-beacon", running ? "running" : "stopped")}
								role="status"
								aria-label={`Caddy is ${running ? "running" : "stopped"}`}
							/>
							<span className={cn("status-label", running ? "running" : "stopped")}>
								{running ? "Running" : "Stopped"}
							</span>
							<div className="status-widget-actions">
								{!running ? (
									<button
										type="button"
										className="svc-btn svc-start"
										disabled={serviceActing}
										onClick={() => handleServiceAction(startCaddy)}
									>
										Start
									</button>
								) : (
									<>
										<button
											type="button"
											className="svc-btn svc-stop"
											disabled={serviceActing}
											onClick={() => handleServiceAction(stopCaddy)}
										>
											Stop
										</button>
										<button
											type="button"
											className="svc-btn"
											disabled={serviceActing}
											onClick={() => handleServiceAction(restartCaddy)}
										>
											Restart
										</button>
									</>
								)}
							</div>
						</div>
					)}
					<div className="app-header-right">
						<nav className="app-nav app-nav-desktop" aria-label="Main navigation">
							{navItems.map((item) => (
								<button
									key={item.key}
									type="button"
									className={view === item.key ? "active" : ""}
									aria-current={view === item.key ? "page" : undefined}
									onClick={() => navigate(item.key)}
								>
									{item.label}
								</button>
							))}
						</nav>
						{authEnabled && (
							<button type="button" className="logout-btn" onClick={handleLogout}>
								Log out
							</button>
						)}
					</div>
				</header>

				<ErrorAlert
					message={serviceError}
					onDismiss={() => setServiceError(null)}
					className="service-error"
				/>

				<main id="main-content" className="app-content">
					{view === "routes" && <Routes caddyRunning={running} />}
					{view === "ip-lists" && <IPLists />}
					{view === "logs" && <Logs caddyRunning={running} />}
					{view === "snapshots" && <Snapshots />}
					{view === "settings" && <Settings onAuthChange={setAuthEnabled} caddyRunning={running} />}
				</main>

				<nav className="app-nav app-nav-mobile" aria-label="Main navigation, mobile">
					{navItems.map((item) => (
						<button
							key={item.key}
							type="button"
							className={view === item.key ? "active" : ""}
							aria-current={view === item.key ? "page" : undefined}
							onClick={() => navigate(item.key)}
						>
							{item.label}
						</button>
					))}
				</nav>
			</div>
		</ErrorBoundary>
	);
}

export default App;
