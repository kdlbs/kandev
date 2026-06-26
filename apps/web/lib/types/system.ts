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
  boot_id: string;
  started_at: string;
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
  install?: InstallState;
  apply_supported?: boolean;
  apply_unsupported_reason?: string;
  manual_commands?: string[];
}

export interface InstallState {
  running_as_service: boolean;
  managed_service: boolean;
  mode?: string;
  manager?: string;
  kind?: string;
  metadata_path?: string;
}

export type SystemJobKind =
  | "vacuum"
  | "optimize"
  | "factory-reset"
  | "backup-create"
  | "restore"
  | "disk-walk"
  | "self-update";

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

export type SystemMetricId =
  | "cpu_percent"
  | "memory_percent"
  | "disk_percent"
  | "cpu_temp"
  | "io_load";

export interface SystemMetricsGlobalSettings {
  metrics: SystemMetricId[];
  interval_seconds: number;
  backend_disk_path: string;
  collect_execution: boolean;
}

export interface SystemMetricsSettingsResponse {
  settings: SystemMetricsGlobalSettings;
}

export interface SystemMetricSample {
  id: SystemMetricId | string;
  label: string;
  unit?: string;
  value?: number;
  available: boolean;
  error?: string;
}

export interface SystemMetricsSource {
  id: string;
  label: string;
  kind: "backend" | "execution" | string;
  executor_type?: string;
  session_id?: string;
  task_id?: string;
  metrics: SystemMetricSample[];
}

export interface SystemMetricsSnapshot {
  timestamp: string;
  interval_seconds: number;
  sources: SystemMetricsSource[];
}

export interface JobAcceptResponse {
  job_id: string;
}

export interface RestartCapability {
  supported: boolean;
  mode: "manual" | "supervisor" | string;
  adapter?: string;
  reason?: string;
  details?: Record<string, unknown>;
}

export interface RestartResponse {
  accepted: boolean;
  message: string;
}

export type LicenseEcosystem = "npm" | "go";

export interface LicenseEntry {
  name: string;
  version: string;
  license: string;
  repository?: string;
  license_text?: string;
  stale?: boolean;
  ecosystem?: LicenseEcosystem;
}
