import type React from "react";
import { useEffect, useState } from "react";
import { fetchTrustedProxies, updateTrustedProxies } from "../api";
import cloudflareRanges from "../data/cloudflare-ips.json";
import { useSettingsSection } from "../hooks/useSettingsSection";

const PRESETS: { label: string; ranges: string[] }[] = [
	{
		label: "Cloudflare",
		ranges: cloudflareRanges,
	},
	{
		label: "Private networks",
		ranges: ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
	},
];

function isValidIPv4(s: string): boolean {
	const parts = s.split(".");
	if (parts.length !== 4) return false;
	for (const part of parts) {
		if (!/^\d{1,3}$/.test(part)) return false;
		const n = Number(part);
		if (n < 0 || n > 255) return false;
		if (part.length > 1 && part[0] === "0") return false;
	}
	return true;
}

function isValidIPv6(s: string): boolean {
	if (s.includes(":::")) return false;
	const groups = s.split(":");
	if (groups.length < 2 || groups.length > 8) return false;
	let emptyCount = 0;
	for (const g of groups) {
		if (g === "") {
			emptyCount++;
			continue;
		}
		if (!/^[0-9a-fA-F]{1,4}$/.test(g)) return false;
	}
	if (emptyCount === 0 && groups.length !== 8) return false;
	if (emptyCount > 0 && groups.length > emptyCount + 1 && !s.includes("::")) return false;
	return true;
}

function isValidIPOrCIDR(value: string): boolean {
	const slash = value.indexOf("/");
	if (slash === -1) {
		return isValidIPv4(value) || isValidIPv6(value);
	}
	const ip = value.slice(0, slash);
	const prefixStr = value.slice(slash + 1);
	if (!/^\d{1,3}$/.test(prefixStr)) return false;
	const prefix = Number(prefixStr);
	if (isValidIPv4(ip)) return prefix >= 0 && prefix <= 32;
	if (isValidIPv6(ip)) return prefix >= 0 && prefix <= 128;
	return false;
}

export default function TrustedProxiesSection({
	onDirtyChange,
	saveRef,
	saving: externalSaving,
}: {
	onDirtyChange?: (dirty: boolean) => void;
	saveRef?: React.RefObject<(() => Promise<void>) | null>;
	saving?: boolean;
}) {
	const { values, setValues, dirty, loaded, load, markLoaded, save } = useSettingsSection({
		ranges: [] as string[],
	});
	const saving = externalSaving ?? false;
	const [input, setInput] = useState("");
	const [inputError, setInputError] = useState("");

	useEffect(() => {
		fetchTrustedProxies()
			.then((tp) => load({ ranges: tp.ranges }))
			.catch(markLoaded);
	}, [load, markLoaded]);

	useEffect(() => {
		onDirtyChange?.(dirty);
	}, [dirty, onDirtyChange]);

	useEffect(() => {
		if (!saveRef) return;
		saveRef.current = () =>
			new Promise<void>((resolve, reject) => {
				save(async (v) => {
					await updateTrustedProxies({ ranges: v.ranges });
					resolve();
					return undefined;
				}).catch(reject);
			});
	}, [save, saveRef]);

	if (!loaded) return null;

	const addRange = (value: string) => {
		const trimmed = value.trim();
		if (!trimmed) return;
		if (!isValidIPOrCIDR(trimmed)) {
			setInputError("Invalid IP or CIDR");
			return;
		}
		if (values.ranges.includes(trimmed)) {
			setInputError("Already added");
			return;
		}
		setValues((prev) => ({ ...prev, ranges: [...prev.ranges, trimmed] }));
		setInput("");
		setInputError("");
	};

	const removeRange = (index: number) => {
		setValues((prev) => ({
			...prev,
			ranges: prev.ranges.filter((_, i) => i !== index),
		}));
	};

	const addPreset = (ranges: string[]) => {
		setValues((prev) => {
			const merged = [...prev.ranges];
			for (const r of ranges) {
				if (!merged.includes(r)) merged.push(r);
			}
			return { ...prev, ranges: merged };
		});
		setInputError("");
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter") {
			e.preventDefault();
			addRange(input);
		}
	};

	return (
		<div className="advanced-subsection">
			<h4 className="settings-subsection-title">Trusted Proxies</h4>
			<p className="settings-description">
				IP ranges trusted to set X-Forwarded-For and other client identity headers. Required when
				running behind a load balancer or CDN.
			</p>

			<div className="settings-field">
				<label htmlFor="trusted-proxy-input">Add IP or CIDR range</label>
				<div className="trusted-proxies-input-row">
					<input
						id="trusted-proxy-input"
						type="text"
						value={input}
						onChange={(e) => {
							setInput(e.target.value);
							setInputError("");
						}}
						onKeyDown={handleKeyDown}
						placeholder="e.g. 10.0.0.0/8"
						disabled={saving}
						autoComplete="off"
					/>
					<button
						type="button"
						className="btn btn-primary"
						disabled={saving || !input.trim()}
						onClick={() => addRange(input)}
					>
						Add
					</button>
				</div>
				{inputError && <span className="settings-toggle-desc warning">{inputError}</span>}
			</div>

			<div className="trusted-proxies-presets">
				{PRESETS.map((p) => (
					<button
						key={p.label}
						type="button"
						className="btn btn-ghost"
						disabled={saving}
						onClick={() => addPreset(p.ranges)}
					>
						+ {p.label}
					</button>
				))}
			</div>

			{values.ranges.length > 0 && (
				<div className="trusted-proxies-tags">
					{values.ranges.map((range, i) => (
						<span key={range} className="trusted-proxy-tag">
							{range}
							<button
								type="button"
								className="trusted-proxy-tag-remove"
								onClick={() => removeRange(i)}
								disabled={saving}
								aria-label={`Remove ${range}`}
							>
								&times;
							</button>
						</span>
					))}
				</div>
			)}
		</div>
	);
}
