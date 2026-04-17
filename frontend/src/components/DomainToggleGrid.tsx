import { useEffect, useState } from "react";
import { fetchIPLists } from "../api";
import { cn } from "../cn";
import type { IPList } from "../types/api";
import type { DomainToggles } from "../types/domain";
import { RequestHeadersGroup, ResponseHeadersGroup } from "./HeadersGroup";
import { Toggle } from "./Toggle";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	toggles: DomainToggles;
	onUpdate: <K extends keyof DomainToggles>(key: K, value: DomainToggles[K]) => void;
	idPrefix: string;
	domain?: string;
}

export function DomainToggleGrid({ toggles, onUpdate, idPrefix, domain }: Props) {
	const [ipLists, setIpLists] = useState<IPList[]>([]);

	useEffect(() => {
		if (toggles.ip_filtering.enabled) {
			fetchIPLists()
				.then(setIpLists)
				.catch(() => {});
		}
	}, [toggles.ip_filtering.enabled]);

	return (
		<div className="toggle-grid">
			<ToggleItem
				label="Force HTTPS"
				description="Redirect HTTP requests to HTTPS"
				checked={toggles.force_https}
				onChange={(v) => onUpdate("force_https", v)}
			/>
			<ToggleItem
				label="Compression"
				description="gzip + zstd encoding"
				checked={toggles.compression}
				onChange={(v) => onUpdate("compression", v)}
			/>
			<RequestHeadersGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} />
			<ResponseHeadersGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} />
			<BasicAuthGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} />
			<AccessLogGroup toggles={toggles} onUpdate={onUpdate} idPrefix={idPrefix} domain={domain} />
			<IPFilteringGroup toggles={toggles} onUpdate={onUpdate} ipLists={ipLists} />
		</div>
	);
}

interface GroupProps {
	toggles: DomainToggles;
	onUpdate: <K extends keyof DomainToggles>(key: K, value: DomainToggles[K]) => void;
	idPrefix: string;
}

function BasicAuthGroup({ toggles, onUpdate, idPrefix }: GroupProps) {
	return (
		<div className={cn("toggle-group", toggles.basic_auth.enabled && "toggle-group-open")}>
			<ToggleItem
				label="Basic Auth"
				description="HTTP basic authentication"
				checked={toggles.basic_auth.enabled}
				onChange={(v) => onUpdate("basic_auth", { ...toggles.basic_auth, enabled: v })}
			/>
			{toggles.basic_auth.enabled && (
				<div className="toggle-detail">
					<label htmlFor={`auth-user-${idPrefix}`}>Username</label>
					<input
						id={`auth-user-${idPrefix}`}
						type="text"
						placeholder="admin"
						maxLength={255}
						value={toggles.basic_auth.username}
						onChange={(e) =>
							onUpdate("basic_auth", {
								...toggles.basic_auth,
								username: e.target.value,
							})
						}
					/>
					<label htmlFor={`auth-pass-${idPrefix}`}>Password</label>
					<input
						id={`auth-pass-${idPrefix}`}
						type="password"
						placeholder="(unchanged)"
						maxLength={512}
						value={toggles.basic_auth.password}
						onChange={(e) =>
							onUpdate("basic_auth", {
								...toggles.basic_auth,
								password: e.target.value,
							})
						}
					/>
				</div>
			)}
		</div>
	);
}

function AccessLogGroup({ toggles, onUpdate, idPrefix, domain }: GroupProps & { domain?: string }) {
	return (
		<div className={cn("toggle-group", toggles.access_log && "toggle-group-open")}>
			<ToggleItem
				label="Access Log"
				description="Log requests to routes under this domain"
				checked={toggles.access_log !== ""}
				onChange={(v) => onUpdate("access_log", v ? "kaji_access" : "")}
			/>
			{toggles.access_log !== "" && (
				<div className="toggle-detail">
					<label className="toggle-radio-option">
						<input
							type="radio"
							name={`${idPrefix}-access-log-type`}
							checked={toggles.access_log === "kaji_access"}
							onChange={() => onUpdate("access_log", "kaji_access")}
						/>
						<span>Shared (kaji_access)</span>
					</label>
					<label className="toggle-radio-option">
						<input
							type="radio"
							name={`${idPrefix}-access-log-type`}
							checked={toggles.access_log !== "" && toggles.access_log !== "kaji_access"}
							onChange={() => {
								const defaultName = (domain ?? "").replace(/[^a-zA-Z0-9_-]/g, "_") || "custom";
								onUpdate("access_log", defaultName);
							}}
						/>
						<span>Dedicated</span>
					</label>
					{toggles.access_log !== "" && toggles.access_log !== "kaji_access" && (
						<input
							id={`access-log-name-${idPrefix}`}
							type="text"
							placeholder="sink name"
							maxLength={255}
							value={toggles.access_log}
							onChange={(e) => {
								const sanitized = e.target.value.replace(/\s+/g, "_");
								onUpdate("access_log", sanitized);
							}}
						/>
					)}
				</div>
			)}
		</div>
	);
}

function IPFilteringGroup({
	toggles,
	onUpdate,
	ipLists,
}: Omit<GroupProps, "idPrefix"> & { ipLists: IPList[] }) {
	return (
		<div className={cn("toggle-group", toggles.ip_filtering.enabled && "toggle-group-open")}>
			<ToggleItem
				label="IP Filtering"
				description="Restrict access by IP whitelist or blacklist"
				checked={toggles.ip_filtering.enabled}
				onChange={(v) =>
					onUpdate(
						"ip_filtering",
						v
							? { enabled: true, list_id: "", type: "blacklist" }
							: { enabled: false, list_id: "", type: "" },
					)
				}
			/>
			{toggles.ip_filtering.enabled && (
				<div className="toggle-detail">
					<Toggle
						options={["blacklist", "whitelist"] as const}
						value={toggles.ip_filtering.type as "blacklist" | "whitelist"}
						onChange={(v: "blacklist" | "whitelist") =>
							onUpdate("ip_filtering", { enabled: true, list_id: "", type: v })
						}
					/>
					{toggles.ip_filtering.type && (
						<select
							value={toggles.ip_filtering.list_id}
							onChange={(e) =>
								onUpdate("ip_filtering", {
									...toggles.ip_filtering,
									list_id: e.target.value,
								})
							}
						>
							<option value="">Select a {toggles.ip_filtering.type}...</option>
							{ipLists
								.filter((l) => l.type === toggles.ip_filtering.type)
								.map((l) => (
									<option key={l.id} value={l.id}>
										{l.name} ({l.resolved_count} IPs)
									</option>
								))}
						</select>
					)}
				</div>
			)}
		</div>
	);
}
