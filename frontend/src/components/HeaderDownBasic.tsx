import type { HeaderDownConfig } from "../types/api";
import { ToggleItem } from "./ToggleGrid";

interface Props {
	config: HeaderDownConfig;
	onChange: (config: HeaderDownConfig) => void;
}

export function HeaderDownBasic({ config, onChange }: Props) {
	return (
		<div className="headers-basic">
			<ToggleItem
				label="Strip Server"
				description="Remove the Server header from upstream responses"
				checked={config.strip_server}
				onChange={(v) => onChange({ ...config, strip_server: v })}
			/>
			<ToggleItem
				label="Strip X-Powered-By"
				description="Remove the X-Powered-By header from upstream responses"
				checked={config.strip_powered_by}
				onChange={(v) => onChange({ ...config, strip_powered_by: v })}
			/>
		</div>
	);
}
