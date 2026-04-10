import { memo, useEffect, useRef, useState } from "react";
import { deepEqual } from "../deepEqual";
import type { ParsedRoute, RouteToggles } from "../types/api";
import { getErrorMessage } from "../utils/getErrorMessage";
import CollapsibleCard from "./CollapsibleCard";
import { ConfirmDeleteButton } from "./ConfirmDeleteButton";
import ToggleGrid from "./ToggleGrid";

interface Props {
	route: ParsedRoute;
	deleting: boolean;
	toggling: boolean;
	onDelete: (id: string) => void;
	onToggleEnabled: (route: ParsedRoute) => void;
	onUpdateToggles: (route: ParsedRoute, toggles: RouteToggles) => Promise<void>;
	globalAutoHttps?: "on" | "off" | "disable_redirects";
}

export default memo(
	function RouteRow({
		route,
		deleting,
		toggling,
		onDelete,
		onToggleEnabled,
		onUpdateToggles,
		globalAutoHttps,
	}: Props) {
		const [toggles, setToggles] = useState<RouteToggles>(route.toggles);
		const [dirty, setDirty] = useState(false);
		const [saving, setSaving] = useState(false);
		const [saveError, setSaveError] = useState<string | null>(null);
		const [stale, setStale] = useState(false);
		const lastBackendToggles = useRef(route.toggles);

		useEffect(() => {
			const changed = !deepEqual(route.toggles, lastBackendToggles.current);
			lastBackendToggles.current = route.toggles;

			if (!dirty) {
				setToggles(route.toggles);
				setStale(false);
			} else if (changed) {
				setStale(true);
			}
		}, [route.toggles, dirty]);

		function updateToggle<K extends keyof RouteToggles>(key: K, value: RouteToggles[K]) {
			const next = { ...toggles, [key]: value };
			setToggles(next);
			setDirty(true);
		}

		async function handleSaveToggles() {
			setSaving(true);
			setSaveError(null);
			try {
				await onUpdateToggles(route, toggles);
				setDirty(false);
				setStale(false);
			} catch (err) {
				setSaveError(getErrorMessage(err, "Failed to save"));
			} finally {
				setSaving(false);
			}
		}

		function handleDiscard() {
			setToggles(lastBackendToggles.current);
			setDirty(false);
			setStale(false);
			setSaveError(null);
		}

		const title = (
			<>
				<span className="route-domain">{route.domain || "(no domain)"}</span>
				<span className="route-arrow">&rarr;</span>
				<span className="route-upstream">{route.upstream || "(no upstream)"}</span>
			</>
		);

		const actions = (
			<>
				<label className="toggle-switch" title={route.disabled ? "Enable" : "Disable"}>
					<input
						type="checkbox"
						checked={!route.disabled}
						onChange={() => onToggleEnabled(route)}
						disabled={!route.id || toggling}
						aria-label={route.disabled ? "Enable route" : "Disable route"}
					/>
					<span className="toggle-slider" />
				</label>

				<ConfirmDeleteButton
					onConfirm={() => onDelete(route.id)}
					label="Delete route"
					disabled={!route.id}
					deleting={deleting}
					deletingLabel="Deleting..."
				/>
			</>
		);

		return (
			<CollapsibleCard
				title={title}
				actions={actions}
				disabled={route.disabled}
				ariaLabel={`${route.domain || "route"}`}
			>
				<ToggleGrid
					toggles={toggles}
					onUpdate={updateToggle}
					idPrefix={route.id}
					isNew={!route.id || !route.toggles.basic_auth.enabled}
					domain={route.domain}
					globalAutoHttps={globalAutoHttps}
				/>

				{stale && (
					<div className="stale-warning">
						<span>Route was updated externally. Your unsaved edits may be outdated.</span>
						<button type="button" onClick={handleDiscard}>
							Discard
						</button>
					</div>
				)}

				{saveError && (
					<div className="inline-error" role="alert">
						{saveError}
					</div>
				)}

				{dirty && !route.disabled && (
					<div className="toggle-actions">
						<button
							type="button"
							className="btn btn-ghost discard-toggles-btn"
							onClick={handleDiscard}
							disabled={saving}
						>
							Discard
						</button>
						<button
							type="button"
							className="btn btn-primary save-toggles-btn"
							onClick={handleSaveToggles}
							disabled={saving}
						>
							{saving ? "Saving..." : "Save Changes"}
						</button>
					</div>
				)}
			</CollapsibleCard>
		);
	},
	(prev, next) => {
		return (
			prev.deleting === next.deleting &&
			prev.toggling === next.toggling &&
			prev.onDelete === next.onDelete &&
			prev.onToggleEnabled === next.onToggleEnabled &&
			prev.onUpdateToggles === next.onUpdateToggles &&
			prev.globalAutoHttps === next.globalAutoHttps &&
			prev.route.id === next.route.id &&
			prev.route.domain === next.route.domain &&
			prev.route.upstream === next.route.upstream &&
			prev.route.disabled === next.route.disabled &&
			prev.route.server === next.route.server &&
			deepEqual(prev.route.toggles, next.route.toggles)
		);
	},
);
