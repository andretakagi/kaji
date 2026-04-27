import { useCallback } from "react";
import {
	createDomainPath,
	createSubdomain,
	createSubdomainPath,
	deleteDomainPath,
	deleteSubdomain,
	deleteSubdomainPath,
	disableDomainPath,
	disableSubdomain,
	disableSubdomainPath,
	enableDomainPath,
	enableSubdomain,
	enableSubdomainPath,
	fetchDomain,
	updateDomain,
	updateDomainPath,
	updateDomainRule,
	updateSubdomain,
	updateSubdomainPath,
	updateSubdomainRule,
} from "../api";
import type {
	CreatePathRequest,
	CreateSubdomainRequest,
	Domain,
	UpdateDomainRequest,
	UpdatePathRequest,
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
	rule: { handler_type: "none", handler_config: {}, advanced_headers: false },
	subdomains: [],
	paths: [],
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

	const makeToggle = useCallback(
		<Args extends unknown[]>(
			enable: (...args: Args) => Promise<unknown>,
			disable: (...args: Args) => Promise<unknown>,
		) =>
			(...args: [...Args, boolean]) =>
				run(async () => {
					const enabled = args[args.length - 1] as boolean;
					const rest = args.slice(0, -1) as unknown as Args;
					if (enabled) {
						await enable(...rest);
					} else {
						await disable(...rest);
					}
					await reload();
				}),
		[run, reload],
	);

	const handleUpdateDomain = useCallback(
		(req: UpdateDomainRequest) =>
			run(async () => {
				await updateDomain(id, req);
				await reload();
				return "Domain updated";
			}),
		[id, run, reload],
	);

	const handleUpdateDomainRule = useCallback(
		(req: UpdateRuleRequest) =>
			run(async () => {
				await updateDomainRule(id, req);
				await reload();
				return "Domain rule updated";
			}),
		[id, run, reload],
	);

	const handleCreateDomainPath = useCallback(
		(req: CreatePathRequest) =>
			run(async () => {
				await createDomainPath(id, req);
				await reload();
				return "Domain path created";
			}),
		[id, run, reload],
	);

	const handleUpdateDomainPath = useCallback(
		(pathId: string, req: UpdatePathRequest) =>
			run(async () => {
				await updateDomainPath(id, pathId, req);
				await reload();
				return "Domain path updated";
			}),
		[id, run, reload],
	);

	const handleDeleteDomainPath = useCallback(
		(pathId: string) =>
			run(async () => {
				await deleteDomainPath(id, pathId);
				await reload();
				return "Domain path deleted";
			}),
		[id, run, reload],
	);

	const handleToggleDomainPath = makeToggle<[string]>(
		(pathId) => enableDomainPath(id, pathId),
		(pathId) => disableDomainPath(id, pathId),
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

	const handleToggleSubdomain = makeToggle<[string]>(
		(subId) => enableSubdomain(id, subId),
		(subId) => disableSubdomain(id, subId),
	);

	const handleUpdateSubdomainRule = useCallback(
		(subId: string, req: UpdateRuleRequest) =>
			run(async () => {
				await updateSubdomainRule(id, subId, req);
				await reload();
				return "Subdomain rule updated";
			}),
		[id, run, reload],
	);

	const handleCreateSubdomainPath = useCallback(
		(subId: string, req: CreatePathRequest) =>
			run(async () => {
				await createSubdomainPath(id, subId, req);
				await reload();
				return "Subdomain path created";
			}),
		[id, run, reload],
	);

	const handleUpdateSubdomainPath = useCallback(
		(subId: string, pathId: string, req: UpdatePathRequest) =>
			run(async () => {
				await updateSubdomainPath(id, subId, pathId, req);
				await reload();
				return "Subdomain path updated";
			}),
		[id, run, reload],
	);

	const handleDeleteSubdomainPath = useCallback(
		(subId: string, pathId: string) =>
			run(async () => {
				await deleteSubdomainPath(id, subId, pathId);
				await reload();
				return "Subdomain path deleted";
			}),
		[id, run, reload],
	);

	const handleToggleSubdomainPath = makeToggle<[string, string]>(
		(subId, pathId) => enableSubdomainPath(id, subId, pathId),
		(subId, pathId) => disableSubdomainPath(id, subId, pathId),
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
		handleUpdateDomainRule,
		handleCreateDomainPath,
		handleUpdateDomainPath,
		handleDeleteDomainPath,
		handleToggleDomainPath,
		handleCreateSubdomain,
		handleUpdateSubdomain,
		handleDeleteSubdomain,
		handleToggleSubdomain,
		handleUpdateSubdomainRule,
		handleCreateSubdomainPath,
		handleUpdateSubdomainPath,
		handleDeleteSubdomainPath,
		handleToggleSubdomainPath,
	} as const;
}
