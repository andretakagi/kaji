import { useId } from "react";
import { cn } from "../cn";
import type { HeaderUpConfig } from "../types/api";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	config: HeaderUpConfig;
	onChange: (config: HeaderUpConfig) => void;
}

export function HeaderUpBasic({ config, onChange }: Props) {
	const idPrefix = useId();

	function update(patch: Partial<HeaderUpConfig>) {
		onChange({ ...config, ...patch });
	}

	return (
		<div className="headers-basic">
			<div className={cn("toggle-group", config.host_override && "toggle-group-open")}>
				<ToggleItem
					label="Host Override"
					description="Set the Host header sent to upstream"
					checked={config.host_override}
					onChange={(v) => update({ host_override: v })}
				/>
				{config.host_override && (
					<div className="toggle-detail">
						<label htmlFor={`${idPrefix}-host-value`}>Host Value</label>
						<input
							id={`${idPrefix}-host-value`}
							type="text"
							placeholder="example.com"
							value={config.host_value}
							onChange={(e) => update({ host_value: e.target.value })}
							maxLength={260}
						/>
					</div>
				)}
			</div>
			<div className={cn("toggle-group", config.authorization && "toggle-group-open")}>
				<ToggleItem
					label="Authorization"
					description="Set the Authorization header sent to upstream"
					checked={config.authorization}
					onChange={(v) => update({ authorization: v })}
				/>
				{config.authorization && (
					<div className="toggle-detail">
						<label htmlFor={`${idPrefix}-auth-value`}>Authorization Value</label>
						<input
							id={`${idPrefix}-auth-value`}
							type="text"
							placeholder="Bearer token123"
							value={config.auth_value}
							onChange={(e) => update({ auth_value: e.target.value })}
							maxLength={1024}
						/>
					</div>
				)}
			</div>
		</div>
	);
}
