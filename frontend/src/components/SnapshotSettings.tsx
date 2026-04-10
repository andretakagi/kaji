import { useEffect, useState } from "react";
import { fetchSnapshots, updateSnapshotSettings } from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import Feedback from "./Feedback";
import { Toggle } from "./Toggle";

export function SnapshotSettings() {
	const [autoEnabled, setAutoEnabled] = useState(false);
	const [pruneLimit, setPruneLimit] = useState(10);
	const [savedAutoEnabled, setSavedAutoEnabled] = useState(false);
	const [savedPruneLimit, setSavedPruneLimit] = useState(10);
	const [loaded, setLoaded] = useState(false);
	const { saving, feedback, run } = useAsyncAction();

	useEffect(() => {
		fetchSnapshots()
			.then((data) => {
				setAutoEnabled(data.auto_snapshot_enabled);
				setPruneLimit(data.auto_snapshot_limit);
				setSavedAutoEnabled(data.auto_snapshot_enabled);
				setSavedPruneLimit(data.auto_snapshot_limit);
				setLoaded(true);
			})
			.catch(() => setLoaded(true));
	}, []);

	const dirty = autoEnabled !== savedAutoEnabled || pruneLimit !== savedPruneLimit;

	const handleSave = () =>
		run(async () => {
			await updateSnapshotSettings({
				auto_snapshot_enabled: autoEnabled,
				auto_snapshot_limit: pruneLimit,
			});
			setSavedAutoEnabled(autoEnabled);
			setSavedPruneLimit(pruneLimit);
			return "Saved";
		});

	if (!loaded) return null;

	return (
		<section className="settings-section">
			<h3>Snapshots</h3>
			<div className="settings-toggle-row">
				<span>Take snapshot before each config change</span>
				<Toggle
					checked={autoEnabled}
					onChange={() => setAutoEnabled((v) => !v)}
					disabled={saving}
				/>
			</div>
			{autoEnabled && (
				<div className="snapshot-settings-limit">
					<span>Keep last</span>
					<input
						type="number"
						min={1}
						max={100}
						value={pruneLimit}
						onChange={(e) =>
							setPruneLimit(Math.min(100, Math.max(1, Number.parseInt(e.target.value, 10) || 1)))
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
