import { useCallback, useEffect, useRef, useState } from "react";
import { deleteIPList, fetchIPListUsage, updateIPList } from "../api";
import { deepEqual } from "../deepEqual";
import { useIPInput } from "../hooks/useIPInput";
import type { IPList, IPListUsage } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";

interface Props {
	list: IPList;
	allLists: IPList[];
	onUpdated: () => void;
	onDeleted: (id: string) => void;
}

export default function IPListCard({ list, allLists, onUpdated, onDeleted }: Props) {
	const [ips, setIps] = useState<string[]>(list.ips);
	const [children, setChildren] = useState<string[]>(list.children);

	const ipField = useIPInput(ips, setIps);

	const [saving, setSaving] = useState(false);
	const [saveError, setSaveError] = useState<string | null>(null);
	const [deleteError, setDeleteError] = useState<string | null>(null);
	const [deleting, setDeleting] = useState(false);

	const [usage, setUsage] = useState<IPListUsage | null>(null);
	const [usageLoading, setUsageLoading] = useState(false);
	const usageLoadedRef = useRef(false);

	const dirty = !deepEqual(ips, list.ips) || !deepEqual(children, list.children);

	useEffect(() => {
		setIps(list.ips);
		setChildren(list.children);
	}, [list]);

	const loadUsage = useCallback(async () => {
		if (usageLoadedRef.current) return;
		usageLoadedRef.current = true;
		setUsageLoading(true);
		try {
			const u = await fetchIPListUsage(list.id);
			setUsage(u);
		} catch {
			// non-fatal
		} finally {
			setUsageLoading(false);
		}
	}, [list.id]);

	function addChild(id: string) {
		if (!id || children.includes(id)) return;
		setChildren((prev) => [...prev, id]);
	}

	function removeChild(id: string) {
		setChildren((prev) => prev.filter((x) => x !== id));
	}

	async function handleSave() {
		setSaving(true);
		setSaveError(null);
		try {
			await updateIPList(list.id, {
				name: list.name,
				description: list.description,
				ips,
				children,
			});
			onUpdated();
		} catch (err) {
			setSaveError(getErrorMessage(err, "Failed to save"));
		} finally {
			setSaving(false);
		}
	}

	function handleDiscard() {
		setIps(list.ips);
		setChildren(list.children);
		ipField.reset();
		setSaveError(null);
		setDeleteError(null);
	}

	async function handleDelete() {
		setDeleting(true);
		setDeleteError(null);
		try {
			await deleteIPList(list.id);
			onDeleted(list.id);
		} catch (err) {
			setDeleteError(getErrorMessage(err, "Failed to delete"));
			setDeleting(false);
		}
	}

	const sameTypeOtherLists = allLists.filter(
		(l) => l.type === list.type && l.id !== list.id && !children.includes(l.id),
	);

	const title = (
		<>
			<span className="ip-list-name" title={list.name}>
				{list.name}
			</span>
			<span className={`ip-list-badge ip-list-badge-${list.type}`}>{list.type}</span>
			<span className="ip-list-count">{list.resolved_count} IPs</span>
		</>
	);

	const actions = (
		<ConfirmDeleteButton
			onConfirm={handleDelete}
			label="Delete IP list"
			deleting={deleting}
			deletingLabel="Deleting..."
		/>
	);

	return (
		<CollapsibleCard title={title} actions={actions} ariaLabel={list.name}>
			<div className="ip-list-detail">
				{deleteError && (
					<div className="inline-error" role="alert">
						{deleteError}
					</div>
				)}
				{list.description && <p className="ip-list-description">{list.description}</p>}

				<div className="ip-list-section">
					<h4>IP Addresses</h4>
					{list.resolved_count === 0 && ips.length === 0 && children.length === 0 && (
						<span className="ip-list-warning">
							No IPs configured - this list resolves to nothing
						</span>
					)}
					{ips.length > 0 && (
						<div className="ip-list-ips">
							{ips.map((ip) => (
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
							onFocus={loadUsage}
							disabled={saving}
						/>
						<button type="button" className="btn btn-ghost" onClick={ipField.add} disabled={saving}>
							Add
						</button>
					</div>
					{ipField.error && <span className="ip-list-error">{ipField.error}</span>}
				</div>

				<div className="ip-list-section">
					<h4>Composed Lists</h4>
					{children.length > 0 && (
						<div className="ip-list-children">
							{children.map((cid) => {
								const child = allLists.find((l) => l.id === cid);
								return (
									<span key={cid} className="ip-chip ip-chip-list">
										{child ? child.name : cid}
										<button
											type="button"
											onClick={() => removeChild(cid)}
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
							<select value="" onChange={(e) => addChild(e.target.value)} disabled={saving}>
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

				<div className="ip-list-section">
					<h4>Used By</h4>
					{usageLoading && <span className="ip-list-count">Loading...</span>}
					{!usageLoading && usage && (
						<>
							{usage.routes.length === 0 && usage.composite_lists.length === 0 && (
								<span className="ip-list-count">Not used anywhere</span>
							)}
							{usage.routes.length > 0 && (
								<div className="ip-list-usage-group">
									<span className="ip-list-usage-label">Routes:</span>
									{usage.routes.map((r) => (
										<span key={r.id} className="ip-chip ip-chip-route">
											{r.domain}
										</span>
									))}
								</div>
							)}
							{usage.composite_lists.length > 0 && (
								<div className="ip-list-usage-group">
									<span className="ip-list-usage-label">Lists:</span>
									{usage.composite_lists.map((cl) => (
										<span key={cl.id} className="ip-chip ip-chip-list">
											{cl.name}
										</span>
									))}
								</div>
							)}
						</>
					)}
				</div>

				{saveError && (
					<div className="inline-error" role="alert">
						{saveError}
					</div>
				)}

				{dirty && (
					<div className="toggle-actions">
						<button
							type="button"
							className="btn btn-ghost"
							onClick={handleDiscard}
							disabled={saving}
						>
							Discard
						</button>
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleSave}
							disabled={saving}
						>
							{saving ? "Saving..." : "Save"}
						</button>
					</div>
				)}
			</div>
		</CollapsibleCard>
	);
}
