import { useState } from "react";
import {
	deleteCertificate,
	downloadCertificate,
	fetchCertificates,
	renewCertificate,
} from "../api";
import { cn } from "../cn";
import { useAsyncAction } from "../hooks/useAsyncAction";
import { usePolledData } from "../hooks/usePolledData";
import type { CertInfo } from "../types/certs";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmActionButton } from "./ConfirmActionButton";
import Feedback from "./Feedback";
import LoadingState from "./LoadingState";

function formatFingerprint(hex: string): string {
	const upper = hex.toUpperCase().replace(/:/g, "");
	const pairs = upper.match(/.{1,2}/g) ?? [];
	const joined = pairs.join(":");
	if (joined.length > 60) return `${joined.slice(0, 59)}...`;
	return joined;
}

function expiryText(cert: CertInfo): string {
	if (cert.status === "expired") return "Expired";
	if (cert.days_left === 0) return "Expires today";
	if (cert.days_left === 1) return "Expires in 1 day";
	return `Expires in ${cert.days_left} days`;
}

export default function Certificates({ caddyRunning }: { caddyRunning: boolean }) {
	const {
		data: certs,
		loading,
		error,
		setError,
		reload,
	} = usePolledData({
		fetcher: fetchCertificates,
		initialData: [] as CertInfo[],
		errorPrefix: "Failed to load certificates",
		enabled: caddyRunning,
	});

	const renewAction = useAsyncAction();
	const deleteAction = useAsyncAction();

	const [forceDelete, setForceDelete] = useState<{
		issuerKey: string;
		domain: string;
	} | null>(null);

	const handleRenew = (cert: CertInfo) =>
		renewAction.run(async () => {
			await renewCertificate(cert.issuer_key, cert.domain);
			await reload();
			return "Certificate renewal requested";
		});

	const handleDelete = (cert: CertInfo) =>
		deleteAction.run(async () => {
			try {
				await deleteCertificate(cert.issuer_key, cert.domain);
				await reload();
				return "Certificate deleted";
			} catch (err) {
				const msg = getErrorMessage(err, "Failed to delete certificate");
				if (msg.toLowerCase().includes("active route") || msg.toLowerCase().includes("in use")) {
					setForceDelete({ issuerKey: cert.issuer_key, domain: cert.domain });
				}
				throw err;
			}
		});

	const handleForceDelete = () => {
		if (!forceDelete) return;
		const { issuerKey, domain } = forceDelete;
		deleteAction.run(async () => {
			await deleteCertificate(issuerKey, domain, true);
			setForceDelete(null);
			await reload();
			return "Certificate force deleted";
		});
	};

	if (!caddyRunning) {
		return (
			<div className="empty-state">Caddy is not running. Start Caddy to view certificates.</div>
		);
	}

	if (loading) {
		return <LoadingState label="certificates" />;
	}

	if (error) {
		return (
			<div className="empty-state">
				<p>{error}</p>
				<button
					type="button"
					className="btn btn-primary"
					onClick={() => {
						setError("");
						reload();
					}}
				>
					Retry
				</button>
			</div>
		);
	}

	return (
		<div className="certificates">
			<Feedback msg={renewAction.feedback.msg} type={renewAction.feedback.type} />
			<Feedback msg={deleteAction.feedback.msg} type={deleteAction.feedback.type} />

			{forceDelete && (
				<div className="cert-force-delete-banner" role="alert">
					<p>
						This certificate's domain has an active route. Deleting it may cause TLS errors until
						Caddy provisions a replacement.
					</p>
					<div className="cert-force-delete-actions">
						<button
							type="button"
							className="btn btn-danger btn-sm"
							onClick={handleForceDelete}
							disabled={deleteAction.saving}
						>
							{deleteAction.saving ? "Deleting..." : "Force Delete"}
						</button>
						<button
							type="button"
							className="btn btn-ghost btn-sm"
							onClick={() => setForceDelete(null)}
							disabled={deleteAction.saving}
						>
							Cancel
						</button>
					</div>
				</div>
			)}

			{certs.length === 0 ? (
				<div className="empty-state">
					No certificates found. Caddy automatically provisions TLS certificates when you create
					routes with public domains.
				</div>
			) : (
				<div className="cert-list">
					{certs.map((cert) => (
						<CertCard
							key={`${cert.issuer_key}-${cert.domain}`}
							cert={cert}
							onRenew={handleRenew}
							onDelete={handleDelete}
							renewingSaving={renewAction.saving}
							deletingSaving={deleteAction.saving}
						/>
					))}
				</div>
			)}
		</div>
	);
}

function CertCard({
	cert,
	onRenew,
	onDelete,
	renewingSaving,
	deletingSaving,
}: {
	cert: CertInfo;
	onRenew: (cert: CertInfo) => void;
	onDelete: (cert: CertInfo) => void;
	renewingSaving: boolean;
	deletingSaving: boolean;
}) {
	const title = (
		<>
			<span className={cn("cert-status-dot", `cert-${cert.status}`)} />
			<span className="cert-domain">{cert.domain}</span>
			<span className={cn("cert-expiry-text", `cert-${cert.status}`)}>{expiryText(cert)}</span>
		</>
	);

	return (
		<CollapsibleCard title={title} ariaLabel={cert.domain}>
			<div className="cert-details">
				{cert.sans.length > 1 && (
					<div className="cert-detail">
						<span className="cert-detail-label">SANs</span>
						<span className="cert-detail-value">{cert.sans.join(", ")}</span>
					</div>
				)}
				<div className="cert-detail">
					<span className="cert-detail-label">Issuer</span>
					<span className="cert-detail-value">{cert.issuer}</span>
				</div>
				<div className="cert-detail">
					<span className="cert-detail-label">Valid from</span>
					<span className="cert-detail-value">
						{new Date(cert.not_before).toLocaleDateString()}
					</span>
				</div>
				<div className="cert-detail">
					<span className="cert-detail-label">Valid until</span>
					<span className="cert-detail-value">{new Date(cert.not_after).toLocaleDateString()}</span>
				</div>
				<div className="cert-detail">
					<span className="cert-detail-label">Fingerprint</span>
					<span className="cert-detail-value cert-fingerprint">
						{formatFingerprint(cert.fingerprint)}
					</span>
				</div>
				<div className="cert-detail">
					<span className="cert-detail-label">Type</span>
					<span className="cert-detail-value">{cert.managed ? "ACME Managed" : "Manual"}</span>
				</div>

				<div className="cert-actions">
					{cert.managed && (
						<ConfirmActionButton
							onConfirm={() => onRenew(cert)}
							trigger="Renew"
							confirmLabel="Yes"
							confirmingLabel="Renewing..."
							variant="primary"
							acting={renewingSaving}
						/>
					)}
					<button
						type="button"
						className="btn btn-ghost btn-sm"
						onClick={() => downloadCertificate(cert.issuer_key, cert.domain)}
					>
						Download
					</button>
					<ConfirmActionButton
						onConfirm={() => onDelete(cert)}
						trigger="Delete"
						confirmLabel="Yes"
						confirmingLabel="Deleting..."
						variant="danger"
						acting={deletingSaving}
					/>
				</div>
			</div>
		</CollapsibleCard>
	);
}
