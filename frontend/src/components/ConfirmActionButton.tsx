import { type ReactNode, useEffect, useRef, useState } from "react";
import { cn } from "../cn";

interface ConfirmActionButtonProps {
	onConfirm: () => void | Promise<void>;
	trigger: ReactNode;
	confirmLabel?: string;
	confirmingLabel?: string;
	cancelLabel?: string;
	variant?: "danger" | "primary" | "ghost";
	disabled?: boolean;
	acting?: boolean;
	className?: string;
}

const DISMISS_DELAY = 2000;

export function ConfirmActionButton({
	onConfirm,
	trigger,
	confirmLabel = "Yes",
	confirmingLabel,
	cancelLabel = "Cancel",
	variant = "danger",
	disabled,
	acting,
	className,
}: ConfirmActionButtonProps) {
	const [confirming, setConfirming] = useState(false);
	const [failed, setFailed] = useState(false);
	const dismissTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
	const isActing = acting ?? false;

	useEffect(() => {
		return () => clearTimeout(dismissTimer.current);
	}, []);

	const handleConfirm = async () => {
		try {
			await onConfirm();
		} catch (_) {
			setFailed(true);
			dismissTimer.current = setTimeout(() => {
				setConfirming(false);
				setFailed(false);
			}, DISMISS_DELAY);
		}
	};

	if (confirming) {
		return (
			<span className={cn("confirm-inline", className)}>
				<button
					type="button"
					className={`btn btn-${variant} btn-sm`}
					onClick={handleConfirm}
					disabled={isActing || failed}
				>
					{isActing && confirmingLabel ? confirmingLabel : confirmLabel}
				</button>
				<button type="button" className="btn btn-ghost btn-sm" onClick={() => setConfirming(false)}>
					{cancelLabel}
				</button>
			</span>
		);
	}

	return (
		<button
			type="button"
			className={cn("btn", `btn-${variant}`, "btn-sm", className)}
			onClick={() => setConfirming(true)}
			disabled={disabled}
		>
			{trigger}
		</button>
	);
}
