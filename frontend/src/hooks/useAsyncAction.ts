import { useCallback, useState } from "react";
import { getErrorMessage } from "../utils/getErrorMessage";

type Feedback = { msg: string; type: "success" | "error" };

const empty: Feedback = { msg: "", type: "success" };

export function useAsyncAction() {
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<Feedback>(empty);

	const run = useCallback(async (action: () => Promise<string | undefined>) => {
		setSaving(true);
		setFeedback(empty);
		try {
			const msg = (await action()) ?? "";
			setFeedback({ msg, type: "success" });
		} catch (err) {
			setFeedback({
				msg: getErrorMessage(err, "Something went wrong"),
				type: "error",
			});
		} finally {
			setSaving(false);
		}
	}, []);

	return { saving, feedback, setFeedback, run } as const;
}
