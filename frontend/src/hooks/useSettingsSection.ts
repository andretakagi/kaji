import { useCallback, useState } from "react";
import { useAsyncAction } from "./useAsyncAction";

export function useSettingsSection<T>(initial: T) {
	const [values, setValues] = useState(initial);
	const [saved, setSaved] = useState(initial);
	const [loaded, setLoaded] = useState(false);
	const action = useAsyncAction();

	const load = useCallback((data: T) => {
		setValues(data);
		setSaved(data);
		setLoaded(true);
	}, []);

	const markLoaded = useCallback(() => setLoaded(true), []);

	const dirty = Object.keys(saved as Record<string, unknown>).some(
		(k) => (values as Record<string, unknown>)[k] !== (saved as Record<string, unknown>)[k],
	);

	const save = (fn: (current: T) => Promise<string | undefined>) =>
		action.run(async () => {
			const msg = await fn(values);
			setSaved(values);
			return msg;
		});

	return { values, setValues, saved, dirty, loaded, load, markLoaded, save, ...action } as const;
}
