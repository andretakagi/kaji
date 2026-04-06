export function validateDomain(domain: string): string | null {
	if (!domain.trim()) {
		return "Domain is required";
	}
	if (domain.length > 253) {
		return "Domain name is too long";
	}
	let check = domain;
	if (check.startsWith("*.")) {
		check = check.slice(2);
	}
	if (!check) {
		return "Domain is required";
	}
	const labels = check.split(".");
	if (labels.length < 2) {
		return "Domain must have at least two labels (e.g. example.com)";
	}
	for (const label of labels) {
		if (!label) {
			return "Domain has an empty label";
		}
		if (label.length > 63) {
			return "Domain label is too long (max 63 characters)";
		}
	}
	return null;
}

export function validateUpstream(upstream: string): string | null {
	if (!upstream.trim()) {
		return "Upstream is required";
	}
	const match = upstream.match(/^(.+):(\d+)$/);
	if (!match) {
		return "Upstream must be host:port (e.g. 127.0.0.1:8080)";
	}
	const port = Number(match[2]);
	if (port < 1 || port > 65535) {
		return "Upstream port must be between 1 and 65535";
	}
	return null;
}

export function validateCaddyAdminUrl(url: string): string | null {
	if (!url.trim()) {
		return "Caddy admin URL is required";
	}
	try {
		const parsed = new URL(url);
		if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
			return "Caddy admin URL must use http or https";
		}
		if (!parsed.hostname) {
			return "Caddy admin URL must include a hostname";
		}
	} catch {
		return "Invalid URL format";
	}
	return null;
}
