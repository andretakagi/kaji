import { type ReactNode, useState } from "react";

interface Props {
	title: ReactNode;
	actions?: ReactNode;
	children: ReactNode;
	disabled?: boolean;
	defaultExpanded?: boolean;
	ariaLabel?: string;
}

export default function CollapsibleCard({
	title,
	actions,
	children,
	disabled,
	defaultExpanded = false,
	ariaLabel,
}: Props) {
	const [expanded, setExpanded] = useState(defaultExpanded);

	return (
		<div className={`card ${disabled ? "card-disabled" : ""}`}>
			<div className="card-header">
				<button
					type="button"
					className="card-toggle"
					aria-expanded={expanded}
					aria-label={ariaLabel ?? (expanded ? "Collapse" : "Expand")}
					onClick={() => setExpanded(!expanded)}
				>
					<span className={`chevron ${expanded ? "open" : ""}`} />
					<div className="card-title">{title}</div>
				</button>
				{actions && <div className="card-actions">{actions}</div>}
			</div>
			{expanded && <div className="card-body">{children}</div>}
		</div>
	);
}
