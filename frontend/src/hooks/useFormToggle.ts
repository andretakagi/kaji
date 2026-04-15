import { useCallback, useRef, useState } from "react";

interface FormToggleOptions {
	onOpen?: () => void;
	onClose?: () => void;
}

export function useFormToggle({ onOpen, onClose }: FormToggleOptions = {}) {
	const [visible, setVisible] = useState(false);
	const stateRef = useRef({ visible, onOpen, onClose });
	stateRef.current = { visible, onOpen, onClose };

	const toggle = useCallback(() => {
		const { visible: isVisible, onOpen: open, onClose: close } = stateRef.current;
		if (isVisible) {
			close?.();
		} else {
			open?.();
		}
		setVisible(!isVisible);
	}, []);

	const open = useCallback(() => {
		stateRef.current.onOpen?.();
		setVisible(true);
	}, []);

	const close = useCallback(() => {
		stateRef.current.onClose?.();
		setVisible(false);
	}, []);

	return { visible, toggle, open, close };
}
