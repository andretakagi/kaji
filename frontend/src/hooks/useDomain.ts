import { useCallback } from "react";
import {
	createRule,
	createSubdomain,
	createSubdomainRule,
	deleteRule,
	deleteSubdomain,
	deleteSubdomainRule,
	disableRule,
	disableSubdomain,
	disableSubdomainRule,
	enableRule,
	enableSubdomain,
	enableSubdomainRule,
	fetchDomain,
	updateDomain,
	updateRule,
	updateSubdomain,
	updateSubdomainRule,
} from "../api";
import type {
	CreateRuleRequest,
	CreateSubdomainRequest,
	Domain,
	UpdateDomainRequest,
	UpdateRuleRequest,
	UpdateSubdomainRequest,
} from "../types/domain";
import { useAsyncAction } from "./useAsyncAction";
import { usePolledData } from "./usePolledData";

const emptyDomain: Domain = {
	id: "",
	name: "",
	enabled: true,
	toggles: {} as Domain["toggles"],
	rules: [],
	subdomains: [],
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

	const handleCreateSubdomain = useCallback(
		(req: CreateSubdomainRequest) =>
			run(async () => {
				await createSubdomain(id, req);
				await reload();
				return "Subdomain created";
			}),
		[id, run, reload],
	);

	const handleUpdateSubdomain = useCallback(
		(subId: string, req: UpdateSubdomainRequest) =>
			run(async () => {
				await updateSubdomain(id, subId, req);
				await reload();
				return "Subdomain updated";
			}),
		[id, run, reload],
	);

	const handleDeleteSubdomain = useCallback(
		(subId: string) =>
			run(async () => {
				await deleteSubdomain(id, subId);
				await reload();
				return "Subdomain deleted";
			}),
		[id, run, reload],
	);

	const handleToggleSubdomain = useCallback(
		(subId: string, enabled: boolean) =>
			run(async () => {
				if (enabled) {
					await enableSubdomain(id, subId);
				} else {
					await disableSubdomain(id, subId);
				}
				await reload();
			}),
		[id, run, reload],
	);

	const handleCreateSubdomainRule = useCallback(
		(subId: string, req: CreateRuleRequest) =>
			run(async () => {
				await createSubdomainRule(id, subId, req);
				await reload();
				return "Rule created";
			}),
		[id, run, reload],
	);

	const handleUpdateSubdomainRule = useCallback(
		(subId: string, ruleId: string, req: UpdateRuleRequest) =>
			run(async () => {
				await updateSubdomainRule(id, subId, ruleId, req);
				await reload();
				return "Rule updated";
			}),
		[id, run, reload],
	);

	const handleDeleteSubdomainRule = useCallback(
		(subId: string, ruleId: string) =>
			run(async () => {
				await deleteSubdomainRule(id, subId, ruleId);
				await reload();
				return "Rule deleted";
			}),
		[id, run, reload],
	);

	const handleToggleSubdomainRule = useCallback(
		(subId: string, ruleId: string, enabled: boolean) =>
			run(async () => {
				if (enabled) {
					await enableSubdomainRule(id, subId, ruleId);
				} else {
					await disableSubdomainRule(id, subId, ruleId);
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
		handleCreateSubdomain,
		handleUpdateSubdomain,
		handleDeleteSubdomain,
		handleToggleSubdomain,
		handleCreateSubdomainRule,
		handleUpdateSubdomainRule,
		handleDeleteSubdomainRule,
		handleToggleSubdomainRule,
	} as const;
}
