import type { HeadersConfig } from "../types/api";
import { ToggleItem } from "./ToggleGrid";

interface HeadersBasicProps {
	headers: HeadersConfig;
	onChange: (headers: HeadersConfig) => void;
	idPrefix: string;
}

export function HeadersBasic({ headers, onChange, idPrefix }: HeadersBasicProps) {
	function updateResponse(key: string, value: unknown) {
		onChange({
			...headers,
			response: { ...headers.response, [key]: value },
		});
	}

	function updateRequest(key: string, value: unknown) {
		onChange({
			...headers,
			request: { ...headers.request, [key]: value },
		});
	}

	return (
		<div className="headers-basic">
			<span className="toggle-detail-heading">Response</span>

			<ToggleItem
				label="Security Headers"
				description="HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy"
				checked={headers.response.security}
				onChange={(v) => updateResponse("security", v)}
			/>

			<div className="headers-sub-group">
				<ToggleItem
					label="CORS"
					description="Cross-origin resource sharing headers"
					checked={headers.response.cors}
					onChange={(v) => updateResponse("cors", v)}
				/>
				{headers.response.cors && (
					<div className="headers-sub-detail">
						<label htmlFor={`cors-origins-${idPrefix}`}>Allowed Origins</label>
						<input
							id={`cors-origins-${idPrefix}`}
							type="text"
							placeholder="* (all origins)"
							maxLength={2000}
							value={headers.response.cors_origins.join(", ")}
							onChange={(e) => {
								const origins = e.target.value
									.split(",")
									.map((s) => s.trim())
									.filter(Boolean);
								updateResponse("cors_origins", origins);
							}}
						/>
					</div>
				)}
			</div>

			<ToggleItem
				label="Cache-Control"
				description="Prevent browser caching (no-store)"
				checked={headers.response.cache_control}
				onChange={(v) => updateResponse("cache_control", v)}
			/>

			<ToggleItem
				label="X-Robots-Tag"
				description="Block search engine indexing (noindex, nofollow)"
				checked={headers.response.x_robots_tag}
				onChange={(v) => updateResponse("x_robots_tag", v)}
			/>

			<span className="toggle-detail-heading">Request</span>

			<div className="headers-sub-group">
				<ToggleItem
					label="Host Override"
					description="Replace the Host header sent to upstream"
					checked={headers.request.host_override}
					onChange={(v) => updateRequest("host_override", v)}
				/>
				{headers.request.host_override && (
					<div className="headers-sub-detail">
						<label htmlFor={`host-value-${idPrefix}`}>Host Value</label>
						<input
							id={`host-value-${idPrefix}`}
							type="text"
							placeholder="example.com"
							maxLength={255}
							value={headers.request.host_value}
							onChange={(e) => updateRequest("host_value", e.target.value)}
						/>
					</div>
				)}
			</div>

			<div className="headers-sub-group">
				<ToggleItem
					label="Authorization"
					description="Set Authorization header on upstream requests"
					checked={headers.request.authorization}
					onChange={(v) => updateRequest("authorization", v)}
				/>
				{headers.request.authorization && (
					<div className="headers-sub-detail">
						<label htmlFor={`auth-value-${idPrefix}`}>Authorization Value</label>
						<input
							id={`auth-value-${idPrefix}`}
							type="text"
							placeholder="Bearer token..."
							maxLength={2000}
							value={headers.request.auth_value}
							onChange={(e) => updateRequest("auth_value", e.target.value)}
						/>
					</div>
				)}
			</div>
		</div>
	);
}
