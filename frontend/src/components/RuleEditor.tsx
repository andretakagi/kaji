import type { HandlerConfigValue, Rule, RuleHandlerType } from "../types/domain";
import {
	defaultErrorConfig,
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
	handlerOptions,
} from "../types/domain";
import HandlerConfig from "./HandlerConfig";

interface Props {
	value: Rule;
	allowNone: boolean;
	onChange: (next: Rule) => void;
	idPrefix: string;
}

function defaultConfigFor(type: RuleHandlerType): HandlerConfigValue {
	switch (type) {
		case "reverse_proxy":
			return { ...defaultReverseProxyConfig };
		case "static_response":
			return { ...defaultStaticResponseConfig };
		case "redirect":
			return { ...defaultRedirectConfig };
		case "file_server":
			return { ...defaultFileServerConfig };
		case "error":
			return { ...defaultErrorConfig };
		case "none":
			return {};
	}
}

export default function RuleEditor({ value, allowNone, onChange, idPrefix }: Props) {
	const setHandler = (handler_type: RuleHandlerType) => {
		onChange({ ...value, handler_type, handler_config: defaultConfigFor(handler_type) });
	};
	const setConfig = (handler_config: HandlerConfigValue) => {
		onChange({ ...value, handler_config });
	};
	return (
		<div className="rule-editor">
			<div className="form-field">
				<label htmlFor={`${idPrefix}-handler`}>Handler</label>
				<select
					id={`${idPrefix}-handler`}
					value={value.handler_type}
					onChange={(e) => setHandler(e.target.value as RuleHandlerType)}
				>
					{allowNone && <option value="none">None</option>}
					{handlerOptions.map((o) => (
						<option key={o.value} value={o.value}>
							{o.label}
						</option>
					))}
				</select>
			</div>
			{value.handler_type !== "none" && (
				<HandlerConfig
					type={value.handler_type}
					config={value.handler_config}
					onChange={setConfig}
				/>
			)}
		</div>
	);
}
