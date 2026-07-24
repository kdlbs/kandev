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
  driver: string;
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
  | "self-update"
  | "storage-analysis"
  | "storage-cleanup"
  | "storage-quarantine-delete";

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

export interface StorageResourceSettings {
  enabled: boolean;
}

export interface StorageGoCacheSettings {
  enabled: boolean;
  max_bytes: number;
  adopted_path: string;
}

export interface StorageDockerSettings {
  dedicated_daemon_acknowledged: boolean;
  build_cache_enabled: boolean;
  build_cache_keep_bytes: number;
  build_cache_unused_hours: number;
  unused_images_enabled: boolean;
  unused_images_hours: number;
}

export interface StorageMaintenanceSettings {
  enabled: boolean;
  check_interval_hours: number;
  idle_for_minutes: number;
  orphan_grace_hours: number;
  quarantine_retention_hours: number;
  workspaces: StorageResourceSettings;
  kandev_containers: StorageResourceSettings;
  go_cache: StorageGoCacheSettings;
  docker: StorageDockerSettings;
}

export interface StorageCapabilities {
  managed_go_cache_path: string;
  go_cache_adoption_available: boolean;
  docker_available: boolean;
  docker_host: string;
  host_global_docker_cleanup_allowed: boolean;
}

export interface StorageWorkspaceSummary {
  total_bytes?: number;
  active_bytes?: number;
  candidate_bytes?: number;
  warnings?: string[];
  available?: boolean;
  warning?: string;
}

export interface StorageGoCacheSummary {
  path?: string;
  size_bytes?: number;
  owned?: boolean;
  enabled?: boolean;
  unmanaged_path?: string;
  unmanaged_size_bytes?: number;
  available?: boolean;
  warning?: string;
}

export interface StorageDockerSummary {
  available: boolean;
  image_layer_bytes?: number;
  build_cache_bytes: number;
  unused_image_bytes: number;
  managed_container_count: number;
  managed_container_bytes: number;
  warnings?: string[];
}

export type StorageQuarantineSummary =
  | {
      available?: true;
      count: number;
      size_bytes: number;
      warning?: never;
    }
  | {
      available: false;
      warning: string;
      count?: never;
      size_bytes?: never;
    };

export interface StorageSummary {
  workspaces: StorageWorkspaceSummary;
  go_cache: StorageGoCacheSummary;
  quarantine: StorageQuarantineSummary;
  docker: StorageDockerSummary;
}

export type StorageRunState =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "skipped_busy";

export interface StorageMaintenanceRun {
  id: string;
  trigger: "scheduled" | "manual" | "analysis";
  state: StorageRunState;
  settings_snapshot: StorageMaintenanceSettings;
  result: Record<string, unknown>;
  message: string;
  started_at: string;
  completed_at?: string;
}

export interface StorageQuarantineEntry {
  id: string;
  resource_type: "task_workspace" | "go_cache";
  task_id?: string;
  workspace_id?: string;
  original_path: string;
  quarantine_path: string;
  size_bytes: number;
  state: "quarantined" | "restored" | "deleted" | "failed";
  quarantined_at: string;
  delete_after: string;
  restored_at?: string;
  deleted_at?: string;
  last_error: string;
  metadata: Record<string, unknown>;
}

export interface StorageOverviewResponse {
  settings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
  summary: StorageSummary;
  analyzed_at: string;
  last_run: StorageMaintenanceRun | null;
}

export interface StorageSettingsResponse {
  settings: StorageMaintenanceSettings;
}

export interface StorageAdoptionResponse extends StorageSettingsResponse {
  capabilities: StorageCapabilities;
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

export type LicenseEcosystem = "npm" | "go" | "source";

export interface LicenseEntry {
  name: string;
  version: string;
  license: string;
  repository?: string;
  license_text?: string;
  stale?: boolean;
  ecosystem?: LicenseEcosystem;
}
