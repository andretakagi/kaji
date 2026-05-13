import { createContext, type ReactNode, useCallback, useContext, useEffect, useState } from "react";
import { fetchUpstreams, POLL_INTERVAL } from "../api";
import type { UpstreamStatus } from "../types/api";
import { useCaddyStatus } from "./CaddyContext";

type UpstreamState = "healthy" | "unhealthy" | "unknown";

interface UpstreamStatusValue {
	getUpstreamState: (address: string) => UpstreamState;
}

const UpstreamStatusContext = createContext<UpstreamStatusValue | null>(null);

export function UpstreamStatusProvider({ children }: { children: ReactNode }) {
	const { caddyRunning } = useCaddyStatus();
	const [upstreams, setUpstreams] = useState<Map<string, UpstreamStatus>>(new Map());
	const [loaded, setLoaded] = useState(false);

	useEffect(() => {
		if (!caddyRunning) {
			setUpstreams(new Map());
			setLoaded(false);
			return;
		}

		let active = true;
		const poll = () => {
			fetchUpstreams()
				.then((list) => {
					if (!active) return;
					const map = new Map<string, UpstreamStatus>();
					for (const u of list) {
						map.set(u.address, u);
					}
					setUpstreams(map);
					setLoaded(true);
				})
				.catch(() => {
					if (active) {
						setUpstreams(new Map());
						setLoaded(false);
					}
				});
		};
		poll();
		const id = setInterval(poll, POLL_INTERVAL);
		return () => {
			active = false;
			clearInterval(id);
		};
	}, [caddyRunning]);

	const getUpstreamState = useCallback(
		(address: string): UpstreamState => {
			if (!caddyRunning || !loaded) return "unknown";
			return upstreams.has(address) ? "healthy" : "unhealthy";
		},
		[caddyRunning, loaded, upstreams],
	);

	return (
		<UpstreamStatusContext.Provider value={{ getUpstreamState }}>
			{children}
		</UpstreamStatusContext.Provider>
	);
}

export function useUpstreamStatus(): UpstreamStatusValue {
	const ctx = useContext(UpstreamStatusContext);
	if (!ctx) throw new Error("useUpstreamStatus must be used within an UpstreamStatusProvider");
	return ctx;
}
