import { useCallback, useEffect, useState } from "react";
import { createIPList, fetchIPLists, POLL_INTERVAL } from "../api";
import { deepEqual } from "../deepEqual";
import type { IPList } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { validateIPOrCIDR } from "../utils/validate";
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
	const [newIps, setNewIps] = useState<string[]>([]);
	const [newIpInput, setNewIpInput] = useState("");
	const [newIpError, setNewIpError] = useState<string | null>(null);
	const [newChildren, setNewChildren] = useState<string[]>([]);
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

	function resetForm() {
		setNewName("");
		setNewDesc("");
		setNewType("blacklist");
		setNewIps([]);
		setNewIpInput("");
		setNewIpError(null);
		setNewChildren([]);
		setCreateError(null);
	}

	function addNewIp() {
		const val = newIpInput.trim();
		if (!val) return;
		const err = validateIPOrCIDR(val);
		if (err) {
			setNewIpError(err);
			return;
		}
		if (newIps.includes(val)) {
			setNewIpError("Already in list");
			return;
		}
		setNewIps((prev) => [...prev, val]);
		setNewIpInput("");
		setNewIpError(null);
	}

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
				ips: newIps,
				children: newChildren,
			});
			resetForm();
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

	const sameTypeOtherLists = lists.filter((l) => l.type === newType && !newChildren.includes(l.id));

	return (
		<div className="ip-lists">
			<SectionHeader title="IP Lists">
				<button
					type="button"
					className="btn btn-primary"
					onClick={() => {
						if (showForm) resetForm();
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
					<div className="ip-list-type-toggle">
						<button
							type="button"
							className={`type-toggle-btn${newType === "blacklist" ? " active" : ""}`}
							onClick={() => {
								setNewType("blacklist");
								setNewChildren([]);
							}}
							disabled={creating}
						>
							Blacklist
						</button>
						<button
							type="button"
							className={`type-toggle-btn${newType === "whitelist" ? " active" : ""}`}
							onClick={() => {
								setNewType("whitelist");
								setNewChildren([]);
							}}
							disabled={creating}
						>
							Whitelist
						</button>
					</div>

					<div className="ip-list-section">
						<h4>IP Addresses</h4>
						{newIps.length > 0 && (
							<div className="ip-list-ips">
								{newIps.map((ip) => (
									<span key={ip} className="ip-chip">
										{ip}
										<button
											type="button"
											onClick={() => setNewIps((prev) => prev.filter((x) => x !== ip))}
											aria-label={`Remove ${ip}`}
										>
											x
										</button>
									</span>
								))}
							</div>
						)}
						<div className="ip-list-add-row">
							<input
								type="text"
								placeholder="192.168.1.0/24"
								value={newIpInput}
								maxLength={45}
								onChange={(e) => {
									setNewIpInput(e.target.value);
									setNewIpError(null);
								}}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										addNewIp();
									}
								}}
								disabled={creating}
							/>
							<button
								type="button"
								className="btn btn-ghost"
								onClick={addNewIp}
								disabled={creating}
							>
								Add
							</button>
						</div>
						{newIpError && <span className="ip-list-error">{newIpError}</span>}
					</div>

					<div className="ip-list-section">
						<h4>Composed Lists</h4>
						{newChildren.length > 0 && (
							<div className="ip-list-children">
								{newChildren.map((cid) => {
									const child = lists.find((l) => l.id === cid);
									return (
										<span key={cid} className="ip-chip ip-chip-list">
											{child ? child.name : cid}
											<button
												type="button"
												onClick={() => setNewChildren((prev) => prev.filter((x) => x !== cid))}
												aria-label={`Remove ${child?.name ?? cid}`}
											>
												x
											</button>
										</span>
									);
								})}
							</div>
						)}
						{sameTypeOtherLists.length > 0 && (
							<div className="ip-list-add-child">
								<select
									value=""
									onChange={(e) => {
										if (e.target.value) setNewChildren((prev) => [...prev, e.target.value]);
									}}
									disabled={creating}
								>
									<option value="">Add a list...</option>
									{sameTypeOtherLists.map((l) => (
										<option key={l.id} value={l.id}>
											{l.name}
										</option>
									))}
								</select>
							</div>
						)}
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
