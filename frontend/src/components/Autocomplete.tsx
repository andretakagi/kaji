import type React from "react";
import { useEffect, useMemo, useRef, useState } from "react";

interface AutocompleteProps {
	id?: string;
	value: string;
	onChange: (value: string) => void;
	options: string[];
	placeholder?: string;
	minChars?: number;
}

export default function Autocomplete({
	id,
	value,
	onChange,
	options,
	placeholder = "",
	minChars = 1,
}: AutocompleteProps) {
	const [open, setOpen] = useState(false);
	const [activeIndex, setActiveIndex] = useState(-1);
	const containerRef = useRef<HTMLDivElement>(null);
	const listRef = useRef<HTMLDivElement>(null);

	const filtered = useMemo(() => {
		if (!value) return options;
		if (value.length < minChars) return [];
		const lower = value.toLowerCase();
		return options.filter((o) => o.toLowerCase().includes(lower));
	}, [value, minChars, options]);

	const showDropdown = open && filtered.length > 0;

	useEffect(() => {
		const handleClickOutside = (e: MouseEvent) => {
			if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
				setOpen(false);
				setActiveIndex(-1);
			}
		};
		document.addEventListener("mousedown", handleClickOutside);
		return () => document.removeEventListener("mousedown", handleClickOutside);
	}, []);

	useEffect(() => {
		if (activeIndex >= 0 && listRef.current) {
			const items = listRef.current.querySelectorAll<HTMLElement>(".autocomplete-item");
			items[activeIndex]?.scrollIntoView({ block: "nearest" });
		}
	}, [activeIndex]);

	function handleSelect(option: string) {
		onChange(option);
		setOpen(false);
		setActiveIndex(-1);
	}

	function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
		if (!showDropdown) return;

		if (e.key === "ArrowDown") {
			e.preventDefault();
			setActiveIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
		} else if (e.key === "ArrowUp") {
			e.preventDefault();
			setActiveIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
		} else if (e.key === "Enter" && activeIndex >= 0) {
			e.preventDefault();
			handleSelect(filtered[activeIndex]);
		} else if (e.key === "Escape") {
			setOpen(false);
			setActiveIndex(-1);
		}
	}

	return (
		<div className="autocomplete" ref={containerRef}>
			<input
				id={id}
				type="text"
				value={value}
				onChange={(e) => {
					onChange(e.target.value);
					setOpen(true);
					setActiveIndex(-1);
				}}
				onFocus={() => setOpen(true)}
				onKeyDown={handleKeyDown}
				placeholder={placeholder}
				autoComplete="off"
				role="combobox"
				aria-expanded={showDropdown}
				aria-autocomplete="list"
				aria-activedescendant={
					showDropdown && activeIndex >= 0 ? `autocomplete-opt-${activeIndex}` : undefined
				}
			/>
			{value && (
				<button
					type="button"
					className="autocomplete-clear"
					onClick={() => {
						onChange("");
						setOpen(false);
						setActiveIndex(-1);
					}}
					aria-label="Clear filter"
				>
					&#215;
				</button>
			)}
			{showDropdown && (
				<div className="autocomplete-dropdown" role="listbox" ref={listRef}>
					{filtered.map((option, i) => (
						<div
							key={option}
							id={`autocomplete-opt-${i}`}
							role="option"
							tabIndex={-1}
							aria-selected={i === activeIndex}
							className={`autocomplete-item${i === activeIndex ? " active" : ""}`}
							onClick={() => handleSelect(option)}
							onKeyDown={(e) => {
								if (e.key === "Enter" || e.key === " ") {
									e.preventDefault();
									handleSelect(option);
								}
							}}
						>
							{option}
						</div>
					))}
				</div>
			)}
		</div>
	);
}
