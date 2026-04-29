import { useEffect, useRef, useState } from "react";
import { fetchGlobalToggles, fetchIPLists } from "../api";
import { cn } from "../cn";
import type { GlobalToggles, IPList } from "../types/api";
import type { DomainToggles, ErrorPage } from "../types/domain";
import { RequestHeadersGroup, ResponseHeadersGroup } from "./HeadersGroup";
import { Toggle } from "./Toggle";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	toggles: DomainToggles;
	onUpdate: <K extends keyof DomainToggles>(key: K, value: DomainToggles[K]) => void;
	idPrefix: string;
	domain?: string;
	hideHeaders?: boolean;
	disabled?: boolean;
	errorMessage?: string;
	isCreate?: boolean;
}

export function DomainToggleGrid({
	toggles,
	onUpdate,
	idPrefix,
	domain,
	hideHeaders,
	disabled,
	errorMessage,
	isCreate,
}: Props) {
	const [ipLists, setIpLists] = useState<IPList[]>([]);
	const [autoHttps, setAutoHttps] = useState<GlobalToggles["auto_https"] | null>(null);

	useEffect(() => {
		fetchGlobalToggles()
			.then((g) => setAutoHttps(g.auto_https))
			.catch(() => {});
	}, []);

	useEffect(() => {
		if (toggles.ip_filtering.enabled) {
			fetchIPLists()
				.then(setIpLists)
				.catch(() => {});
		}
	}, [toggles.ip_filtering.enabled]);

	return (
		<div className="toggle-grid">
			{autoHttps && autoHttps !== "off" ? (
				<ToggleItem
					label="Force HTTPS"
					description="Managed by global HTTPS setting"
					checked={autoHttps === "on"}
					onChange={() => {}}
					disabled
				/>
			) : (
				<ToggleItem
					label="Force HTTPS"
					description="Redirect HTTP requests to HTTPS"
					checked={toggles.force_https}
					onChange={(v) => onUpdate("force_https", v)}
					disabled={disabled}
				/>
			)}
			<ToggleItem
				label="Compression"
				description="gzip + zstd encoding"
				checked={toggles.compression}
				onChange={(v) => onUpdate("compression", v)}
				disabled={disabled}
			/>
			{!hideHeaders && (
				<RequestHeadersGroup
					toggles={toggles}
					onUpdate={onUpdate}
					idPrefix={idPrefix}
					disabled={disabled}
				/>
			)}
			{!hideHeaders && (
				<ResponseHeadersGroup
					toggles={toggles}
					onUpdate={onUpdate}
					idPrefix={idPrefix}
					disabled={disabled}
				/>
			)}
			<BasicAuthGroup
				toggles={toggles}
				onUpdate={onUpdate}
				idPrefix={idPrefix}
				disabled={disabled}
				isCreate={isCreate}
			/>
			<AccessLogGroup
				toggles={toggles}
				onUpdate={onUpdate}
				idPrefix={idPrefix}
				domain={domain}
				disabled={disabled}
			/>
			<IPFilteringGroup
				toggles={toggles}
				onUpdate={onUpdate}
				ipLists={ipLists}
				disabled={disabled}
			/>
			<ErrorPagesGroup
				toggles={toggles}
				onUpdate={onUpdate}
				idPrefix={idPrefix}
				disabled={disabled}
				errorMessage={errorMessage}
			/>
		</div>
	);
}

interface GroupProps {
	toggles: DomainToggles;
	onUpdate: <K extends keyof DomainToggles>(key: K, value: DomainToggles[K]) => void;
	idPrefix: string;
	disabled?: boolean;
}

function BasicAuthGroup({
	toggles,
	onUpdate,
	idPrefix,
	disabled,
	isCreate,
}: GroupProps & { isCreate?: boolean }) {
	return (
		<div className={cn("toggle-group", toggles.basic_auth.enabled && "toggle-group-open")}>
			<ToggleItem
				label="Basic Auth"
				description="Basic authentication"
				checked={toggles.basic_auth.enabled}
				onChange={(v) => onUpdate("basic_auth", { ...toggles.basic_auth, enabled: v })}
				disabled={disabled}
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
						disabled={disabled}
					/>
					<label htmlFor={`auth-pass-${idPrefix}`}>Password</label>
					<input
						id={`auth-pass-${idPrefix}`}
						type="password"
						placeholder={isCreate ? "password" : "(unchanged)"}
						maxLength={512}
						value={toggles.basic_auth.password}
						onChange={(e) =>
							onUpdate("basic_auth", {
								...toggles.basic_auth,
								password: e.target.value,
							})
						}
						disabled={disabled}
					/>
				</div>
			)}
		</div>
	);
}

