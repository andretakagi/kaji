import { type ReactNode, useState } from "react";

interface Props {
	title: ReactNode;
	actions?: ReactNode;
	children: ReactNode;
	disabled?: boolean;
	defaultExpanded?: boolean;
	forceExpanded?: boolean;
	ariaLabel?: string;
}

export default function CollapsibleCard({
	title,
	actions,
	children,
	disabled,
	defaultExpanded = false,
	forceExpanded,
	ariaLabel,
}: Props) {
	const [expanded, setExpanded] = useState(defaultExpanded);
	const isExpanded = expanded || !!forceExpanded;

	return (
		<div className={`card ${disabled ? "card-disabled" : ""}`}>
			<div className="card-header">
				<button
					type="button"
					className="card-toggle"
					aria-expanded={isExpanded}
					aria-label={ariaLabel ?? (isExpanded ? "Collapse" : "Expand")}
					onClick={() => setExpanded(!isExpanded)}
				>
					<span className={`chevron ${isExpanded ? "open" : ""}`} />
					<div className="card-title">{title}</div>
				</button>
				{actions && <div className="card-actions">{actions}</div>}
			</div>
			{isExpanded && <div className="card-body">{children}</div>}
		</div>
	);
}
