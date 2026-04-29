import { useRef, useState } from "react";
import type { HeaderEntry, HeaderOperation } from "../types/api";
import { HeaderRow } from "./HeaderRow";
import { ToggleItem } from "./ToggleGrid";

interface HeadersAdvancedProps {
	builtin: HeaderEntry[];
	custom: HeaderEntry[];
	builtinKeySet: Set<string>;
	defaultBuiltins: HeaderEntry[];
	operations: HeaderOperation[];
	deferred?: boolean;
	onDeferredChange?: (v: boolean) => void;
	onBuiltinChange: (builtin: HeaderEntry[]) => void;
	onCustomChange: (custom: HeaderEntry[]) => void;
	expandFromToggles?: () => HeaderEntry[];
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
			entry: { key: "", value: "", operation: "set" as const, enabled: true },
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

export function HeadersAdvanced({
	builtin,
	custom,
	builtinKeySet,
	defaultBuiltins,
	operations,
	deferred,
	onDeferredChange,
	onBuiltinChange,
	onCustomChange,
	expandFromToggles,
}: HeadersAdvancedProps) {
	const didExpand = useRef(false);

	const customEntries = useKeyedEntries(custom);

	if (!didExpand.current) {
		didExpand.current = true;

		if (builtin.length === 0) {
			const initial = expandFromToggles ? expandFromToggles() : defaultBuiltins;
			onBuiltinChange(initial);
			return null;
		}
	}

	const customKeys = new Set(custom.map((e) => e.key));

	function updateBuiltin(index: number, entry: HeaderEntry) {
		const next = [...builtin];
		next[index] = entry;
		onBuiltinChange(next);
	}

	function updateCustom(index: number, entry: HeaderEntry) {
		onCustomChange(customEntries.update(index, entry));
	}

	function addCustom() {
		onCustomChange(customEntries.add());
	}

	function deleteCustom(index: number) {
		onCustomChange(customEntries.remove(index));
	}

	return (
		<div className="headers-advanced">
			<div className="headers-advanced-section">
				{onDeferredChange !== undefined && (
					<ToggleItem
						label="Deferred"
						description="Apply headers after response is received from upstream"
						checked={deferred ?? false}
						onChange={onDeferredChange}
					/>
				)}

				{builtin.map((entry, i) => (
					<HeaderRow
						key={entry.key}
						entry={entry}
						isBuiltin={true}
						isOverridden={customKeys.has(entry.key)}
						operations={operations}
						onChange={(e) => updateBuiltin(i, e)}
					/>
				))}

				{customEntries.keyed.length > 0 && <div className="headers-advanced-divider" />}

				{customEntries.keyed.map((k, i) => (
					<HeaderRow
						key={k.id}
						entry={k.entry}
						isBuiltin={false}
						isOverridden={builtinKeySet.has(k.entry.key)}
						operations={operations}
						onChange={(e) => updateCustom(i, e)}
						onDelete={() => deleteCustom(i)}
					/>
				))}

				<button type="button" className="btn btn-ghost" onClick={addCustom}>
					+ Add Header
				</button>
			</div>
		</div>
	);
}
