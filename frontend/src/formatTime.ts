function isSameDay(a: Date, b: Date): boolean {
	return (
		a.getFullYear() === b.getFullYear() &&
		a.getMonth() === b.getMonth() &&
		a.getDate() === b.getDate()
	);
}

interface FormatTimeOptions {
	seconds?: boolean;
}

export function formatTime(input: Date | number | string, opts?: FormatTimeOptions): string {
	const d = typeof input === "number" ? new Date(input * 1000) : new Date(input);

	if (isSameDay(d, new Date())) {
		return d.toLocaleTimeString([], {
			hour: "2-digit",
			minute: "2-digit",
			...(opts?.seconds && { second: "2-digit" }),
		});
	}
	return d.toLocaleDateString([], {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}
