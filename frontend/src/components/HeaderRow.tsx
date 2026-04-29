import { useState } from "react";
import { cn } from "../cn";
import type { HeaderEntry, HeaderOperation } from "../types/api";
import { miniEditors } from "./MiniEditors";
import { Toggle } from "./Toggle";

interface HeaderRowProps {
	entry: HeaderEntry;
	isBuiltin: boolean;
	isOverridden: boolean;
	operations: HeaderOperation[];
	onChange: (entry: HeaderEntry) => void;
	onDelete?: () => void;
}

export function HeaderRow({
	entry,
	isBuiltin,
	isOverridden,
	operations,
	onChange,
	onDelete,
}: HeaderRowProps) {
	const [expanded, setExpanded] = useState(false);
	const MiniEditor = miniEditors[entry.key];
	const op = entry.operation;
	const showValue = op !== "delete";
	const showSearch = op === "replace";

	function handleOpChange(next: HeaderOperation) {
		const update: HeaderEntry = { ...entry, operation: next };
		if (next !== "replace") {
			delete update.search;
		}
		onChange(update);
	}

	return (
		<div
			className={cn(
				"header-row",
				isOverridden && "header-row-overridden",
				expanded && "header-row-expanded",
			)}
		>
			<div className="header-row-main">
				<Toggle
					inline
					small
					value={entry.enabled}
					onChange={(v) => onChange({ ...entry, enabled: v })}
					aria-label={`Toggle ${entry.key}`}
				/>

				<div className="header-row-op">
					{isBuiltin ? (
						<span className="header-row-op-label">{op}</span>
					) : (
						<select
							className="header-row-op-select"
							value={op}
							onChange={(e) => handleOpChange(e.target.value as HeaderOperation)}
						>
							{operations.map((o) => (
								<option key={o} value={o}>
									{o}
								</option>
							))}
						</select>
					)}
				</div>

				{isBuiltin ? (
					<span className="header-row-key">{entry.key}</span>
				) : (
					<input
						type="text"
						className="header-row-key-input"
						placeholder="Header-Name"
						maxLength={255}
						value={entry.key}
						onChange={(e) => onChange({ ...entry, key: e.target.value })}
					/>
				)}

				{showSearch && (
					<input
						type="text"
						className="header-row-search-input"
						placeholder="search"
						maxLength={4096}
						value={entry.search ?? ""}
						onChange={(e) => onChange({ ...entry, search: e.target.value })}
					/>
				)}

				{showValue && (
					<input
						type="text"
						className="header-row-value-input"
						placeholder={showSearch ? "replace" : "value"}
						maxLength={4096}
						value={entry.value}
						onChange={(e) => onChange({ ...entry, value: e.target.value })}
					/>
				)}

				{MiniEditor && showValue && (
					<button
						type="button"
						className={cn("btn", "btn-ghost", "header-row-expand")}
						onClick={() => setExpanded(!expanded)}
						aria-label={expanded ? "Collapse editor" : "Expand editor"}
					>
						{expanded ? "\u25B4" : "\u25BE"}
					</button>
				)}

				{onDelete && (
					<button
						type="button"
						className="btn btn-ghost header-row-delete"
						onClick={onDelete}
						aria-label="Delete header"
					>
						&#x2715;
					</button>
				)}

				{isOverridden && <span className="header-row-override-label">overridden</span>}
			</div>

			{expanded && MiniEditor && showValue && (
				<div className="header-row-editor">
					<MiniEditor value={entry.value} onChange={(v) => onChange({ ...entry, value: v })} />
				</div>
			)}
		</div>
	);
}
