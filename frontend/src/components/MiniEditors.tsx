import type { ComponentType } from "react";
import { Toggle } from "./Toggle";

interface MiniEditorProps {
	value: string;
	onChange: (value: string) => void;
}

function HSTSEditor({ value, onChange }: MiniEditorProps) {
	const lower = value.toLowerCase();
	const maxAgeMatch = lower.match(/max-age=(\d+)/);
	const maxAge = maxAgeMatch ? Number.parseInt(maxAgeMatch[1], 10) : 31536000;
	const includeSubDomains = lower.includes("includesubdomains");
	const preload = lower.includes("preload");

	function rebuild(age: number, sub: boolean, pre: boolean) {
		const parts = [`max-age=${age}`];
		if (sub) parts.push("includeSubDomains");
		if (pre) parts.push("preload");
		onChange(parts.join("; "));
	}

	return (
		<div className="mini-editor mini-editor-hsts">
			<label className="mini-editor-field">
				<span>max-age (seconds)</span>
				<input
					type="number"
					min={0}
					value={maxAge}
					onChange={(e) => {
						const n = Math.max(0, Number.parseInt(e.target.value, 10) || 0);
						rebuild(n, includeSubDomains, preload);
					}}
				/>
			</label>
			<label className="mini-editor-field mini-editor-checkbox">
				<input
					type="checkbox"
					checked={includeSubDomains}
					onChange={(e) => rebuild(maxAge, e.target.checked, preload)}
				/>
				<span>includeSubDomains</span>
			</label>
			<label className="mini-editor-field mini-editor-checkbox">
				<input
					type="checkbox"
					checked={preload}
					onChange={(e) => rebuild(maxAge, includeSubDomains, e.target.checked)}
				/>
				<span>preload</span>
			</label>
		</div>
	);
}

function XFrameOptionsEditor({ value, onChange }: MiniEditorProps) {
	const current = value.toUpperCase() === "SAMEORIGIN" ? "SAMEORIGIN" : "DENY";

	return (
		<div className="mini-editor mini-editor-x-frame">
			<Toggle options={["DENY", "SAMEORIGIN"] as const} value={current} onChange={onChange} small />
		</div>
	);
}

const referrerPolicies = [
	"no-referrer",
	"no-referrer-when-downgrade",
	"origin",
	"origin-when-cross-origin",
	"same-origin",
	"strict-origin",
	"strict-origin-when-cross-origin",
	"unsafe-url",
] as const;

function ReferrerPolicyEditor({ value, onChange }: MiniEditorProps) {
	return (
		<div className="mini-editor mini-editor-referrer">
			<select value={value} onChange={(e) => onChange(e.target.value)}>
				{referrerPolicies.map((p) => (
					<option key={p} value={p}>
						{p}
					</option>
				))}
			</select>
		</div>
	);
}

const cacheDirectives = ["no-cache", "no-store", "public", "private", "must-revalidate"] as const;

function CacheControlEditor({ value, onChange }: MiniEditorProps) {
	const parts = value
		.split(",")
		.map((s) => s.trim().toLowerCase())
		.filter(Boolean);

	const active = new Set(parts.filter((p) => !p.startsWith("max-age=")));
	const maxAgePart = parts.find((p) => p.startsWith("max-age="));
	const maxAge = maxAgePart ? Number.parseInt(maxAgePart.split("=")[1], 10) : null;

	function rebuild(directives: Set<string>, age: number | null) {
		const result = [...directives];
		if (age !== null) result.push(`max-age=${age}`);
		onChange(result.join(", "));
	}

	function toggleDirective(dir: string, on: boolean) {
		const next = new Set(active);
		if (on) next.add(dir);
		else next.delete(dir);
		rebuild(next, maxAge);
	}

	return (
		<div className="mini-editor mini-editor-cache">
			{cacheDirectives.map((dir) => (
				<label key={dir} className="mini-editor-field mini-editor-checkbox">
					<input
						type="checkbox"
						checked={active.has(dir)}
						onChange={(e) => toggleDirective(dir, e.target.checked)}
					/>
					<span>{dir}</span>
				</label>
			))}
			<label className="mini-editor-field">
				<span>max-age (seconds)</span>
				<input
					type="number"
					min={0}
					placeholder="not set"
					value={maxAge ?? ""}
					onChange={(e) => {
						const raw = e.target.value;
						if (raw === "") {
							rebuild(active, null);
						} else {
							rebuild(active, Math.max(0, Number.parseInt(raw, 10) || 0));
						}
					}}
				/>
			</label>
		</div>
	);
}

function XRobotsTagEditor({ value, onChange }: MiniEditorProps) {
	const parts = value
		.split(",")
		.map((s) => s.trim().toLowerCase())
		.filter(Boolean);
	const active = new Set(parts);
	const directives = ["noindex", "nofollow", "noarchive"] as const;

	function toggle(dir: string, on: boolean) {
		const next = new Set(active);
		if (on) next.add(dir);
		else next.delete(dir);
		onChange([...next].join(", "));
	}

	return (
		<div className="mini-editor mini-editor-robots">
			{directives.map((dir) => (
				<label key={dir} className="mini-editor-field mini-editor-checkbox">
					<input
						type="checkbox"
						checked={active.has(dir)}
						onChange={(e) => toggle(dir, e.target.checked)}
					/>
					<span>{dir}</span>
				</label>
			))}
		</div>
	);
}

export const miniEditors: Record<
	string,
	ComponentType<{ value: string; onChange: (v: string) => void }>
> = {
	"Strict-Transport-Security": HSTSEditor,
	"X-Frame-Options": XFrameOptionsEditor,
	"Referrer-Policy": ReferrerPolicyEditor,
	"Cache-Control": CacheControlEditor,
	"X-Robots-Tag": XRobotsTagEditor,
};
