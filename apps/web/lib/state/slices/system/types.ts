import type {
  SystemInfo,
  DiskUsageResponse,
  DatabaseStats,
  SnapshotInfo,
  UpdatesResponse,
  SystemJob,
  SystemMetricsSnapshot,
  StorageMaintenanceRun,
  StorageOverviewResponse,
  StorageQuarantineEntry,
} from "@/lib/types/system";

export type SystemBackupsState = {
  items: SnapshotInfo[];
  loaded: boolean;
};

export type SystemJobsMap = Record<string, SystemJob>;

export type SystemSliceState = {
  system: {
    info: SystemInfo | null;
    diskUsage: DiskUsageResponse | null;
    database: DatabaseStats | null;
    backups: SystemBackupsState;
    updates: UpdatesResponse | null;
    jobs: SystemJobsMap;
    metrics: SystemMetricsSnapshot | null;
    storage: {
      overview: StorageOverviewResponse | null;
      runs: StorageMaintenanceRun[];
      quarantine: StorageQuarantineEntry[];
    };
  };
};

export type SystemSliceActions = {
  setSystemInfo: (info: SystemInfo) => void;
  setSystemDiskUsage: (usage: DiskUsageResponse) => void;
  setSystemDatabase: (stats: DatabaseStats) => void;
  setSystemBackups: (items: SnapshotInfo[]) => void;
  setSystemUpdates: (updates: UpdatesResponse) => void;
  upsertSystemJob: (job: SystemJob) => void;
  clearSystemJob: (jobId: string) => void;
  setSystemMetricsSnapshot: (snapshot: SystemMetricsSnapshot) => void;
  setSystemStorageOverview: (overview: StorageOverviewResponse) => void;
  setSystemStorageRuns: (runs: StorageMaintenanceRun[]) => void;
  setSystemStorageQuarantine: (entries: StorageQuarantineEntry[]) => void;
};

export type SystemSlice = SystemSliceState & SystemSliceActions;
