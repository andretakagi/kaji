import { createContext, type ReactNode, useContext } from "react";

interface CaddyStatus {
	caddyRunning: boolean;
}

const CaddyContext = createContext<CaddyStatus | null>(null);

export function CaddyProvider({ running, children }: { running: boolean; children: ReactNode }) {
	return (
		<CaddyContext.Provider value={{ caddyRunning: running }}>{children}</CaddyContext.Provider>
	);
}

export function useCaddyStatus(): CaddyStatus {
	const ctx = useContext(CaddyContext);
	if (!ctx) throw new Error("useCaddyStatus must be used within a CaddyProvider");
	return ctx;
}

export function RequireCaddy({ message, children }: { message: string; children?: ReactNode }) {
	const { caddyRunning } = useCaddyStatus();
	if (!caddyRunning) {
		return (
			<div className="caddy-offline" role="status">
				Caddy is not running. {message}
			</div>
		);
	}
	return children;
}
