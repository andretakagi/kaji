import { useCallback, useEffect, useRef, useState } from "react";
import { POLL_INTERVAL } from "../api";
import { deepEqual } from "../deepEqual";
import { getErrorMessage } from "../utils/getErrorMessage";

interface UsePolledDataOptions<T> {
	fetcher: () => Promise<T>;
	initialData: T;
	errorPrefix: string;
	enabled?: boolean;
}

export function usePolledData<T>({
	fetcher,
	initialData,
	errorPrefix,
	enabled = true,
}: UsePolledDataOptions<T>) {
	const [data, setData] = useState<T>(initialData);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");
	const fetcherRef = useRef(fetcher);
	fetcherRef.current = fetcher;

	const load = useCallback(async () => {
		try {
			const result = await fetcherRef.current();
			setData((prev) => {
				if (deepEqual(prev, result)) return prev;
				return result;
			});
		} catch (err) {
			setError(getErrorMessage(err, errorPrefix));
		} finally {
			setLoading(false);
		}
	}, [errorPrefix]);

	useEffect(() => {
		if (!enabled) return;
		load();
		const id = setInterval(load, POLL_INTERVAL);
		return () => clearInterval(id);
	}, [load, enabled]);

	return { data, setData, loading, error, setError, reload: load } as const;
}