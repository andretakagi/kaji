export interface CaddyLogEntry {
	level: "debug" | "info" | "warn" | "error";
	ts: number;
	logger: string;
	msg: string;
	request?: {
		remote_addr: string;
		proto: string;
		method: string;
		host: string;
		uri: string;
		headers?: Record<string, string[]>;
		tls?: {
			resumed: boolean;
			version: number;
			cipher_suite: number;
			proto: string;
			server_name: string;
		};
	};
	duration?: number;
	status?: number;
	size?: number;
	resp_headers?: Record<string, string[]>;
	extra?: Record<string, unknown>;
}

export interface LogQueryParams {
	limit?: number;
	offset?: number;
	level?: CaddyLogEntry["level"];
	host?: string;
	status_min?: number;
	status_max?: number;
	since?: string;
	until?: string;
}

export interface LogQueryResponse {
	entries: CaddyLogEntry[];
	has_more: boolean;
}

export interface CaddyLogSink {
	writer?: {
		output: string;
		filename?: string;
		roll_size_mb?: number;
		roll_keep?: number;
		roll_keep_for?: number;
	};
	encoder?: {
		format: string;
	};
	level?: string;
	include?: string[];
	exclude?: string[];
}

export interface CaddyLoggingConfig {
	logs?: Record<string, CaddyLogSink>;
	log_dir?: string;
}

export interface LokiConfig {
	enabled: boolean;
	endpoint: string;
	bearer_token: string;
	tenant_id: string;
	labels: Record<string, string>;
	batch_size: number;
	flush_interval_seconds: number;
	sinks: string[];
}

export interface LokiSinkStatus {
	tailing: boolean;
	last_push_at: string;
	entries_pushed: number;
	last_error: string;
}

export interface LokiStatus {
	running: boolean;
	sinks: Record<string, LokiSinkStatus>;
}

export interface LokiTestResult {
	success: boolean;
	message: string;
}
