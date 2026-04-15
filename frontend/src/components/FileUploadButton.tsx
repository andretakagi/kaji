import { useRef } from "react";

export default function FileUploadButton({
	accept,
	disabled,
	className = "btn btn-primary",
	children,
	onChange,
}: {
	accept?: string;
	disabled?: boolean;
	className?: string;
	children: React.ReactNode;
	onChange: (file: File) => void;
}) {
	const inputRef = useRef<HTMLInputElement>(null);

	const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
		const file = e.target.files?.[0];
		if (!file) return;
		onChange(file);
		e.target.value = "";
	};

	return (
		<>
			<input ref={inputRef} type="file" accept={accept} onChange={handleChange} hidden />
			<button
				type="button"
				className={className}
				disabled={disabled}
				onClick={() => inputRef.current?.click()}
			>
				{children}
			</button>
		</>
	);
}
