import { memo, useRef, useState } from "react";
import { updateLogSkipRules } from "../api";
import type { Feedback } from "../hooks/useAsyncAction";
import type { LogSkipConfig, SkipCondition } from "../types/logs";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import { Toggle } from "./Toggle";

type KeyedCondition = SkipCondition & { _key: number };

interface Props {
	sinkName: string;
	initialRules: LogSkipConfig;
	onSaved?: () => void;
}

type CaddyMatcher = Record<string, unknown>;

function conditionsToCaddyMatchers(conditions: SkipCondition[]): CaddyMatcher[] {
	return conditions.map((c) => {
		if (c.type === "path") {
			return { path: [c.value] };
		}
		if (c.type === "path_regexp") {
			return { path_regexp: { pattern: c.value } };
		}
		if (c.type === "header") {
			return { header: { [c.key ?? ""]: [c.value] } };
		}
		// remote_ip
		return { remote_ip: { ranges: [c.value] } };
	});
}

function isCaddyMatcher(v: unknown): v is CaddyMatcher {
	return typeof v === "object" && v !== null && !Array.isArray(v);
}

function caddyMatchersToConditions(matchers: unknown[]): {
	conditions: SkipCondition[];
	dropped: string[];
} {
	const conditions: SkipCondition[] = [];
	const dropped: string[] = [];

	for (const m of matchers) {
		if (!isCaddyMatcher(m)) {
			dropped.push(JSON.stringify(m));
			continue;
		}

		if ("path" in m && Array.isArray(m.path)) {
			for (const v of m.path) {
				conditions.push({ type: "path", value: String(v) });
			}
			continue;
		}

		if ("path_regexp" in m && isCaddyMatcher(m.path_regexp)) {
			const pattern = m.path_regexp.pattern;
			conditions.push({ type: "path_regexp", value: typeof pattern === "string" ? pattern : "" });
			continue;
		}

		if ("header" in m && isCaddyMatcher(m.header)) {
			const entries = Object.entries(m.header);
			for (const [key, vals] of entries) {
				if (Array.isArray(vals)) {
					for (const v of vals) {
						conditions.push({ type: "header", key, value: String(v) });
					}
				} else {
					conditions.push({ type: "header", key, value: "" });
				}
			}
			continue;
		}

		if ("remote_ip" in m && isCaddyMatcher(m.remote_ip) && Array.isArray(m.remote_ip.ranges)) {
			for (const r of m.remote_ip.ranges) {
				conditions.push({ type: "remote_ip", value: String(r) });
			}
			continue;
		}

		dropped.push(JSON.stringify(m));
	}

	return { conditions, dropped };
}

function matchersFromText(text: string): { matchers: unknown[]; error: string } {
	let parsed: unknown;
	try {
		parsed = JSON.parse(text);
	} catch {
		return { matchers: [], error: "Invalid JSON" };
	}
	if (!Array.isArray(parsed)) {
		return { matchers: [], error: "Expected a JSON array of matchers" };
	}
	return { matchers: parsed, error: "" };
}

