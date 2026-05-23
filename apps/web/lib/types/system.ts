// System pages — frontend types mirroring the
// `apps/backend/internal/system/` HTTP surface (see
// docs/specs/system-page/spec.md "Backend surface").

export interface SystemInfo {
  version: string;
  commit: string;
  build_time: string;
  go_version: string;
  os: string;
  arch: string;
}

export interface DiskBreakdown {
  data_dir: number;
  worktrees: number;
  repos: number;
  sessions: number;
  tasks: number;
  quick_chat: number;
  backups: number;
  total: number;
  warnings: string[];
  /** ISO timestamp. */
  computed_at: string;
}

export interface DiskUsageResponse {
  data: DiskBreakdown | null;
  computing: boolean;
  home_dir: string;
}

export interface DatabaseStats {
  path: string;
  size_bytes: number;
  wal_size_bytes: number;
  schema_version: string;
  /** ISO timestamp; null when no backup has been taken yet. */
  last_backup_at: string | null;
}

export type SnapshotKind = "auto" | "manual";

export interface SnapshotInfo {
  name: string;
  path: string;
  size_bytes: number;
  /** ISO timestamp. */
  mtime: string;
  kind: SnapshotKind;
}

export interface LogFileInfo {
  name: string;
  size: number;
  /** ISO timestamp. */
  mtime: string;
  current: boolean;
}

export interface LogTailResponse {
  lines: string[];
}

export interface UpdatesResponse {
  current: string;
  latest: string;
  latest_url: string;
  /** ISO timestamp. */
  latest_checked_at: string;
  update_available: boolean;
}

export type SystemJobKind =
  | "vacuum"
  | "optimize"
  | "factory-reset"
  | "backup-create"
  | "restore"
  | "disk-walk";

export type SystemJobState = "queued" | "running" | "succeeded" | "failed";

export interface SystemJob {
  id: string;
  kind: SystemJobKind | string;
  state: SystemJobState;
  message?: string;
  result?: Record<string, unknown>;
  /** ISO timestamp. */
  started_at: string;
  /** ISO timestamp. */
  ended_at?: string;
}

export interface JobAcceptResponse {
  job_id: string;
}

export type LicenseEcosystem = "npm" | "go";

export interface LicenseEntry {
  name: string;
  version: string;
  license: string;
  repository?: string;
  license_text?: string;
  ecosystem?: LicenseEcosystem;
}
