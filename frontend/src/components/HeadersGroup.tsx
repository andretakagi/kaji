import { useState } from "react";
import { cn } from "../cn";
import type { HeadersConfig } from "../types/api";
import {
	builtinDomainRequestKeys,
	builtinResponseKeys,
	defaultDomainRequestBuiltins,
	defaultResponseBuiltins,
	expandBasicDomainRequestToAdvanced,
	expandBasicToAdvanced,
} from "../utils/headerDefaults";
import { HeadersAdvanced } from "./HeadersAdvanced";
import { HeadersBasic } from "./HeadersBasic";
import { RequestHeadersBasic } from "./RequestHeadersBasic";
import { Toggle } from "./Toggle";
import { ToggleItem } from "./ToggleGrid";

interface HeadersGroupProps {
	toggles: { headers: HeadersConfig };
	onUpdate: (key: "headers", value: HeadersConfig) => void;
	idPrefix: string;
	advancedMode?: boolean;
	onModeChange?: (advanced: boolean) => void;
	disabled?: boolean;
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
	disabled,
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
				disabled={disabled}
			/>
			{enabled && (
				<div className={cn("toggle-detail", disabled && "toggle-detail-disabled")}>
					<div className="headers-mode-switch">
						<Toggle
							options={["basic", "advanced"] as const}
							value={advanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => {
								const isAdvanced = v === "advanced";
								setAdvanced(isAdvanced);
								onModeChange?.(isAdvanced);
							}}
							disabled={disabled}
						/>
					</div>

					{advanced ? (
						<HeadersAdvanced
							builtin={toggles.headers.response.builtin}
							custom={toggles.headers.response.custom}
							builtinKeySet={builtinResponseKeys}
							defaultBuiltins={defaultResponseBuiltins}
							operations={["set", "add", "delete"]}
							deferred={toggles.headers.response.deferred}
							onDeferredChange={(v) =>
								updateHeaders({
									...toggles.headers,
									response: { ...toggles.headers.response, deferred: v },
								})
							}
							onBuiltinChange={(builtin) =>
								updateHeaders({
									...toggles.headers,
									response: { ...toggles.headers.response, builtin },
								})
							}
							onCustomChange={(custom) =>
								updateHeaders({
									...toggles.headers,
									response: { ...toggles.headers.response, custom },
								})
							}
							expandFromToggles={() => expandBasicToAdvanced(toggles.headers.response)}
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
	advancedMode = false,
	onModeChange,
	disabled,
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
				description="Forwarding and identification headers sent to the upstream"
				checked={enabled}
				onChange={(v) =>
					onUpdate("headers", {
						...toggles.headers,
						request: { ...toggles.headers.request, enabled: v },
					})
				}
				disabled={disabled}
			/>
			{enabled && (
				<div className={cn("toggle-detail", disabled && "toggle-detail-disabled")}>
					<div className="headers-mode-switch">
						<Toggle
							options={["basic", "advanced"] as const}
							value={advanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => {
								const isAdvanced = v === "advanced";
								setAdvanced(isAdvanced);
								onModeChange?.(isAdvanced);
							}}
							disabled={disabled}
						/>
					</div>

					{advanced ? (
						<HeadersAdvanced
							builtin={toggles.headers.request.builtin}
							custom={toggles.headers.request.custom}
							builtinKeySet={builtinDomainRequestKeys}
							defaultBuiltins={defaultDomainRequestBuiltins}
							operations={["set", "add", "delete", "replace"]}
							onBuiltinChange={(builtin) =>
								updateHeaders({
									...toggles.headers,
									request: { ...toggles.headers.request, builtin },
								})
							}
							onCustomChange={(custom) =>
								updateHeaders({
									...toggles.headers,
									request: { ...toggles.headers.request, custom },
								})
							}
							expandFromToggles={() => expandBasicDomainRequestToAdvanced(toggles.headers.request)}
						/>
					) : (
						<>
							{hasAdvancedRequestCustomizations(toggles.headers) && (
								<span className="headers-advanced-warning">
									Advanced customizations exist. Saving in basic mode will reset headers to
									defaults. Switch to advanced mode to view them.
								</span>
							)}
							<RequestHeadersBasic headers={toggles.headers} onChange={updateHeaders} />
						</>
					)}
				</div>
			)}
		</div>
	);
}
