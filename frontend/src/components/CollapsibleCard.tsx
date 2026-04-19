import { type ReactNode, useState } from "react";
import { cn } from "../cn";

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
		<div className={cn("card", disabled && "card-disabled")}>
			<div className="card-header">
				<button
					type="button"
					className="card-toggle"
					aria-expanded={isExpanded}
					aria-label={ariaLabel ?? (isExpanded ? "Collapse" : "Expand")}
					onClick={() => setExpanded(!isExpanded)}
				>
					<span className={cn("chevron", isExpanded && "open")} />
					<div className="card-title">{title}</div>
				</button>
				{actions && <div className="card-actions">{actions}</div>}
			</div>
			<div
				className={cn("card-body-wrapper", isExpanded && "expanded")}
				aria-hidden={!isExpanded || undefined}
				{...(disabled && !isExpanded ? { inert: true } : {})}
			>
				<div className="card-body">
					<div className="card-body-inner">{children}</div>
				</div>
			</div>
		</div>
	);
}
