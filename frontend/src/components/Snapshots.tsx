import { useCallback, useEffect, useState } from "react";
import { createSnapshot, deleteSnapshot, fetchSnapshots, restoreSnapshot } from "../api";
import { formatTime } from "../formatTime";
import { useAsyncAction } from "../hooks/useAsyncAction";
import type { SnapshotIndex } from "../types/snapshots";
import { getErrorMessage } from "../utils/getErrorMessage";
import { ConfirmActionButton } from "./ConfirmActionButton";
import Feedback from "./Feedback";

function defaultSnapshotName(): string {
	const d = new Date();
	const pad = (n: number) => String(n).padStart(2, "0");
	return `snapshot-${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}`;
}

export default function Snapshots() {
	const [index, setIndex] = useState<SnapshotIndex | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState("");

	// Create form
	const [showCreate, setShowCreate] = useState(false);
	const [createName, setCreateName] = useState("");
	const [createDesc, setCreateDesc] = useState("");
	const createAction = useAsyncAction();

	const restoreAction = useAsyncAction();
	const deleteAction = useAsyncAction();

	const load = useCallback(async () => {
		setLoading(true);
		setError("");
		try {
			const data = await fetchSnapshots();
			setIndex(data);
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load snapshots"));
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		load();
	}, [load]);

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
			await load();
			return "Snapshot restored";
		});

	const handleDelete = (id: string) =>
		deleteAction.run(async () => {
			await deleteSnapshot(id);
			await load();
			return "Snapshot deleted";
		});

	const openCreate = () => {
		setCreateName(defaultSnapshotName());
		setCreateDesc("");
		setShowCreate(true);
		createAction.setFeedback({ msg: "", type: "success" });
	};

	if (loading) {
		return <div className="empty-state">Loading snapshots...</div>;
	}

	if (error) {
		return (
			<div className="empty-state">
				<p>{error}</p>
				<button type="button" className="btn btn-primary" onClick={load}>
					Retry
				</button>
			</div>
		);
	}

	const snapshots = index?.snapshots ?? [];
	const currentId = index?.current_id ?? "";
	const sorted = [...snapshots].sort(
		(a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
	);

	return (
		<div className="snapshots">
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
								maxLength={255}
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
								maxLength={1000}
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
				<div className="snapshot-list">
					{sorted.map((s) => {
						const isCurrent = s.id === currentId;

						return (
							<div className={`snapshot-card${isCurrent ? " snapshot-current" : ""}`} key={s.id}>
								<div className="snapshot-header">
									<span className="snapshot-name" title={s.name}>
										{s.name}
									</span>
									<span className={`snapshot-badge ${s.type}`}>{s.type}</span>
									{isCurrent && <span className="snapshot-badge active">current</span>}
									<span className="snapshot-time">{formatTime(s.created_at)}</span>
								</div>
								{s.description && <p className="snapshot-desc">{s.description}</p>}
								<div className="snapshot-row-actions">
									<ConfirmActionButton
										onConfirm={() => handleRestore(s.id)}
										trigger="Restore"
										confirmLabel="Yes"
										confirmingLabel="Restoring..."
										variant="primary"
										disabled={isCurrent}
										acting={restoreAction.saving}
									/>
									<ConfirmActionButton
										onConfirm={() => handleDelete(s.id)}
										trigger="Delete"
										confirmLabel="Yes"
										confirmingLabel="Deleting..."
										variant="danger"
										disabled={isCurrent}
										acting={deleteAction.saving}
									/>
								</div>
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
}
