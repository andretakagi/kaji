import { useCallback, useEffect, useState } from "react";
import { createIPList, fetchIPLists, POLL_INTERVAL } from "../api";
import { deepEqual } from "../deepEqual";
import type { IPList } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { ErrorAlert } from "./ErrorAlert";
import IPListCard from "./IPListCard";
import { SectionHeader } from "./SectionHeader";

export default function IPLists() {
	const [lists, setLists] = useState<IPList[]>([]);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);
	const [showForm, setShowForm] = useState(false);

	const [newName, setNewName] = useState("");
	const [newDesc, setNewDesc] = useState("");
	const [newType, setNewType] = useState<"blacklist" | "whitelist">("blacklist");
	const [creating, setCreating] = useState(false);
	const [createError, setCreateError] = useState<string | null>(null);

	const load = useCallback(async () => {
		try {
			const data = await fetchIPLists();
			setLists((prev) => {
				if (deepEqual(prev, data)) return prev;
				return data;
			});
		} catch (err) {
			setError(getErrorMessage(err, "Failed to load IP lists"));
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		load();
		const id = setInterval(load, POLL_INTERVAL);
		return () => clearInterval(id);
	}, [load]);

	async function handleCreate(e: React.SubmitEvent) {
		e.preventDefault();
		if (!newName.trim()) {
			setCreateError("Name is required");
			return;
		}
		setCreating(true);
		setCreateError(null);
		try {
			await createIPList({
				name: newName.trim(),
				description: newDesc.trim(),
				type: newType,
				ips: [],
				children: [],
			});
			setNewName("");
			setNewDesc("");
			setNewType("blacklist");
			setShowForm(false);
			await load();
		} catch (err) {
			setCreateError(getErrorMessage(err, "Failed to create list"));
		} finally {
			setCreating(false);
		}
	}

	function handleDeleted(id: string) {
		setLists((prev) => prev.filter((l) => l.id !== id));
	}

	if (loading) {
		return <div className="empty-state">Loading IP lists...</div>;
	}

	return (
		<div className="ip-lists">
			<SectionHeader title="IP Lists">
				<button
					type="button"
					className="btn btn-primary"
					onClick={() => {
						if (showForm) {
							setNewName("");
							setNewDesc("");
							setNewType("blacklist");
							setCreateError(null);
						}
						setShowForm(!showForm);
					}}
				>
					{showForm ? "Cancel" : "New List"}
				</button>
			</SectionHeader>

			<ErrorAlert message={error} onDismiss={() => setError(null)} />

			{showForm && (
				<form className="ip-list-create-form" onSubmit={handleCreate}>
					<div className="form-row">
						<div className="form-field">
							<label htmlFor="ipl-new-name">Name</label>
							<input
								id="ipl-new-name"
								type="text"
								placeholder="e.g. Office IPs"
								value={newName}
								onChange={(e) => setNewName(e.target.value)}
								disabled={creating}
							/>
						</div>
						<div className="form-field">
							<label htmlFor="ipl-new-desc">Description</label>
							<input
								id="ipl-new-desc"
								type="text"
								placeholder="Optional"
								value={newDesc}
								onChange={(e) => setNewDesc(e.target.value)}
								disabled={creating}
							/>
						</div>
					</div>
					<div className="ip-list-type-selector">
						<label className="toggle-radio-option">
							<input
								type="radio"
								name="ipl-new-type"
								value="blacklist"
								checked={newType === "blacklist"}
								onChange={() => setNewType("blacklist")}
								disabled={creating}
							/>
							<span>Blacklist</span>
						</label>
						<label className="toggle-radio-option">
							<input
								type="radio"
								name="ipl-new-type"
								value="whitelist"
								checked={newType === "whitelist"}
								onChange={() => setNewType("whitelist")}
								disabled={creating}
							/>
							<span>Whitelist</span>
						</label>
					</div>
					{createError && <span className="ip-list-error">{createError}</span>}
					<button type="submit" className="btn btn-primary submit-btn" disabled={creating}>
						{creating ? "Creating..." : "Create"}
					</button>
				</form>
			)}

			{lists.length === 0 ? (
				<div className="empty-state">No IP lists yet. Create one to get started.</div>
			) : (
				<div className="ip-list-items">
					{lists.map((list) => (
						<IPListCard
							key={list.id}
							list={list}
							allLists={lists}
							onUpdated={load}
							onDeleted={handleDeleted}
						/>
					))}
				</div>
			)}
		</div>
	);
}
