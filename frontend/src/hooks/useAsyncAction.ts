import { useCallback, useEffect, useRef, useState } from "react";
import { getErrorMessage } from "../utils/getErrorMessage";

export type Feedback = { msg: string; type: "success" | "error" };

const empty: Feedback = { msg: "", type: "success" };
const SUCCESS_TIMEOUT_MS = 12000;

export function useAsyncAction() {
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<Feedback>(empty);
	const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

	const clearTimer = useCallback(() => {
		if (timerRef.current !== null) {
			clearTimeout(timerRef.current);
			timerRef.current = null;
		}
	}, []);

	useEffect(() => clearTimer, [clearTimer]);

	const updateFeedback = useCallback(
		(next: Feedback) => {
			clearTimer();
			setFeedback(next);
			if (next.type === "success" && next.msg) {
				timerRef.current = setTimeout(() => {
					setFeedback(empty);
					timerRef.current = null;
				}, SUCCESS_TIMEOUT_MS);
			}
		},
		[clearTimer],
	);

	const run = useCallback(
		async (action: () => Promise<string | undefined>): Promise<boolean> => {
			setSaving(true);
			clearTimer();
			setFeedback(empty);
			try {
				const msg = (await action()) ?? "";
				updateFeedback({ msg, type: "success" });
				return true;
			} catch (err) {
				updateFeedback({
					msg: getErrorMessage(err, "Something went wrong"),
					type: "error",
				});
				return false;
			} finally {
				setSaving(false);
			}
		},
		[clearTimer, updateFeedback],
	);

	return { saving, feedback, setFeedback: updateFeedback, run } as const;
}
