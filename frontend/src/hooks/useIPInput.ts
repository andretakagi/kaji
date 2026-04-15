import { type Dispatch, type SetStateAction, useState } from "react";
import { validateIPOrCIDR } from "../utils/validate";

export function useIPInput(ips: string[], setIps: Dispatch<SetStateAction<string[]>>) {
	const [input, setInputRaw] = useState("");
	const [error, setError] = useState<string | null>(null);

	function setInput(value: string) {
		setInputRaw(value);
		setError(null);
	}

	function add() {
		const val = input.trim();
		if (!val) return;
		const err = validateIPOrCIDR(val);
		if (err) {
			setError(err);
			return;
		}
		if (ips.includes(val)) {
			setError("Already in list");
			return;
		}
		setIps((prev) => [...prev, val]);
		setInputRaw("");
		setError(null);
	}

	function remove(ip: string) {
		setIps((prev) => prev.filter((x) => x !== ip));
	}

	function reset() {
		setInputRaw("");
		setError(null);
	}

	return { input, setInput, error, add, remove, reset };
}
