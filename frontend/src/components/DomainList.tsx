import { RequireCaddy, useCaddyStatus } from "../contexts/CaddyContext";
import { useDomainList } from "../hooks/useDomainList";
import { useFormToggle } from "../hooks/useFormToggle";
import DomainCard from "./DomainCard";
import DomainWizard from "./DomainWizard";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import LoadingState from "./LoadingState";
import { SectionHeader } from "./SectionHeader";

export default function DomainList() {
	const { caddyRunning } = useCaddyStatus();
	const {
		domains,
		loading,
		error,
		setError,
		saving,
		feedback,
		handleCreate,
		handleCreateSubdomain,
		handleDelete,
		handleToggleEnabled,
	} = useDomainList();

	const form = useFormToggle();

	async function onCreateDomain(req: Parameters<typeof handleCreate>[0]) {
		await handleCreate(req);
		form.close();
	}

	async function onCreateSubdomain(...args: Parameters<typeof handleCreateSubdomain>) {
		await handleCreateSubdomain(...args);
		form.close();
	}

	if (loading) {
		return <LoadingState label="domains" />;
	}

	return (
		<div className="domains">
			<SectionHeader title="Routes">
				{!form.visible && (
					<button
						type="button"
						className="btn btn-primary"
						disabled={!caddyRunning}
						onClick={form.open}
					>
						Add Domain
					</button>
				)}
			</SectionHeader>

			{!error && <RequireCaddy message="Start it to manage domains." />}

			<ErrorAlert message={error} onDismiss={() => setError("")} />
			<Feedback msg={feedback.msg} type={feedback.type} />

			{form.visible && (
				<DomainWizard
					onCreate={onCreateDomain}
					onCreateSubdomain={onCreateSubdomain}
					onCancel={form.close}
					existingDomains={domains}
				/>
			)}

			{domains.length === 0 ? (
				<div className="empty-state">
					No domains yet. Domains group rules under a single hostname, with shared settings for
					HTTPS, compression, headers, and more.
				</div>
			) : (
				<div className="domain-list">
					{domains.map((domain) => (
						<DomainCard
							key={domain.id}
							domain={domain}
							onToggleEnabled={handleToggleEnabled}
							onDelete={handleDelete}
							saving={saving}
						/>
					))}
				</div>
			)}
		</div>
	);
}
