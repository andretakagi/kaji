import { useCallback } from "react";
import {
	createRule,
	deleteRule,
	disableRule,
	enableRule,
	fetchDomain,
	updateDomain,
	updateRule,
} from "../api";
import type {
	CreateRuleRequest,
	Domain,
	UpdateDomainRequest,
	UpdateRuleRequest,
} from "../types/domain";
import { useAsyncAction } from "./useAsyncAction";
import { usePolledData } from "./usePolledData";

const emptyDomain: Domain = {
	id: "",
	name: "",
	enabled: true,
	toggles: {} as Domain["toggles"],
	rules: [],
};

export function useDomain(id: string) {
	const {
		data: domain,
		loading,
		error,
		setError,
		reload,
	} = usePolledData<Domain>({
		fetcher: () => fetchDomain(id),
		initialData: emptyDomain,
		errorPrefix: "Failed to load domain",
		enabled: !!id,
	});

	const { saving, feedback, setFeedback, run } = useAsyncAction();

	const handleUpdateDomain = useCallback(
		(req: UpdateDomainRequest) =>
			run(async () => {
				await updateDomain(id, req);
				await reload();
				return "Domain updated";
			}),
		[id, run, reload],
	);

	const handleCreateRule = useCallback(
		(req: CreateRuleRequest) =>
			run(async () => {
				await createRule(id, req);
				await reload();
				return "Rule created";
			}),
		[id, run, reload],
	);

	const handleUpdateRule = useCallback(
		(ruleId: string, req: UpdateRuleRequest) =>
			run(async () => {
				await updateRule(id, ruleId, req);
				await reload();
				return "Rule updated";
			}),
		[id, run, reload],
	);

	const handleDeleteRule = useCallback(
		(ruleId: string) =>
			run(async () => {
				await deleteRule(id, ruleId);
				await reload();
				return "Rule deleted";
			}),
		[id, run, reload],
	);

	const handleToggleRule = useCallback(
		(ruleId: string, enabled: boolean) =>
			run(async () => {
				if (enabled) {
					await enableRule(id, ruleId);
				} else {
					await disableRule(id, ruleId);
				}
				await reload();
			}),
		[id, run, reload],
	);

	return {
		domain,
		loading,
		error,
		setError,
		saving,
		feedback,
		setFeedback,
		reload,
		handleUpdateDomain,
		handleCreateRule,
		handleUpdateRule,
		handleDeleteRule,
		handleToggleRule,
	} as const;
}
