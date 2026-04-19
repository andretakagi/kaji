import { useId } from "react";
import { cn } from "../cn";
import { Toggle } from "./Toggle";

export function ToggleItem({
	label,
	description,
	checked,
	onChange,
	disabled,
}: {
	label: string;
	description: string;
	checked: boolean;
	onChange: (v: boolean) => void;
	disabled?: boolean;
}) {
	const id = useId();
	return (
		<label className={cn("toggle-item", disabled && "toggle-item-disabled")} htmlFor={id}>
			<div className="toggle-item-text">
				<span className="toggle-item-label">{label}</span>
				<span className="toggle-item-desc">{description}</span>
			</div>
			<Toggle inline small id={id} value={checked} onChange={onChange} disabled={disabled} />
		</label>
	);
}
