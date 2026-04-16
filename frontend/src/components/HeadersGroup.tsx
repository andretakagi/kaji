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
}

function hasAdvancedCustomizations(headers: HeadersConfig): boolean {
	return (
		headers.response.builtin.length > 0 ||
		headers.response.custom.length > 0 ||
		headers.request.builtin.length > 0 ||
		headers.request.custom.length > 0
	);
}

export function HeadersGroup({
	toggles,
	onUpdate,
	idPrefix,
	advancedMode = false,
}: HeadersGroupProps) {
	const [advanced, setAdvanced] = useState(advancedMode);
	const enabled = toggles.headers.enabled;

	function updateHeaders(headers: HeadersConfig) {
		onUpdate("headers", headers);
	}

	return (
		<div className={cn("toggle-group", enabled && "toggle-group-open")}>
			<ToggleItem
				label="Headers"
				description="Response and request header management"
				checked={enabled}
				onChange={(v) => onUpdate("headers", { ...toggles.headers, enabled: v })}
			/>
			{enabled && (
				<div className="toggle-detail">
					<div className="headers-mode-switch">
						<Toggle
							options={["basic", "advanced"] as const}
							value={advanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => setAdvanced(v === "advanced")}
						/>
					</div>

					{advanced ? (
						<HeadersAdvanced headers={toggles.headers} onChange={updateHeaders} />
					) : (
						<>
							{hasAdvancedCustomizations(toggles.headers) && (
								<span className="headers-advanced-warning">
									Advanced customizations exist and will be preserved. Switch to advanced mode to
									view them.
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
