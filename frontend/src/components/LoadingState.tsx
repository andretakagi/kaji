export default function LoadingState({ label }: { label: string }) {
	return (
		<div className="empty-state" role="status" aria-live="polite">
			Loading {label}...
		</div>
	);
}
