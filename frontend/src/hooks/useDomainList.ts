import { useCallback } from "react";
import { createDomainFull, deleteDomain, disableDomain, enableDomain, fetchDomains } from "../api";
import type { CreateDomainFullRequest, Domain } from "../types/domain";
import { useAsyncAction } from "./useAsyncAction";
import { usePolledData } from "./usePolledData";

export function useDomainList() {
	const {
		data: domains,
		setData: setDomains,
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
		(req: CreateDomainFullRequest) =>
			run(async () => {
				await createDomainFull(req);
				await reload();
				return "Domain created";
			}),
		[run, reload],
	);

	const handleDelete = useCallback(
		(id: string) =>
			run(async () => {
				await deleteDomain(id);
				setDomains((prev) => prev.filter((d) => d.id !== id));
				return "Domain deleted";
			}),
		[run, setDomains],
	);

	const handleToggleEnabled = useCallback(
		(id: string, enabled: boolean) =>
			run(async () => {
				const updated = enabled ? await enableDomain(id) : await disableDomain(id);
				setDomains((prev) => prev.map((d) => (d.id === updated.id ? updated : d)));
			}),
		[run, setDomains],
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
