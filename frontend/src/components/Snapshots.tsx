import { useCallback, useEffect, useState } from "react";
import {
	createSnapshot,
	deleteSnapshot,
	fetchSnapshots,
	restoreSnapshot,
	updateSnapshot,
	updateSnapshotSettings,
} from "../api";
import { useAsyncAction } from "../hooks/useAsyncAction";
import type { Snapshot, SnapshotIndex } from "../types/snapshots";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import Feedback from "./Feedback";

interface GraphColumn {
	key: string;
	index: number;
	active: boolean;
	hasDot: boolean;
}

interface LayoutNode {
	snapshot: Snapshot;
	column: number;
	graphColumns: GraphColumn[];
	forkColumn: number;
}

function relativeTime(iso: string): string {
	const now = Date.now();
	const then = new Date(iso).getTime();
	const diffMs = now - then;
	if (diffMs < 0) return "just now";
	const seconds = Math.floor(diffMs / 1000);
	if (seconds < 60) return "just now";
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	if (days < 30) return `${days}d ago`;
	const months = Math.floor(days / 30);
	return `${months}mo ago`;
}

function defaultSnapshotName(): string {
	const d = new Date();
	const pad = (n: number) => String(n).padStart(2, "0");
	return `snapshot-${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

function buildLayout(snapshots: Snapshot[], currentId: string): LayoutNode[] {
	if (snapshots.length === 0) return [];

	const byId = new Map<string, Snapshot>();
	for (const s of snapshots) byId.set(s.id, s);

	// Build the main path from root to current
	const mainPath = new Set<string>();
	let walk = currentId;
	while (walk) {
		mainPath.add(walk);
		const snap = byId.get(walk);
		if (!snap?.parent_id || snap.parent_id === walk) break;
		walk = snap.parent_id;
	}

	// Sort newest first
	const sorted = [...snapshots].sort(
		(a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
	);

	// Track which branches are active at each row. Column 0 = main path,
	// column 1+ = branches. We only need column 0 and 1 for a simple view.
	const maxCols = 2;
	const nodes: LayoutNode[] = [];

	// Figure out which snapshots are branch points (have children not on main path)
	const childrenOf = new Map<string, string[]>();
	for (const s of snapshots) {
		if (s.parent_id) {
			const list = childrenOf.get(s.parent_id) ?? [];
			list.push(s.id);
			childrenOf.set(s.parent_id, list);
		}
	}

	// For branch nodes, track the parent where the branch forks
	const branchForkParent = new Map<string, string>();
	for (const s of sorted) {
		if (!mainPath.has(s.id) && s.parent_id && mainPath.has(s.parent_id)) {
			branchForkParent.set(s.id, s.parent_id);
		}
	}

	// Walk through sorted snapshots and assign columns
	// Active columns track which vertical lines are drawn
	let branchActive = false;
	let branchLastSeen = "";

	for (const s of sorted) {
		const onMain = mainPath.has(s.id);
		const col = onMain ? 0 : 1;
		let forkCol = -1;

		if (!onMain) {
			branchActive = true;
			branchLastSeen = s.id;
			if (branchForkParent.has(s.id)) {
				forkCol = 0;
			}
		}

		// Determine active columns for line drawing
		const active = [false, false];
		active[0] = true; // main path line always active

		if (!onMain) {
			active[1] = true;
		} else if (branchActive) {
			// Check if there are still branch nodes below us (newer, already processed)
			// Branch line is active if the last branch node was already placed
			// and this main node is the fork parent
			const isForkParent = [...branchForkParent.values()].includes(s.id);
			if (isForkParent) {
				active[1] = true;
				branchActive = false;
			} else if (branchActive && branchLastSeen) {
				active[1] = true;
			}
		}

		const graphColumns: GraphColumn[] = active.slice(0, maxCols).map((isActive, i) => ({
			key: `${s.id}-col${i}`,
			index: i,
			active: isActive,
			hasDot: isActive && i === col,
		}));

		nodes.push({
			snapshot: s,
			column: col,
			graphColumns,
			forkColumn: forkCol,
		});
	}

	return nodes;
}

export default function Snapshots() {
	const [index, setIndex] = useState<SnapshotIndex | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");

	// Settings
	const [autoEnabled, setAutoEnabled] = useState(false);
	const [pruneLimit, setPruneLimit] = useState(10);
	const settingsAction = useAsyncAction();

	// Create form
	const [showCreate, setShowCreate] = useState(false);
	const [createName, setCreateName] = useState("");
	const [createDesc, setCreateDesc] = useState("");
	const createAction = useAsyncAction();

	// Edit
	const [editingId, setEditingId] = useState<string | null>(null);
	const [editName, setEditName] = useState("");
	const [editDesc, setEditDesc] = useState("");
	const editAction = useAsyncAction();

	// Restore
	const [confirmRestoreId, setConfirmRestoreId] = useState<string | null>(null);
	const restoreAction = useAsyncAction();

	// Delete
	const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
	const deleteAction = useAsyncAction();

	const load = useCallback(async () => {
		setLoading(true);
		setError("");
		try {
			const data = await fetchSnapshots();
			setIndex(data);
			setAutoEnabled(data.auto_snapshot_enabled);
			setPruneLimit(data.auto_snapshot_limit);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load snapshots"));
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		load();
	}, [load]);

	const handleSaveSettings = () =>
		settingsAction.run(async () => {
			await updateSnapshotSettings({
				auto_snapshot_enabled: autoEnabled,
				auto_snapshot_limit: pruneLimit,
			});
			return "Saved";
		});

	const handleCreate = () =>
		createAction.run(async () => {
			await createSnapshot(createName, createDesc);
			setShowCreate(false);
			setCreateName("");
			setCreateDesc("");
			await load();
			return "Snapshot created";
		});

	const handleRestore = (id: string) =>
		restoreAction.run(async () => {
			await restoreSnapshot(id);
			setConfirmRestoreId(null);
			await load();
			return "Snapshot restored";
		});

	const handleEdit = (id: string) =>
		editAction.run(async () => {
			await updateSnapshot(id, editName, editDesc);
			setEditingId(null);
			await load();
			return "Snapshot updated";
		});

	const handleDelete = (id: string) =>
		deleteAction.run(async () => {
			await deleteSnapshot(id);
			setConfirmDeleteId(null);
			await load();
			return "Snapshot deleted";
		});

	const openCreate = () => {
		setCreateName(defaultSnapshotName());
		setCreateDesc("");
		setShowCreate(true);
		createAction.setFeedback({ msg: "", type: "success" });
	};

	const openEdit = (snap: Snapshot) => {
		setEditingId(snap.id);
		setEditName(snap.name);
		setEditDesc(snap.description);
		editAction.setFeedback({ msg: "", type: "success" });
	};

	if (loading) {
		return <div className="empty-state">Loading snapshots...</div>;
	}

	if (error) {
		return (
			<div className="empty-state">
				<p>{error}</p>
				<button
					type="button"
					className="btn btn-primary"
					onClick={load}
					style={{ marginTop: "0.75rem" }}
				>
					Retry
				</button>
			</div>
		);
	}

	const snapshots = index?.snapshots ?? [];
	const currentId = index?.current_id ?? "";
	const layout = buildLayout(snapshots, currentId);
	const maxCols = layout.length > 0 ? Math.max(...layout.map((n) => n.graphColumns.length)) : 1;

	return (
		<div className="snapshots">
			<CollapsibleCard title="Snapshot Settings">
				<div className="snapshot-settings">
					<div className="settings-toggle-row">
						<span>Take snapshot before each config change</span>
						<label className="toggle-switch">
							<input
								type="checkbox"
								checked={autoEnabled}
								onChange={() => setAutoEnabled((v) => !v)}
								disabled={settingsAction.saving}
							/>
							<span className="toggle-slider" />
						</label>
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
									setPruneLimit(Math.max(1, Number.parseInt(e.target.value, 10) || 1))
								}
								className="snapshot-limit-input"
								disabled={settingsAction.saving}
							/>
							<span>auto snapshots</span>
						</div>
					)}
					<button
						type="button"
						className="btn btn-primary settings-save-btn"
						disabled={settingsAction.saving}
						onClick={handleSaveSettings}
					>
						{settingsAction.saving ? "Saving..." : "Save"}
					</button>
					<Feedback msg={settingsAction.feedback.msg} type={settingsAction.feedback.type} />
				</div>
			</CollapsibleCard>

			<div className="snapshot-action-bar">
				{showCreate ? (
					<div className="snapshot-create-form">
						<div className="settings-field">
							<label htmlFor="snapshot-name">Name</label>
							<input
								id="snapshot-name"
								type="text"
								value={createName}
								onChange={(e) => setCreateName(e.target.value)}
								placeholder="Snapshot name"
								disabled={createAction.saving}
							/>
						</div>
						<div className="settings-field">
							<label htmlFor="snapshot-desc">Description</label>
							<textarea
								id="snapshot-desc"
								value={createDesc}
								onChange={(e) => setCreateDesc(e.target.value)}
								placeholder="Optional description"
								rows={2}
								disabled={createAction.saving}
							/>
						</div>
						<div className="snapshot-create-actions">
							<button
								type="button"
								className="btn btn-primary"
								disabled={createAction.saving || !createName.trim()}
								onClick={handleCreate}
							>
								{createAction.saving ? "Creating..." : "Create"}
							</button>
							<button
								type="button"
								className="btn btn-ghost"
								disabled={createAction.saving}
								onClick={() => setShowCreate(false)}
							>
								Cancel
							</button>
						</div>
						<Feedback msg={createAction.feedback.msg} type={createAction.feedback.type} />
					</div>
				) : (
					<button type="button" className="btn btn-primary" onClick={openCreate}>
						Take Snapshot
					</button>
				)}
			</div>

			<Feedback msg={restoreAction.feedback.msg} type={restoreAction.feedback.type} />
			<Feedback msg={deleteAction.feedback.msg} type={deleteAction.feedback.type} />

			{snapshots.length === 0 ? (
				<div className="empty-state">
					No snapshots yet. Take one to save the current configuration.
				</div>
			) : (
				<div className="snapshot-timeline">
					{layout.map((node) => {
						const s = node.snapshot;
						const isCurrent = s.id === currentId;
						const isEditing = editingId === s.id;
						const isConfirmingRestore = confirmRestoreId === s.id;
						const isConfirmingDelete = confirmDeleteId === s.id;
						const graphWidth = maxCols * 28;

						return (
							<div className="snapshot-row" key={s.id}>
								<div className="snapshot-graph" style={{ width: graphWidth, minWidth: graphWidth }}>
									{node.graphColumns.map((gc) => (
										<div className="graph-col" key={gc.key} style={{ left: gc.index * 28 + 9 }}>
											{gc.active && !gc.hasDot && <div className="graph-line" />}
											{gc.hasDot && (
												<>
													<div className="graph-line" />
													<div className={`graph-dot${isCurrent ? " current" : ""}`} />
												</>
											)}
										</div>
									))}
									{node.forkColumn >= 0 && (
										<div
											className="graph-fork"
											style={{
												left: node.forkColumn * 28 + 14,
												width: (node.column - node.forkColumn) * 28,
											}}
										/>
									)}
								</div>

								<div className={`snapshot-content${isCurrent ? " snapshot-current" : ""}`}>
									{isEditing ? (
										<div className="snapshot-edit-form">
											<div className="settings-field">
												<label htmlFor={`edit-name-${s.id}`}>Name</label>
												<input
													id={`edit-name-${s.id}`}
													type="text"
													value={editName}
													onChange={(e) => setEditName(e.target.value)}
													disabled={editAction.saving}
												/>
											</div>
											<div className="settings-field">
												<label htmlFor={`edit-desc-${s.id}`}>Description</label>
												<textarea
													id={`edit-desc-${s.id}`}
													value={editDesc}
													onChange={(e) => setEditDesc(e.target.value)}
													rows={2}
													disabled={editAction.saving}
												/>
											</div>
											<div className="snapshot-edit-actions">
												<button
													type="button"
													className="btn btn-primary btn-sm"
													disabled={editAction.saving || !editName.trim()}
													onClick={() => handleEdit(s.id)}
												>
													{editAction.saving ? "Saving..." : "Save"}
												</button>
												<button
													type="button"
													className="btn btn-ghost btn-sm"
													disabled={editAction.saving}
													onClick={() => setEditingId(null)}
												>
													Cancel
												</button>
											</div>
											<Feedback msg={editAction.feedback.msg} type={editAction.feedback.type} />
										</div>
									) : (
										<>
											<div className="snapshot-header">
												<span className="snapshot-name">{s.name}</span>
												<span className={`snapshot-badge ${s.type}`}>{s.type}</span>
												{isCurrent && <span className="snapshot-badge active">current</span>}
												<span className="snapshot-time">{relativeTime(s.created_at)}</span>
											</div>
											{s.description && <p className="snapshot-desc">{s.description}</p>}
											<div className="snapshot-row-actions">
												{isConfirmingRestore ? (
													<span className="confirm-inline">
														<span className="confirm-inline-label">Restore this snapshot?</span>
														<button
															type="button"
															className="btn btn-primary btn-sm"
															disabled={restoreAction.saving}
															onClick={() => handleRestore(s.id)}
														>
															{restoreAction.saving ? "Restoring..." : "Yes"}
														</button>
														<button
															type="button"
															className="btn btn-ghost btn-sm"
															onClick={() => setConfirmRestoreId(null)}
														>
															Cancel
														</button>
													</span>
												) : (
													<button
														type="button"
														className="btn btn-ghost btn-sm"
														disabled={isCurrent}
														onClick={() => setConfirmRestoreId(s.id)}
													>
														Restore
													</button>
												)}

												{s.type === "manual" && !isEditing && (
													<button
														type="button"
														className="btn btn-ghost btn-sm"
														onClick={() => openEdit(s)}
													>
														Edit
													</button>
												)}

												{isConfirmingDelete ? (
													<span className="confirm-inline">
														<span className="confirm-inline-label">Delete?</span>
														<button
															type="button"
															className="btn btn-danger btn-sm"
															disabled={deleteAction.saving}
															onClick={() => handleDelete(s.id)}
														>
															{deleteAction.saving ? "Deleting..." : "Yes"}
														</button>
														<button
															type="button"
															className="btn btn-ghost btn-sm"
															onClick={() => setConfirmDeleteId(null)}
														>
															Cancel
														</button>
													</span>
												) : (
													<button
														type="button"
														className="btn btn-danger btn-sm"
														disabled={isCurrent}
														onClick={() => setConfirmDeleteId(s.id)}
													>
														Delete
													</button>
												)}
											</div>
										</>
									)}
								</div>
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
}
