import type { HandlerConfigValue, Rule, RuleHandlerType } from "../types/domain";
import {
	defaultFileServerConfig,
	defaultRedirectConfig,
	defaultReverseProxyConfig,
	defaultStaticResponseConfig,
} from "../types/domain";
import HandlerConfig from "./HandlerConfig";

interface Props {
	value: Rule;
	allowNone: boolean;
	onChange: (next: Rule) => void;
	idPrefix: string;
}

const handlerOptions: { value: Exclude<RuleHandlerType, "none">; label: string }[] = [
	{ value: "reverse_proxy", label: "Reverse Proxy" },
	{ value: "redirect", label: "Redirect" },
	{ value: "file_server", label: "File Server" },
	{ value: "static_response", label: "Static Response" },
];

function defaultConfigFor(t: RuleHandlerType): HandlerConfigValue {
	switch (t) {
		case "reverse_proxy":
			return { ...defaultReverseProxyConfig };
		case "static_response":
			return { ...defaultStaticResponseConfig };
		case "redirect":
			return { ...defaultRedirectConfig };
		case "file_server":
			return { ...defaultFileServerConfig };
		case "none":
			return {};
	}
}

export default function RuleEditor({ value, allowNone, onChange, idPrefix }: Props) {
	function setHandler(handler_type: RuleHandlerType) {
		onChange({ ...value, handler_type, handler_config: defaultConfigFor(handler_type) });
	}
	function setConfig(handler_config: HandlerConfigValue) {
		onChange({ ...value, handler_config });
	}
	return (
		<div className="rule-editor">
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
