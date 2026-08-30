// System pages — frontend types mirroring the
// `apps/backend/internal/system/` HTTP surface (see
// docs/specs/system-page/requirements/system-page.md "Backend surface").

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
  backup_directory: string;
  size_bytes: number;
  wal_size_bytes: number;
  schema_version: string;
  /** ISO timestamp; null when no backup has been taken yet. */
  last_backup_at: string | null;
}

export type SnapshotKind = "auto" | "manual";

export interface SnapshotInfo {
  name: string;
  size_bytes: number;
  /** ISO timestamp. */
  mtime: string;
  kind: SnapshotKind;
}

export type DiagnosticBundleStatus =
  | "collecting"
  | "building"
  | "ready"
  | "partial"
  | "failed"
  | "expired";

export type DiagnosticBundleSource = "backend" | "frontend" | "runtime" | "acp";

export interface DiagnosticBundleCapabilities {
  sources: DiagnosticBundleSource[];
  acp_debug_enabled: boolean;
  acp_max_sessions: number;
}

export type DiagnosticSessionAvailability = "host_retained" | "reachable" | "unavailable";

export interface DiagnosticSession {
  task_id: string;
  /** Returned only by the authorized ACP picker; never included in archive metadata. */
  task_title?: string;
  session_id: string;
  agent?: string;
  provider?: string;
  model?: string;
  status?: string;
  executor_type?: string;
  started_at?: string;
  last_activity_at?: string;
  acp_availability?: DiagnosticSessionAvailability;
}

export interface DiagnosticBundleJob {
  id: string;
  status: DiagnosticBundleStatus;
  sources: DiagnosticBundleSource[];
  session_ids?: string[];
  reused?: boolean;
  build_deadline: string;
  capture_deadline?: string;
  expires_at: string | null;
  browser_profiles: number;
  frontend_entry_count: number;
  frontend_bytes: number;
  warnings: string[];
  runtime_entry_count?: number;
  acp_session_count?: number;
  download_url?: string;
}

export interface FrontendLogUploadChunk {
  browser_id: string;
  capture_stream_id: string;
  chunk_index: number;
  done: boolean;
  storage_mode: "indexeddb" | "memory";
  capture_metadata: Record<string, unknown> | null;
  entries: unknown[];
}

export interface UpdatesResponse {
  current: string;
  latest: string;
  latest_url: string;
  /** ISO timestamp. */
  latest_checked_at: string;
  update_available: boolean;
  channel: UpdatesChannel;
  channel_editable: boolean;
  channel_unsupported_reason: string;
  install?: InstallState;
  apply_supported?: boolean;
  apply_unsupported_reason?: string;
  manual_commands?: string[];
}

export type UpdatesChannel = "stable" | "nightly";

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

export interface MessageQueueSettingsValue {
  max_per_session: number;
  merge_enabled: boolean;
  auto_merge_enabled: boolean;
}

/** Partial PATCH payload: omitted fields are left unchanged server-side. */
export type MessageQueueSettingsPatch = Partial<MessageQueueSettingsValue>;

export type MessageQueueSettingsSource = "default" | "setting" | "configuration" | "environment";

export interface MessageQueueEffectiveSettings extends MessageQueueSettingsValue {
  source: MessageQueueSettingsSource;
  locked: boolean;
}

export interface MessageQueueSettingsResponse {
  settings: MessageQueueSettingsValue;
  effective: MessageQueueEffectiveSettings;
}

export type SleepInhibitionPlatform = "darwin" | "windows" | "linux" | "other";
export type SleepInhibitionIssue =
  | "unsupported_platform"
  | "system_service_unavailable"
  | "request_failed";

export interface SleepInhibitionSettings {
  enabled: boolean;
}

export interface SleepInhibitionStatus {
  platform: SleepInhibitionPlatform;
  supported: boolean;
  active: boolean;
  issue?: SleepInhibitionIssue;
}

export interface SleepInhibitionResponse {
  settings: SleepInhibitionSettings;
  status: SleepInhibitionStatus;
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

export interface StorageWorkspaceSettings extends StorageResourceSettings {
  dependency_cleanup_enabled: boolean;
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
  workspaces: StorageWorkspaceSettings;
  kandev_containers: StorageResourceSettings;
  go_cache: StorageGoCacheSettings;
  docker: StorageDockerSettings;
}

export interface StorageCapabilities {
  managed_go_cache_path: string;
  go_cache_adoption_available: boolean;
  temporary_artifacts_available: boolean;
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

export interface StorageTemporaryArtifactsSummary {
  available?: boolean;
  total_count?: number;
  total_bytes?: number;
  active_count?: number;
  active_bytes?: number;
  protected_count?: number;
  protected_bytes?: number;
  stale_count?: number;
  stale_bytes?: number;
  skipped_count?: number;
  warnings?: string[];
  warning?: string;
}

export interface StorageSummary {
  workspaces: StorageWorkspaceSummary;
  go_cache: StorageGoCacheSummary;
  quarantine: StorageQuarantineSummary;
  temporary_artifacts: StorageTemporaryArtifactsSummary;
  docker: StorageDockerSummary;
}

export type StorageRunState =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "skipped_busy";

export interface StorageBusyResource {
  kind: string;
  label: string;
}

export interface StorageBusyResponse {
  error: string;
  busy_resources: StorageBusyResource[];
  force_available: boolean;
}

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
  resource_type: "task_workspace" | "go_cache" | "temporary_artifact";
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

export type StorageQuarantinePurgeScope = "eligible" | "all";

export interface StorageQuarantinePurgeFailure {
  id: string;
  error: string;
}

export interface StorageQuarantinePurgeResult {
  scope: StorageQuarantinePurgeScope;
  considered: number;
  deleted: number;
  deleted_bytes: number;
  protected: number;
  protected_bytes: number;
  failed: number;
  failed_bytes: number;
  failures?: StorageQuarantinePurgeFailure[];
}

export interface StorageOverviewResponse {
  settings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
  summary: StorageSummary;
  analyzed_at: string;
  last_run: StorageMaintenanceRun | null;
}

export interface StorageDiskCapacityResponse {
  path: string;
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  used_percent: number;
  available: boolean;
  warning?: string;
}

export interface StoragePolicyResponse {
  settings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
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
