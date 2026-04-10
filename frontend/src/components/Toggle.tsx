interface ToggleProps {
	checked: boolean;
	onChange: (checked: boolean) => void;
	disabled?: boolean;
	small?: boolean;
	title?: string;
	id?: string;
	"aria-label"?: string;
	stopPropagation?: boolean;
	/** Render as <span> instead of <label>. Use when Toggle is inside another <label>. */
	inline?: boolean;
}

export function Toggle({
	checked,
	onChange,
	disabled,
	small,
	title,
	id,
	"aria-label": ariaLabel,
	stopPropagation,
	inline,
}: ToggleProps) {
	const handleClick = stopPropagation ? (e: React.MouseEvent) => e.stopPropagation() : undefined;
	const handleKeyDown = stopPropagation
		? (e: React.KeyboardEvent) => e.stopPropagation()
		: undefined;

	const className = `toggle-switch${small ? " small" : ""}`;

	if (inline) {
		return (
			<span className={className} title={title}>
				<input
					type="checkbox"
					id={id}
					checked={checked}
					onChange={(e) => onChange(e.target.checked)}
					disabled={disabled}
					aria-label={ariaLabel}
				/>
				<span className="toggle-slider" />
			</span>
		);
	}

	return (
		<label className={className} title={title} onClick={handleClick} onKeyDown={handleKeyDown}>
			<input
				type="checkbox"
				id={id}
				checked={checked}
				onChange={(e) => onChange(e.target.checked)}
				disabled={disabled}
				aria-label={ariaLabel}
			/>
			<span className="toggle-slider" />
		</label>
	);
}