export const LogSkipRulesEditor = memo(function LogSkipRulesEditor({
	sinkName,
	initialRules,
	onSaved,
}: Props) {
	const nextKey = useRef(0);
	function assignKey(c: SkipCondition): KeyedCondition {
		return { ...c, _key: nextKey.current++ };
	}
	function assignKeys(cs: SkipCondition[]): KeyedCondition[] {
		return cs.map(assignKey);
	}

	const [rules, setRules] = useState<LogSkipConfig>(() => ({
		...initialRules,
		conditions: initialRules.conditions ?? [],
	}));
	const [keyedConditions, setKeyedConditions] = useState<KeyedCondition[]>(() =>
		assignKeys(initialRules.conditions ?? []),
	);
	const [advancedText, setAdvancedText] = useState(() => {
		if (initialRules.mode === "advanced" && initialRules.advanced_raw) {
			return JSON.stringify(initialRules.advanced_raw, null, 2);
		}
		return "";
	});
	const [saving, setSaving] = useState(false);
	const [feedback, setFeedback] = useState<Feedback | null>(null);
	const [confirmSwitch, setConfirmSwitch] = useState<{ dropped: string[] } | null>(null);
	const [savedCount, setSavedCount] = useState(() => {
		if (initialRules.mode === "advanced") return initialRules.advanced_raw?.length ?? 0;
		return initialRules.conditions?.length ?? 0;
	});

	const isAdvanced = rules.mode === "advanced";

	function addCondition() {
		const next: SkipCondition = { type: "path", value: "" };
		setRules((r) => ({ ...r, conditions: [...(r.conditions ?? []), next] }));
		setKeyedConditions((prev) => [...prev, assignKey(next)]);
	}

	function removeCondition(index: number) {
		setRules((r) => ({
			...r,
			conditions: (r.conditions ?? []).filter((_, j) => j !== index),
		}));
		setKeyedConditions((prev) => prev.filter((_, j) => j !== index));
	}

	function mergeCondition(current: SkipCondition, updates: Partial<SkipCondition>): SkipCondition {
		const typeChanging = updates.type !== undefined && updates.type !== current.type;
		if (typeChanging) {
			if (updates.type === "header") {
				return { type: "header", key: "", value: current.value };
			}
			const { key: _key, ...rest } = current;
			return { ...rest, ...updates } as SkipCondition;
		}
		return { ...current, ...updates } as SkipCondition;
	}

	function updateCondition(index: number, updates: Partial<SkipCondition>) {
		setRules((r) => {
			const conditions = [...(r.conditions ?? [])];
			const current = conditions[index];
			if (!current) return r;
			conditions[index] = mergeCondition(current, updates);
			return { ...r, conditions };
		});
		setKeyedConditions((prev) => {
			const next = [...prev];
			const current = next[index];
			if (!current) return prev;
			next[index] = { ...mergeCondition(current, updates), _key: current._key };
			return next;
		});
	}

	function switchToAdvanced() {
		const matchers = conditionsToCaddyMatchers(rules.conditions ?? []);
		const text = JSON.stringify(matchers, null, 2);
		setAdvancedText(text);
		setRules((r) => ({
			...r,
			mode: "advanced",
			advanced_raw: matchers,
		}));
		setFeedback(null);
	}

	function requestSwitchToBasic() {
		const { matchers, error } = matchersFromText(advancedText);
		if (error) {
			setFeedback({ msg: error, type: "error" });
			return;
		}
		const { conditions, dropped } = caddyMatchersToConditions(matchers);
		if (dropped.length > 0) {
			setConfirmSwitch({ dropped });
			return;
		}
		applyBasicSwitch(conditions);
	}

	function applyBasicSwitch(conditions: SkipCondition[]) {
		setRules({ mode: "basic", conditions, advanced_raw: null });
		setKeyedConditions(assignKeys(conditions));
		setConfirmSwitch(null);
		setFeedback(null);
	}

	async function handleSave() {
		let payload: LogSkipConfig;
		if (isAdvanced) {
			const { matchers, error } = matchersFromText(advancedText);
			if (error) {
				setFeedback({ msg: error, type: "error" });
				return;
			}
			payload = { mode: "advanced", conditions: [], advanced_raw: matchers };
		} else {
			payload = { ...rules, advanced_raw: null };
		}

		setSaving(true);
		setFeedback(null);
		try {
			const saved = await updateLogSkipRules(sinkName, payload);
			const savedConditions = saved.conditions ?? [];
			setRules({ ...saved, conditions: savedConditions });
			setKeyedConditions(assignKeys(savedConditions));
			if (saved.mode === "advanced" && saved.advanced_raw) {
				setAdvancedText(JSON.stringify(saved.advanced_raw, null, 2));
			}
			const nextCount =
				saved.mode === "advanced" ? (saved.advanced_raw?.length ?? 0) : savedConditions.length;
			setSavedCount(nextCount);
			setFeedback({ msg: "Saved", type: "success" });
			setTimeout(() => setFeedback(null), 3000);
			onSaved?.();
		} catch (err) {
			setFeedback({ msg: getErrorMessage(err, "Failed to save"), type: "error" });
		} finally {
			setSaving(false);
		}
	}

	const title = (
		<>
			Log Skip Rules
			{savedCount > 0 && <span className="log-skip-rules-badge">{savedCount}</span>}
		</>
	);

	return (
		<div className="log-skip-rules">
			<CollapsibleCard title={title} ariaLabel="Log Skip Rules">
				<div className="log-skip-rules-body">
					{confirmSwitch && (
						<div className="log-skip-rules-confirm">
							<p>The following matchers cannot be represented in basic mode and will be removed:</p>
							<ul>
								{confirmSwitch.dropped.map((d) => (
									<li key={d}>
										<code>{d}</code>
									</li>
								))}
							</ul>
							<div className="log-skip-rules-confirm-actions">
								<button
									type="button"
									className="btn btn-danger btn-sm"
									onClick={() => {
										const { matchers } = matchersFromText(advancedText);
										const { conditions: parsed } = caddyMatchersToConditions(matchers);
										applyBasicSwitch(parsed);
									}}
								>
									Drop and switch
								</button>
								<button
									type="button"
									className="btn btn-ghost btn-sm"
									onClick={() => setConfirmSwitch(null)}
								>
									Cancel
								</button>
							</div>
						</div>
					)}

					<div className="log-skip-rules-mode-toggle">
						<Toggle<"basic" | "advanced">
							options={["basic", "advanced"] as const}
							value={isAdvanced ? "advanced" : "basic"}
							onChange={(v: "basic" | "advanced") => {
								if (v === "advanced") switchToAdvanced();
								else requestSwitchToBasic();
							}}
							aria-label="Skip rules mode"
						/>
					</div>

					{isAdvanced ? (
						<textarea
							className="log-skip-advanced-editor"
							value={advancedText}
							onChange={(e) => setAdvancedText(e.target.value)}
							rows={8}
							spellCheck={false}
							placeholder="[]"
						/>
					) : (
						<>
							{keyedConditions.length === 0 ? (
								<p className="log-skip-rules-empty">No skip rules configured.</p>
							) : (
								<div className="log-skip-condition-list">
									{keyedConditions.map((cond, i) => (
										<div key={cond._key} className="log-skip-condition-row">
											<select
												value={cond.type}
												onChange={(e) =>
													updateCondition(i, {
														type: e.target.value as SkipCondition["type"],
													})
												}
											>
												<option value="path">path</option>
												<option value="path_regexp">path regexp</option>
												<option value="header">header</option>
												<option value="remote_ip">remote ip</option>
											</select>
											{cond.type === "header" && (
												<input
													type="text"
													placeholder="Header name"
													value={cond.key ?? ""}
													onChange={(e) => updateCondition(i, { key: e.target.value })}
												/>
											)}
											<input
												type="text"
												placeholder={
													cond.type === "path"
														? "/healthz"
														: cond.type === "path_regexp"
															? "^/static/"
															: cond.type === "remote_ip"
																? "192.168.0.0/24"
																: "Header value"
												}
												value={cond.value}
												onChange={(e) => updateCondition(i, { value: e.target.value })}
											/>
											<button
												type="button"
												className="btn btn-ghost btn-sm log-skip-remove-btn"
												aria-label="Remove condition"
												onClick={() => removeCondition(i)}
											>
												&times;
											</button>
										</div>
									))}
								</div>
							)}
							<button type="button" className="btn btn-ghost btn-sm" onClick={addCondition}>
								+ Add condition
							</button>
						</>
					)}

					<div className="log-skip-rules-footer">
						<button
							type="button"
							className="btn btn-primary log-config-save-btn"
							disabled={saving}
							onClick={handleSave}
						>
							{saving ? "Saving..." : "Save"}
						</button>
						{feedback && (
							<span className={`feedback log-config-feedback ${feedback.type}`}>
								{feedback.msg}
							</span>
						)}
					</div>
				</div>
			</CollapsibleCard>
		</div>
	);
});
