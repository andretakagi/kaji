import { useRef, useState } from "react";
import type { HeaderEntry, HeadersConfig } from "../types/api";
import {
	builtinRequestKeys,
	builtinResponseKeys,
	expandBasicRequestToAdvanced,
	expandBasicToAdvanced,
} from "../utils/headerDefaults";
import { HeaderRow } from "./HeaderRow";

interface HeadersAdvancedProps {
	headers: HeadersConfig;
	onChange: (headers: HeadersConfig) => void;
}

interface KeyedEntry {
	id: number;
	entry: HeaderEntry;
}

function useKeyedEntries(entries: HeaderEntry[]) {
	const nextId = useRef(entries.length);
	const [keyed, setKeyed] = useState<KeyedEntry[]>(() =>
		entries.map((entry, i) => ({ id: i, entry })),
	);

	const prevEntries = useRef(entries);
	if (entries !== prevEntries.current) {
		const currentValues = keyed.map((k) => k.entry);
		const changed =
			entries.length !== currentValues.length || entries.some((e, i) => e !== currentValues[i]);
		if (changed) {
			const next = entries.map((entry, i) => ({ id: nextId.current + i, entry }));
			nextId.current += entries.length;
			setKeyed(next);
		}
		prevEntries.current = entries;
	}

	function add() {
		nextId.current += 1;
		const newEntry: KeyedEntry = {
			id: nextId.current,
			entry: { key: "", value: "", enabled: true },
		};
		const next = [...keyed, newEntry];
		setKeyed(next);
		return next.map((k) => k.entry);
	}

	function update(index: number, entry: HeaderEntry) {
		const next = [...keyed];
		next[index] = { ...next[index], entry };
		setKeyed(next);
		return next.map((k) => k.entry);
	}

	function remove(index: number) {
		const next = keyed.filter((_, i) => i !== index);
		setKeyed(next);
		return next.map((k) => k.entry);
	}

	return { keyed, add, update, remove };
}

export function HeadersAdvanced({ headers, onChange }: HeadersAdvancedProps) {
	const didExpand = useRef(false);

	const respCustom = useKeyedEntries(headers.response.custom);
	const reqCustom = useKeyedEntries(headers.request.custom);

	if (!didExpand.current) {
		let expanded = false;
		let next = { ...headers };

		if (headers.response.builtin.length === 0) {
			next = {
				...next,
				response: {
					...next.response,
					builtin: expandBasicToAdvanced(headers.response),
				},
			};
			expanded = true;
		}

		if (headers.request.builtin.length === 0) {
			next = {
				...next,
				request: {
					...next.request,
					builtin: expandBasicRequestToAdvanced(headers.request),
				},
			};
			expanded = true;
		}

		didExpand.current = true;

		if (expanded) {
			onChange(next);
			return null;
		}
	}

	const responseCustomKeys = new Set(headers.response.custom.map((e) => e.key));
	const requestCustomKeys = new Set(headers.request.custom.map((e) => e.key));

	function updateResponseBuiltin(index: number, entry: HeaderEntry) {
		const builtin = [...headers.response.builtin];
		builtin[index] = entry;
		onChange({ ...headers, response: { ...headers.response, builtin } });
	}

	function updateResponseCustom(index: number, entry: HeaderEntry) {
		const custom = respCustom.update(index, entry);
		onChange({ ...headers, response: { ...headers.response, custom } });
	}

	function addResponseCustom() {
		const custom = respCustom.add();
		onChange({ ...headers, response: { ...headers.response, custom } });
	}

	function deleteResponseCustom(index: number) {
		const custom = respCustom.remove(index);
		onChange({ ...headers, response: { ...headers.response, custom } });
	}

	function updateRequestBuiltin(index: number, entry: HeaderEntry) {
		const builtin = [...headers.request.builtin];
		builtin[index] = entry;
		onChange({ ...headers, request: { ...headers.request, builtin } });
	}

	function updateRequestCustom(index: number, entry: HeaderEntry) {
		const custom = reqCustom.update(index, entry);
		onChange({ ...headers, request: { ...headers.request, custom } });
	}

	function addRequestCustom() {
		const custom = reqCustom.add();
		onChange({ ...headers, request: { ...headers.request, custom } });
	}

	function deleteRequestCustom(index: number) {
		const custom = reqCustom.remove(index);
		onChange({ ...headers, request: { ...headers.request, custom } });
	}

	return (
		<div className="headers-advanced">
			<span className="toggle-detail-heading">Response Headers</span>

			<div className="headers-advanced-section">
				{headers.response.builtin.map((entry, i) => (
					<HeaderRow
						key={entry.key}
						entry={entry}
						isBuiltin={true}
						isOverridden={responseCustomKeys.has(entry.key)}
						onChange={(e) => updateResponseBuiltin(i, e)}
					/>
				))}

				{respCustom.keyed.length > 0 && <div className="headers-advanced-divider" />}

				{respCustom.keyed.map((k, i) => (
					<HeaderRow
						key={k.id}
						entry={k.entry}
						isBuiltin={false}
						isOverridden={builtinResponseKeys.has(k.entry.key)}
						onChange={(e) => updateResponseCustom(i, e)}
						onDelete={() => deleteResponseCustom(i)}
					/>
				))}

				<button type="button" className="btn btn-ghost" onClick={addResponseCustom}>
					+ Add Header
				</button>
			</div>

			<span className="toggle-detail-heading">Request Headers</span>

			<div className="headers-advanced-section">
				{headers.request.builtin.map((entry, i) => (
					<HeaderRow
						key={entry.key}
						entry={entry}
						isBuiltin={true}
						isOverridden={requestCustomKeys.has(entry.key)}
						onChange={(e) => updateRequestBuiltin(i, e)}
					/>
				))}

				{reqCustom.keyed.length > 0 && <div className="headers-advanced-divider" />}

				{reqCustom.keyed.map((k, i) => (
					<HeaderRow
						key={k.id}
						entry={k.entry}
						isBuiltin={false}
						isOverridden={builtinRequestKeys.has(k.entry.key)}
						onChange={(e) => updateRequestCustom(i, e)}
						onDelete={() => deleteRequestCustom(i)}
					/>
				))}

				<button type="button" className="btn btn-ghost" onClick={addRequestCustom}>
					+ Add Header
				</button>
			</div>
		</div>
	);
}
