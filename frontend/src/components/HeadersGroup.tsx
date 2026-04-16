import { useState } from "react";
import { cn } from "../cn";
import type { HeadersConfig, RouteToggles } from "../types/api";
import { HeadersAdvanced } from "./HeadersAdvanced";
import { HeadersBasic } from "./HeadersBasic";
import { Toggle } from "./Toggle";
import { ToggleItem } from "./ToggleGrid";

interface HeadersGroupProps {
	toggles: RouteToggles;
	onUpdate: <K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) => void;
	idPrefix: string;
	advancedMode?: boolean;
	onModeChange?: (advanced: boolean) => void;
}

function hasAdvancedResponseCustomizations(headers: HeadersConfig): boolean {
	return headers.response.builtin.length > 0 || headers.response.custom.length > 0;
}

function hasAdvancedRequestCustomizations(headers: HeadersConfig): boolean {
	return headers.request.builtin.length > 0 || headers.request.custom.length > 0;
}

export function ResponseHeadersGroup({
	toggles,
	onUpdate,
	idPrefix,
	advancedMode = false,
	onModeChange,
}: HeadersGroupProps) {
	const [advanced, setAdvanced] = useState(advancedMode);
	const enabled = toggles.headers.response.enabled;

	function updateHeaders(headers: HeadersConfig) {
		onUpdate("headers", headers);
	}

	return (
		<div className={cn("toggle-group", enabled && "toggle-group-open")}>
			<ToggleItem
				label="Response Headers"
				description="Security, CORS, caching, and custom response headers"
				checked={enabled}
				onChange={(v) =>
					onUpdate("headers", {
						...toggles.headers,
						response: { ...toggles.headers.response, enabled: v },
					})
				}
			/>
			{enabled && (
				<div className="toggle-detail">
					<div className="headers-mode-switch">
						<Toggle
							options={["basic", "advanced"] as const}
							value={advanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => {
								const isAdvanced = v === "advanced";
								setAdvanced(isAdvanced);
								onModeChange?.(isAdvanced);
							}}
						/>
					</div>

					{advanced ? (
						<HeadersAdvanced
							headers={toggles.headers}
							onChange={updateHeaders}
							section="response"
						/>
					) : (
						<>
							{hasAdvancedResponseCustomizations(toggles.headers) && (
								<span className="headers-advanced-warning">
									Advanced customizations exist. Saving in basic mode will reset headers to
									defaults. Switch to advanced mode to view them.
								</span>
							)}
							<HeadersBasic
								headers={toggles.headers}
								onChange={updateHeaders}
								idPrefix={idPrefix}
								section="response"
							/>
						</>
					)}
				</div>
			)}
		</div>
	);
}

export function RequestHeadersGroup({
	toggles,
	onUpdate,
	idPrefix,
	advancedMode = false,
	onModeChange,
}: HeadersGroupProps) {
	const [advanced, setAdvanced] = useState(advancedMode);
	const enabled = toggles.headers.request.enabled;

	function updateHeaders(headers: HeadersConfig) {
		onUpdate("headers", headers);
	}

	return (
		<div className={cn("toggle-group", enabled && "toggle-group-open")}>
			<ToggleItem
				label="Request Headers"
				description="Host override, authorization, and custom request headers"
				checked={enabled}
				onChange={(v) =>
					onUpdate("headers", {
						...toggles.headers,
						request: { ...toggles.headers.request, enabled: v },
					})
				}
			/>
			{enabled && (
				<div className="toggle-detail">
					<div className="headers-mode-switch">
						<Toggle
							options={["basic", "advanced"] as const}
							value={advanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => {
								const isAdvanced = v === "advanced";
								setAdvanced(isAdvanced);
								onModeChange?.(isAdvanced);
							}}
						/>
					</div>

					{advanced ? (
						<HeadersAdvanced headers={toggles.headers} onChange={updateHeaders} section="request" />
					) : (
						<>
							{hasAdvancedRequestCustomizations(toggles.headers) && (
								<span className="headers-advanced-warning">
									Advanced customizations exist. Saving in basic mode will reset headers to
									defaults. Switch to advanced mode to view them.
								</span>
							)}
							<HeadersBasic
								headers={toggles.headers}
								onChange={updateHeaders}
								idPrefix={idPrefix}
								section="request"
							/>
						</>
					)}
				</div>
			)}
		</div>
	);
}
