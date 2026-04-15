import { useCallback, useEffect, useRef, useState } from "react";
import {
	createRoute,
	deleteRoute,
	disableRoute,
	enableRoute,
	fetchGlobalToggles,
	updateRoute,
} from "../api";
import { useCaddyStatus } from "../contexts/CaddyContext";
import type { GlobalToggles, ParsedRoute, RouteToggles } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { defaultToggles } from "../utils/parseRoutes";
import { validateDomain, validateUpstream } from "../utils/validate";
import { useFormToggle } from "./useFormToggle";

interface UseRouteFormOptions {
	routes: ParsedRoute[];
	loadRoutes: () => Promise<void>;
	setError: (msg: string) => void;
}

export function useRouteForm({ routes, loadRoutes, setError }: UseRouteFormOptions) {
	const { caddyRunning } = useCaddyStatus();

	const [globalToggles, setGlobalToggles] = useState<GlobalToggles | null>(null);
	const [domain, setDomain] = useState("");
	const [upstream, setUpstream] = useState("");
	const [formToggles, setFormToggles] = useState<RouteToggles>({ ...defaultToggles });
	const form = useFormToggle({ onClose: () => setFormToggles({ ...defaultToggles }) });
	const [warning, setWarning] = useState("");
	const [submitting, setSubmitting] = useState(false);
	const [deleting, setDeleting] = useState<string | null>(null);
	const deletingRef = useRef(deleting);
	deletingRef.current = deleting;
	const [toggling, setToggling] = useState<string | null>(null);

	useEffect(() => {
		if (!caddyRunning) return;
		fetchGlobalToggles().then(setGlobalToggles);
	}, [caddyRunning]);

	function updateFormToggle<K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) {
		setFormToggles((prev) => ({ ...prev, [key]: value }));
	}

	async function handleAdd(e: React.SubmitEvent) {
		e.preventDefault();
		setError("");
		setWarning("");

		const domainErr = validateDomain(domain);
		if (domainErr) {
			setError(domainErr);
			return;
		}
		const upstreamErr = validateUpstream(upstream);
		if (upstreamErr) {
			setError(upstreamErr);
			return;
		}

		if (formToggles.load_balancing.enabled) {
			if (formToggles.load_balancing.upstreams.length === 0) {
				setError("Load balancing requires at least one additional upstream");
				return;
			}
			for (const u of formToggles.load_balancing.upstreams) {
				const err = validateUpstream(u);
				if (err) {
					setError(`Additional upstream: ${err}`);
					return;
				}
			}
		}

		if (formToggles.basic_auth.enabled) {
			if (!formToggles.basic_auth.username.trim()) {
				setError("Username is required for basic auth");
				return;
			}
			if (!formToggles.basic_auth.password) {
				setError("Password is required for basic auth");
				return;
			}
		}

		if (routes.some((r) => r.domain === domain.trim())) {
			setError("A route for this domain already exists");
			return;
		}

		setSubmitting(true);
		try {
			const res = await createRoute({
				domain: domain.trim(),
				upstream: upstream.trim(),
				toggles: formToggles,
			});
			if (res.warning) {
				setWarning(res.warning);
			}
			setDomain("");
			setUpstream("");
			form.close();
			await loadRoutes().catch(() => {});
		} catch (err) {
			setError(getErrorMessage(err, "Failed to create route"));
		} finally {
			setSubmitting(false);
		}
	}

	const handleDelete = useCallback(
		async (id: string) => {
			if (deletingRef.current) return;
			setWarning("");
			setDeleting(id);
			try {
				const res = await deleteRoute(id);
				if (res.warning) {
					setWarning(res.warning);
				}
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to delete route"));
			} finally {
				setDeleting(null);
			}
		},
		[loadRoutes, setError],
	);

	const handleToggleEnabled = useCallback(
		async (route: ParsedRoute) => {
			if (toggling) return;
			setWarning("");
			setToggling(route.id);
			try {
				const res = route.disabled ? await enableRoute(route.id) : await disableRoute(route.id);
				if (res.warning) {
					setWarning(res.warning);
				}
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to toggle route"));
			} finally {
				setToggling(null);
			}
		},
		[loadRoutes, toggling, setError],
	);

	const handleUpdateToggles = useCallback(
		async (route: ParsedRoute, toggles: RouteToggles) => {
			setWarning("");
			try {
				const res = await updateRoute({
					id: route.id,
					domain: route.domain,
					upstream: route.upstream,
					toggles,
				});
				if (res.warning) {
					setWarning(res.warning);
				}
				await loadRoutes().catch(() => {});
			} catch (err) {
				setError(getErrorMessage(err, "Failed to update route"));
				throw err;
			}
		},
		[loadRoutes, setError],
	);

	return {
		globalToggles,
		domain,
		setDomain,
		upstream,
		setUpstream,
		formToggles,
		updateFormToggle,
		form,
		warning,
		setWarning,
		submitting,
		deleting,
		toggling,
		handleAdd,
		handleDelete,
		handleToggleEnabled,
		handleUpdateToggles,
	};
}
