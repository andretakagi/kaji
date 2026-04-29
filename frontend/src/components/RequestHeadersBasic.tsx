import type { DomainRequestHeaders, HeadersConfig } from "../types/api";
import { ToggleItem } from "./ToggleGrid";

interface RequestHeadersBasicProps {
	headers: HeadersConfig;
	onChange: (headers: HeadersConfig) => void;
}

export function RequestHeadersBasic({ headers, onChange }: RequestHeadersBasicProps) {
	function updateRequest<K extends keyof DomainRequestHeaders>(
		key: K,
		value: DomainRequestHeaders[K],
	) {
		onChange({
			...headers,
			request: { ...headers.request, [key]: value },
		});
	}

	return (
		<div className="headers-basic">
			<ToggleItem
				label="X-Forwarded-For"
				description="Client IP address forwarded to the upstream"
				checked={headers.request.x_forwarded_for}
				onChange={(v) => updateRequest("x_forwarded_for", v)}
			/>
			<ToggleItem
				label="X-Real-IP"
				description="Real client IP, single value"
				checked={headers.request.x_real_ip}
				onChange={(v) => updateRequest("x_real_ip", v)}
			/>
			<ToggleItem
				label="X-Forwarded-Proto"
				description="Original request scheme (http or https)"
				checked={headers.request.x_forwarded_proto}
				onChange={(v) => updateRequest("x_forwarded_proto", v)}
			/>
			<ToggleItem
				label="X-Forwarded-Host"
				description="Original Host header sent by the client"
				checked={headers.request.x_forwarded_host}
				onChange={(v) => updateRequest("x_forwarded_host", v)}
			/>
			<ToggleItem
				label="X-Request-ID"
				description="Unique identifier for each request"
				checked={headers.request.x_request_id}
				onChange={(v) => updateRequest("x_request_id", v)}
			/>
		</div>
	);
}
