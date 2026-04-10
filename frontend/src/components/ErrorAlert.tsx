interface ErrorAlertProps {
	message: string | null;
	onDismiss: () => void;
	className?: string;
}

export function ErrorAlert({ message, onDismiss, className }: ErrorAlertProps) {
	if (!message) return null;
	return (
		<div className={`alert-error${className ? ` ${className}` : ""}`} role="alert">
			{message}
			<button type="button" onClick={onDismiss}>
				Dismiss
			</button>
		</div>
	);
}
