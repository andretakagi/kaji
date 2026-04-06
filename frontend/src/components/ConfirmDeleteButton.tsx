import { useEffect, useRef, useState } from "react";
import { DeleteIcon } from "./DeleteIcon";

interface ConfirmDeleteButtonProps {
	onConfirm: () => void | Promise<void>;
	label: string;
	disabled?: boolean;
	deleting?: boolean;
	deletingLabel?: string;
}

const DISMISS_DELAY = 2000;

export function ConfirmDeleteButton({
	onConfirm,
	label,
	disabled,
	deleting,
	deletingLabel,
}: ConfirmDeleteButtonProps) {
	const [confirming, setConfirming] = useState(false);
	const [failed, setFailed] = useState(false);
	const dismissTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
	const isDeleting = deleting ?? false;

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
			<span className="delete-confirm">
				<button
					type="button"
					className="confirm-yes"
					onClick={handleConfirm}
					disabled={isDeleting || failed}
				>
					{isDeleting && deletingLabel ? deletingLabel : "Delete"}
				</button>
				<button type="button" className="confirm-no" onClick={() => setConfirming(false)}>
					No
				</button>
			</span>
		);
	}

	return (
		<button
			type="button"
			className="delete-btn"
			onClick={() => setConfirming(true)}
			disabled={disabled}
			title={label}
			aria-label={label}
		>
			<DeleteIcon />
		</button>
	);
}
