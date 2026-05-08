import { useEffect, useState } from "react";
import { fetchTrustedProxies, updateTrustedProxies } from "../api";
import { useSettingsSection } from "../hooks/useSettingsSection";
import Feedback from "./Feedback";

const PRESETS: { label: string; ranges: string[] }[] = [
	{
		label: "Cloudflare",
		ranges: [
			"173.245.48.0/20",
			"103.21.244.0/22",
			"103.22.200.0/22",
			"103.31.4.0/22",
			"141.101.64.0/18",
			"108.162.192.0/18",
			"190.93.240.0/20",
			"188.114.96.0/20",
			"197.234.240.0/22",
			"198.41.128.0/17",
			"162.158.0.0/15",
			"104.16.0.0/13",
			"104.24.0.0/14",
			"172.64.0.0/13",
			"131.0.72.0/22",
			"2400:cb00::/32",
			"2606:4700::/32",
			"2803:f800::/32",
			"2405:b500::/32",
			"2405:8100::/32",
			"2a06:98c0::/29",
			"2c0f:f248::/32",
		],
	},
	{
		label: "Private networks",
		ranges: ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"],
	},
];

function isValidIPOrCIDR(value: string): boolean {
	const cidrV4 = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;
	const cidrV6 = /^[0-9a-fA-F:]+\/\d{1,3}$/;
	const ipV4 = /^(\d{1,3}\.){3}\d{1,3}$/;
	const ipV6 = /^[0-9a-fA-F:]+$/;
	return cidrV4.test(value) || cidrV6.test(value) || ipV4.test(value) || ipV6.test(value);
}

export default function TrustedProxiesSection() {
	const { values, setValues, dirty, loaded, load, save, saving, feedback } = useSettingsSection({
		ranges: [] as string[],
	});
	const [input, setInput] = useState("");
	const [inputError, setInputError] = useState("");

	useEffect(() => {
		fetchTrustedProxies().then((tp) => load({ ranges: tp.ranges }));
	}, [load]);

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

	const handleSave = () => {
		save(async (v) => {
			await updateTrustedProxies({ ranges: v.ranges });
			return "Saved";
		});
	};

	return (
		<section className="settings-section">
			<h3>Trusted Proxies</h3>
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

			{dirty && (
				<button
					type="button"
					className="btn btn-primary settings-save-btn"
					disabled={saving}
					onClick={handleSave}
				>
					{saving ? "Saving..." : "Save"}
				</button>
			)}
			<Feedback msg={feedback.msg} type={feedback.type} className="settings-feedback" />
		</section>
	);
}
