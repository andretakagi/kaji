import { cn } from "../cn";

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
			key={`${type}:${msg}`}
			className={cn("feedback", type, className)}
			role={type === "error" ? "alert" : "status"}
		>
			{msg}
		</div>
	);
}

export default Feedback;
