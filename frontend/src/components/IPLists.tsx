import { useState } from "react";
import { createIPList, fetchIPLists } from "../api";
import { useFormToggle } from "../hooks/useFormToggle";
import { useIPInput } from "../hooks/useIPInput";
import { usePolledData } from "../hooks/usePolledData";
import type { IPList } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import { ErrorAlert } from "./ErrorAlert";
import IPListCard from "./IPListCard";
import LoadingState from "./LoadingState";
import { SectionHeader } from "./SectionHeader";
import { Toggle } from "./Toggle";

export default function IPLists() {
	const {
		data: lists,
		setData: setLists,
		loading,
		error,
		setError,
		reload: load,
	} = usePolledData({
		fetcher: fetchIPLists,
		initialData: [] as IPList[],
		errorPrefix: "Failed to load IP lists",
	});
	const [newName, setNewName] = useState("");
	const [newDesc, setNewDesc] = useState("");
	const [newType, setNewType] = useState<"blacklist" | "whitelist">("blacklist");
	const [newIps, setNewIps] = useState<string[]>([]);
	const [newChildren, setNewChildren] = useState<string[]>([]);
	const [creating, setCreating] = useState(false);
	const [createError, setCreateError] = useState<string | null>(null);

	const ipField = useIPInput(newIps, setNewIps);

	function resetForm() {
		setNewName("");
		setNewDesc("");
		setNewType("blacklist");
		setNewIps([]);
		ipField.reset();
		setNewChildren([]);
		setCreateError(null);
	}

	const form = useFormToggle({ onClose: resetForm });

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
			form.close();
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
		return <LoadingState label="IP lists" />;
	}

	const sameTypeOtherLists = lists.filter((l) => l.type === newType && !newChildren.includes(l.id));

	return (
		<div className="ip-lists">
			<SectionHeader title="IP Lists">
				<button type="button" className="btn btn-primary" onClick={form.toggle}>
					{form.visible ? "Cancel" : "New List"}
				</button>
			</SectionHeader>

			<ErrorAlert message={error} onDismiss={() => setError("")} />

			{form.visible && (
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
					<Toggle
						options={["blacklist", "whitelist"] as const}
						value={newType}
						onChange={(v: "blacklist" | "whitelist") => {
							setNewType(v);
							setNewChildren([]);
						}}
						disabled={creating}
						aria-label="List type"
					/>

					<div className="ip-list-section">
						<h4>IP Addresses</h4>
						{newIps.length > 0 && (
							<div className="ip-list-ips">
								{newIps.map((ip) => (
									<span key={ip} className="ip-chip">
										{ip}
										<button
											type="button"
											onClick={() => ipField.remove(ip)}
											aria-label={`Remove ${ip}`}
										>
											<svg
												width="8"
												height="8"
												viewBox="0 0 8 8"
												fill="none"
												stroke="currentColor"
												strokeWidth="1.5"
												strokeLinecap="round"
												aria-hidden="true"
											>
												<path d="M1 1l6 6M7 1L1 7" />
											</svg>
										</button>
									</span>
								))}
							</div>
						)}
						<div className="ip-list-add-row">
							<input
								type="text"
								placeholder="192.168.1.0/24"
								value={ipField.input}
								maxLength={45}
								onChange={(e) => ipField.setInput(e.target.value)}
								onKeyDown={(e) => {
									if (e.key === "Enter") {
										e.preventDefault();
										ipField.add();
									}
								}}
								disabled={creating}
							/>
							<button
								type="button"
								className="btn btn-ghost"
								onClick={ipField.add}
								disabled={creating}
							>
								Add
							</button>
						</div>
						{ipField.error && <span className="ip-list-error">{ipField.error}</span>}
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
												<svg
													width="8"
													height="8"
													viewBox="0 0 8 8"
													fill="none"
													stroke="currentColor"
													strokeWidth="1.5"
													strokeLinecap="round"
													aria-hidden="true"
												>
													<path d="M1 1l6 6M7 1L1 7" />
												</svg>
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

					{createError && (
						<div className="inline-error" role="alert">
							{createError}
						</div>
					)}
					<button type="submit" className="btn btn-primary submit-btn" disabled={creating}>
						{creating ? "Creating..." : "Create"}
					</button>
				</form>
			)}

			{lists.length === 0 ? (
				<div className="empty-state">
					No IP lists yet. Lists let you define reusable groups of IP addresses or CIDR ranges that
					can be attached to routes as allowlists or blocklists.
				</div>
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
