import { RequireCaddy, useCaddyStatus } from "../contexts/CaddyContext";
import { useDomainList } from "../hooks/useDomainList";
import { useFormToggle } from "../hooks/useFormToggle";
import DomainCard from "./DomainCard";
import DomainForm from "./DomainForm";
import { ErrorAlert } from "./ErrorAlert";
import Feedback from "./Feedback";
import LoadingState from "./LoadingState";
import { SectionHeader } from "./SectionHeader";

interface Props {
	onNavigate: (id: string) => void;
}

export default function DomainList({ onNavigate }: Props) {
	const { caddyRunning } = useCaddyStatus();
	const {
		domains,
		loading,
		error,
		setError,
		saving,
		feedback,
		handleCreate,
		handleDelete,
		handleToggleEnabled,
	} = useDomainList();

	const form = useFormToggle();

	async function onCreateDomain(req: Parameters<typeof handleCreate>[0]) {
		await handleCreate(req);
		form.close();
	}

	if (loading) {
		return <LoadingState label="domains" />;
	}

	return (
		<div className="domains">
			<SectionHeader title="Domains">
				<button
					type="button"
					className="btn btn-primary"
					disabled={!caddyRunning}
					onClick={form.toggle}
				>
					{form.visible ? "Cancel" : "Add Domain"}
				</button>
			</SectionHeader>

			<RequireCaddy message="Start it to manage domains." />

			<ErrorAlert message={error} onDismiss={() => setError("")} />
			<Feedback msg={feedback.msg} type={feedback.type} />

			{form.visible && (
				<DomainForm onCreate={onCreateDomain} onCancel={form.close} saving={saving} />
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
							onNavigate={onNavigate}
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
