interface SectionHeaderProps {
	title: string;
	children?: React.ReactNode;
}

export function SectionHeader({ title, children }: SectionHeaderProps) {
	return (
		<div className="section-header">
			<h2>{title}</h2>
			{children}
		</div>
	);
}
