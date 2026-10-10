export type StorageTemporaryRootStatus = "measured" | "partial" | "unavailable" | "not_applicable";

export interface StorageTemporaryRootMeasurement {
  requested_path: string;
  path: string;
  aliases?: string[];
  status: StorageTemporaryRootStatus;
  size_bytes?: number;
  breakdown?: StorageTemporaryEntryBreakdown;
  skipped_count?: number;
  reason?: string;
  warnings?: string[];
}

export type StorageTemporaryEntryOwnership = "registered_kandev" | "untracked" | "unknown";

export interface StorageTemporaryEntry {
  name: string;
  kind: "file" | "directory" | "symlink" | "other";
  size_bytes?: number;
  completeness: "measured" | "partial";
  ownership: StorageTemporaryEntryOwnership;
}

export interface StorageTemporaryEntryBreakdown {
  status: StorageTemporaryRootStatus;
  entries: StorageTemporaryEntry[];
  other_observed_bytes: number;
  other_observed_count: number;
}

export interface StorageSystemTemporarySummary {
  status: StorageTemporaryRootStatus;
  roots: StorageTemporaryRootMeasurement[];
  size_bytes?: number;
  included_in_total: false;
  reason?: string;
  warnings?: string[];
}

export interface StorageDiskCapacityResponse {
  path: string;
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  used_percent: number;
  available: boolean;
  warning?: string;
  observed_at?: string;
  stale?: boolean;
  temporary_roots?: StorageTemporaryDiskCapacity[];
  temporary_roots_warning?: string;
}

export interface StorageTemporaryDiskCapacity {
  requested_path: string;
  path: string;
  aliases?: string[];
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  used_percent: number;
  available: boolean;
  warning?: string;
  observed_at?: string;
  shared_with_home: boolean | null;
  stale?: boolean;
}
