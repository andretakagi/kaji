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

const ipv4Re = /^(\d{1,3}\.){3}\d{1,3}$/;
const cidrV4Re = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;
const ipv6Re = /^[0-9a-fA-F:]+$/;
const cidrV6Re = /^[0-9a-fA-F:]+\/\d{1,3}$/;

export function validateIPOrCIDR(value: string): string | null {
	const v = value.trim();
	if (!v) return "IP address is required";

	if (v.includes("/")) {
		if (cidrV4Re.test(v)) {
			const parts = v.split("/");
			const prefix = Number(parts[1]);
			if (prefix < 0 || prefix > 32) return "Invalid CIDR prefix (0-32 for IPv4)";
			const octets = parts[0].split(".");
			if (octets.some((o) => Number(o) > 255)) return "Invalid IPv4 address";
			return null;
		}
		if (cidrV6Re.test(v)) {
			const prefix = Number(v.split("/").pop());
			if (prefix < 0 || prefix > 128) return "Invalid CIDR prefix (0-128 for IPv6)";
			return null;
		}
		return "Invalid CIDR notation";
	}

	if (ipv4Re.test(v)) {
		const octets = v.split(".");
		if (octets.some((o) => Number(o) > 255)) return "Invalid IPv4 address";
		return null;
	}
	if (ipv6Re.test(v) && v.includes(":")) return null;
	return "Invalid IP address";
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
