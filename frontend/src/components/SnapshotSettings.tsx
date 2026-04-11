import { useEffect } from "react";
import { fetchSnapshots, updateSnapshotSettings } from "../api";
import { useSettingsSection } from "../hooks/useSettingsSection";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

export function SnapshotSettings() {
	const { values, setValues, dirty, loaded, load, markLoaded, save, saving, feedback } =
		useSettingsSection({ autoEnabled: false, pruneLimit: 10 });

	useEffect(() => {
		fetchSnapshots()
			.then((data) =>
				load({ autoEnabled: data.auto_snapshot_enabled, pruneLimit: data.auto_snapshot_limit }),
			)
			.catch(markLoaded);
	}, [load, markLoaded]);

	const handleSave = () =>
		save(async (v) => {
			await updateSnapshotSettings({
				auto_snapshot_enabled: v.autoEnabled,
				auto_snapshot_limit: v.pruneLimit,
			});
			return "Saved";
		});

	if (!loaded) return null;

	return (
		<section className="settings-section">
			<h3>Snapshots</h3>
			<div className="settings-toggle-row">
				<span>Take snapshot before each config change</span>
				<Toggle
					value={values.autoEnabled}
					onChange={() => setValues((v) => ({ ...v, autoEnabled: !v.autoEnabled }))}
					disabled={saving}
				/>
			</div>
			{values.autoEnabled && (
				<div className="snapshot-settings-limit">
					<span>Keep last</span>
					<input
						type="number"
						min={1}
						max={100}
						value={values.pruneLimit}
						onChange={(e) =>
							setValues((v) => ({
								...v,
								pruneLimit: Math.min(100, Math.max(1, Number.parseInt(e.target.value, 10) || 1)),
							}))
						}
						className="snapshot-limit-input"
						disabled={saving}
					/>
					<span>auto snapshots</span>
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
			<Feedback msg={feedback.msg} type={feedback.type} />
		</section>
	);
}
