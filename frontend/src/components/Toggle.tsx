import { cn } from "../cn";

type OptionDef<T extends string> = T | { value: T; label: string };

interface BooleanToggleProps {
	value: boolean;
	onChange: (value: boolean) => void;
	disabled?: boolean;
	small?: boolean;
	title?: string;
	id?: string;
	"aria-label"?: string;
	stopPropagation?: boolean;
	inline?: boolean;
	options?: undefined;
}

interface SegmentedToggleProps<T extends string> {
	options: readonly OptionDef<T>[];
	value: T;
	onChange: (value: T) => void;
	disabled?: boolean;
	small?: boolean;
	id?: string;
	"aria-label"?: string;
}

export type ToggleProps<T extends string = string> = BooleanToggleProps | SegmentedToggleProps<T>;

function isBooleanToggle(props: ToggleProps<string>): props is BooleanToggleProps {
	return !("options" in props) || props.options === undefined;
}

function optionValue<T extends string>(opt: OptionDef<T>): T {
	return typeof opt === "string" ? opt : opt.value;
}

function optionLabel<T extends string>(opt: OptionDef<T>): string {
	return typeof opt === "string" ? opt : opt.label;
}

export function Toggle<T extends string = string>(props: ToggleProps<T>) {
	if (isBooleanToggle(props as ToggleProps<string>)) {
		return <BooleanSwitch {...(props as BooleanToggleProps)} />;
	}
	return <SegmentedControl {...(props as SegmentedToggleProps<T>)} />;
}

function BooleanSwitch({
	value,
	onChange,
	disabled,
	small,
	title,
	id,
	"aria-label": ariaLabel,
	stopPropagation,
	inline,
}: BooleanToggleProps) {
	const handleClick = stopPropagation ? (e: React.MouseEvent) => e.stopPropagation() : undefined;
	const handleKeyDown = stopPropagation
		? (e: React.KeyboardEvent) => e.stopPropagation()
		: undefined;

	const className = cn("toggle-switch", small && "small");

	if (inline) {
		return (
			<span className={className} title={title}>
				<input
					type="checkbox"
					id={id}
					checked={value}
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
				checked={value}
				onChange={(e) => onChange(e.target.checked)}
				disabled={disabled}
				aria-label={ariaLabel}
			/>
			<span className="toggle-slider" />
		</label>
	);
}

function SegmentedControl<T extends string>({
	options,
	value,
	onChange,
	disabled,
	id,
	"aria-label": ariaLabel,
}: SegmentedToggleProps<T>) {
	return (
		<fieldset
			className={cn("toggle-segmented", disabled && "toggle-segmented-disabled")}
			id={id}
			aria-label={ariaLabel}
			disabled={disabled}
		>
			{options.map((opt) => {
				const v = optionValue(opt);
				const active = v === value;
				return (
					<button
						key={v}
						type="button"
						aria-pressed={active}
						className={cn("toggle-segment", active && "active")}
						onClick={() => onChange(v)}
					>
						{optionLabel(opt)}
					</button>
				);
			})}
		</fieldset>
	);
}