function AccessLogGroup({
	toggles,
	onUpdate,
	idPrefix,
	domain,
	disabled,
}: GroupProps & { domain?: string }) {
	return (
		<div className={cn("toggle-group", toggles.access_log && "toggle-group-open")}>
			<ToggleItem
				label="Access Log"
				description="Log requests to this domain and its subdomains"
				checked={toggles.access_log !== ""}
				onChange={(v) => onUpdate("access_log", v ? "kaji_access" : "")}
				disabled={disabled}
			/>
			{toggles.access_log !== "" && (
				<div className="toggle-detail">
					<label className="toggle-radio-option">
						<input
							type="radio"
							name={`${idPrefix}-access-log-type`}
							checked={toggles.access_log === "kaji_access"}
							onChange={() => onUpdate("access_log", "kaji_access")}
							disabled={disabled}
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
							disabled={disabled}
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
							disabled={disabled}
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
	disabled,
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
				disabled={disabled}
			/>
			{toggles.ip_filtering.enabled && (
				<div className="toggle-detail">
					<Toggle
						options={["blacklist", "whitelist"] as const}
						value={toggles.ip_filtering.type as "blacklist" | "whitelist"}
						onChange={(v: "blacklist" | "whitelist") =>
							onUpdate("ip_filtering", { enabled: true, list_id: "", type: v })
						}
						disabled={disabled}
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
							disabled={disabled}
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

const statusCodePresets = [
	{ value: "4xx", label: "4xx - All client errors" },
	{ value: "5xx", label: "5xx - All server errors" },
	{ value: "403", label: "403 - Forbidden" },
	{ value: "404", label: "404 - Not Found" },
	{ value: "502", label: "502 - Bad Gateway" },
	{ value: "503", label: "503 - Service Unavailable" },
];

const contentTypePresets = [
	{ value: "text/html", label: "text/html" },
	{ value: "text/plain", label: "text/plain" },
	{ value: "application/json", label: "application/json" },
];

function isPresetStatusCode(code: string): boolean {
	return statusCodePresets.some((p) => p.value === code);
}

function isPresetContentType(ct: string): boolean {
	return contentTypePresets.some((p) => p.value === ct);
}

function ErrorPagesGroup({
	toggles,
	onUpdate,
	idPrefix,
	disabled,
	errorMessage,
}: GroupProps & { errorMessage?: string }) {
	const enabled = toggles.error_pages.length > 0;
	const nextKey = useRef(0);
	const [keys, setKeys] = useState<number[]>(() =>
		toggles.error_pages.map(() => nextKey.current++),
	);

	useEffect(() => {
		setKeys((prev) => {
			if (prev.length === toggles.error_pages.length) return prev;
			const next: number[] = [];
			for (let i = 0; i < toggles.error_pages.length; i++) {
				next.push(prev[i] ?? nextKey.current++);
			}
			return next;
		});
	}, [toggles.error_pages.length]);

	const addEntry = () => {
		const next: ErrorPage = { status_code: "404", body: "", content_type: "text/html" };
		const k = nextKey.current++;
		setKeys((prev) => [...prev, k]);
		onUpdate("error_pages", [...toggles.error_pages, next]);
	};

	const removeEntry = (index: number) => {
		setKeys((prev) => prev.filter((_, i) => i !== index));
		onUpdate(
			"error_pages",
			toggles.error_pages.filter((_, i) => i !== index),
		);
	};

	const updateEntry = (index: number, patch: Partial<ErrorPage>) => {
		onUpdate(
			"error_pages",
			toggles.error_pages.map((ep, i) => (i === index ? { ...ep, ...patch } : ep)),
		);
	};

	return (
		<div className={cn("toggle-group", enabled && "toggle-group-open")}>
			<ToggleItem
				label="Error Pages"
				description="Custom responses for error status codes"
				checked={enabled}
				onChange={(v) => {
					if (v) {
						const k = nextKey.current++;
						setKeys([k]);
						onUpdate("error_pages", [{ status_code: "404", body: "", content_type: "text/html" }]);
					} else {
						setKeys([]);
						onUpdate("error_pages", []);
					}
				}}
				disabled={disabled}
			/>
			{enabled && (
				<div className="toggle-detail">
					{toggles.error_pages.map((ep, i) => (
						<ErrorPageEntry
							key={keys[i]}
							entry={ep}
							index={i}
							idPrefix={idPrefix}
							onChange={(patch) => updateEntry(i, patch)}
							onRemove={() => removeEntry(i)}
							disabled={disabled}
							errorMessage={errorMessage}
						/>
					))}
					<button type="button" className="btn btn-ghost" onClick={addEntry} disabled={disabled}>
						+ Add Error Page
					</button>
				</div>
			)}
		</div>
	);
}

function ErrorPageEntry({
	entry,
	index,
	idPrefix,
	onChange,
	onRemove,
	disabled,
	errorMessage,
}: {
	entry: ErrorPage;
	index: number;
	idPrefix: string;
	onChange: (patch: Partial<ErrorPage>) => void;
	onRemove: () => void;
	disabled?: boolean;
	errorMessage?: string;
}) {
	const isCustomStatus = !isPresetStatusCode(entry.status_code);
	const isCustomContentType = !isPresetContentType(entry.content_type);

	return (
		<div className="error-page-entry">
			<div className="error-page-header">
				<span className="error-page-label">Error Page {index + 1}</span>
				<button
					type="button"
					className="btn btn-ghost header-row-delete"
					onClick={onRemove}
					disabled={disabled}
					aria-label={`Remove error page ${index + 1}`}
				>
					&#x2715;
				</button>
			</div>

			<div className="form-row">
				<div className="form-field">
					<label htmlFor={`${idPrefix}-ep-${index}-status`}>Status Code</label>
					<select
						id={`${idPrefix}-ep-${index}-status`}
						value={isCustomStatus ? "custom" : entry.status_code}
						onChange={(e) => {
							if (e.target.value === "custom") {
								onChange({ status_code: "" });
							} else {
								onChange({ status_code: e.target.value });
							}
						}}
						disabled={disabled}
					>
						{statusCodePresets.map((p) => (
							<option key={p.value} value={p.value}>
								{p.label}
							</option>
						))}
						<option value="custom">Custom...</option>
					</select>
					{isCustomStatus && (
						<input
							type="text"
							placeholder="e.g. 400, 404, 500"
							value={entry.status_code}
							onChange={(e) => onChange({ status_code: e.target.value })}
							disabled={disabled}
						/>
					)}
				</div>
				<div className="form-field">
					<label htmlFor={`${idPrefix}-ep-${index}-ct`}>Content Type</label>
					<select
						id={`${idPrefix}-ep-${index}-ct`}
						value={isCustomContentType ? "custom" : entry.content_type}
						onChange={(e) => {
							if (e.target.value === "custom") {
								onChange({ content_type: "" });
							} else {
								onChange({ content_type: e.target.value });
							}
						}}
						disabled={disabled}
					>
						{contentTypePresets.map((p) => (
							<option key={p.value} value={p.value}>
								{p.label}
							</option>
						))}
						<option value="custom">Custom...</option>
					</select>
					{isCustomContentType && (
						<input
							type="text"
							placeholder="e.g. text/xml"
							value={entry.content_type}
							onChange={(e) => onChange({ content_type: e.target.value })}
							disabled={disabled}
						/>
					)}
				</div>
			</div>

			<div className="form-field">
				<label htmlFor={`${idPrefix}-ep-${index}-body`}>Body</label>
				<textarea
					id={`${idPrefix}-ep-${index}-body`}
					placeholder="Response body for this error"
					value={entry.body}
					onChange={(e) => onChange({ body: e.target.value })}
					disabled={disabled}
					rows={4}
				/>
				{errorMessage !== undefined && (
					<span className="form-hint">
						{"Use {http.error.message} in the body to include the error message"}
						{errorMessage && (
							<>
								{": "}
								<code>{errorMessage}</code>
							</>
						)}
					</span>
				)}
			</div>
		</div>
	);
}
