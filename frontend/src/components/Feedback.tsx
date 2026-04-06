function Feedback({
	msg,
	type,
	className,
}: {
	msg: string;
	type: "success" | "error";
	className?: string;
}) {
	if (!msg) return null;
	return (
		<div
			className={`feedback ${type}${className ? ` ${className}` : ""}`}
			role={type === "error" ? "alert" : "status"}
		>
			{msg}
		</div>
	);
}

export default Feedback;
