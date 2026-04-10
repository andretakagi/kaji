import { cn } from "../cn";

interface ErrorAlertProps {
	message: string | null;
	onDismiss: () => void;
	className?: string;
}

export function ErrorAlert({ message, onDismiss, className }: ErrorAlertProps) {
	if (!message) return null;
	return (
		<div className={cn("alert-error", className)} role="alert">
			{message}
			<button type="button" onClick={onDismiss}>
				Dismiss
			</button>
		</div>
	);
}
