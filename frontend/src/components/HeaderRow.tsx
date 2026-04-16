import { useState } from "react";
import { cn } from "../cn";
import type { HeaderEntry } from "../types/api";
import { miniEditors } from "./MiniEditors";
import { Toggle } from "./Toggle";

interface HeaderRowProps {
	entry: HeaderEntry;
	isBuiltin: boolean;
	isOverridden: boolean;
	onChange: (entry: HeaderEntry) => void;
	onDelete?: () => void;
}

export function HeaderRow({ entry, isBuiltin, isOverridden, onChange, onDelete }: HeaderRowProps) {
	const [expanded, setExpanded] = useState(false);
	const MiniEditor = miniEditors[entry.key];

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

				<input
					type="text"
					className="header-row-value-input"
					placeholder="value"
					maxLength={4096}
					value={entry.value}
					onChange={(e) => onChange({ ...entry, value: e.target.value })}
				/>

				{MiniEditor && (
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

			{expanded && MiniEditor && (
				<div className="header-row-editor">
					<MiniEditor value={entry.value} onChange={(v) => onChange({ ...entry, value: v })} />
				</div>
			)}
		</div>
	);
}
