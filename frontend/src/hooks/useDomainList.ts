import { useCallback } from "react";
import { createDomain, deleteDomain, disableDomain, enableDomain, fetchDomains } from "../api";
import type { CreateDomainRequest, Domain } from "../types/domain";
import { useAsyncAction } from "./useAsyncAction";
import { usePolledData } from "./usePolledData";

export function useDomainList() {
	const {
		data: domains,
		loading,
		error,
		setError,
		reload,
	} = usePolledData<Domain[]>({
		fetcher: fetchDomains,
		initialData: [],
		errorPrefix: "Failed to load domains",
	});

	const { saving, feedback, setFeedback, run } = useAsyncAction();

	const handleCreate = useCallback(
		(req: CreateDomainRequest) =>
			run(async () => {
				await createDomain(req);
				await reload();
				return "Domain created";
			}),
		[run, reload],
	);

	const handleDelete = useCallback(
		(id: string) =>
			run(async () => {
				await deleteDomain(id);
				await reload();
				return "Domain deleted";
			}),
		[run, reload],
	);

	const handleToggleEnabled = useCallback(
		(id: string, enabled: boolean) =>
			run(async () => {
				if (enabled) {
					await enableDomain(id);
				} else {
					await disableDomain(id);
				}
				await reload();
				return enabled ? "Domain enabled" : "Domain disabled";
			}),
		[run, reload],
	);

	return {
		domains,
		loading,
		error,
		setError,
		saving,
		feedback,
		setFeedback,
		reload,
		handleCreate,
		handleDelete,
		handleToggleEnabled,
	} as const;
}
