export interface Snapshot {
	id: string;
	name: string;
	description: string;
	type: "auto" | "manual";
	parent_id: string;
	created_at: string;
}

export interface SnapshotIndex {
	current_id: string;
	auto_snapshot_enabled: boolean;
	auto_snapshot_limit: number;
	snapshots: Snapshot[];
}

export interface SnapshotSettings {
	auto_snapshot_enabled: boolean;
	auto_snapshot_limit: number;
}
