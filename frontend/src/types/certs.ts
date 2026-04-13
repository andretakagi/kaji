export interface CertInfo {
	domain: string;
	sans: string[];
	issuer: string;
	not_before: string;
	not_after: string;
	days_left: number;
	status: "valid" | "expiring" | "critical" | "expired" | "missing";
	managed: boolean;
	issuer_key: string;
	fingerprint: string;
}
