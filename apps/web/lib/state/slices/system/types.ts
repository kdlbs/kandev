import type {
  SystemInfo,
  DiskUsageResponse,
  DatabaseStats,
  SnapshotInfo,
  LogFileInfo,
  UpdatesResponse,
  SystemJob,
  SystemMetricsSnapshot,
} from "@/lib/types/system";

export type SystemBackupsState = {
  items: SnapshotInfo[];
  loaded: boolean;
};

export type SystemLogsState = {
  files: LogFileInfo[];
  tail: string[];
  tailLoaded: boolean;
};

export type SystemJobsMap = Record<string, SystemJob>;

export type SystemSliceState = {
  system: {
    info: SystemInfo | null;
    diskUsage: DiskUsageResponse | null;
    database: DatabaseStats | null;
    backups: SystemBackupsState;
    logs: SystemLogsState;
    updates: UpdatesResponse | null;
    jobs: SystemJobsMap;
    metrics: SystemMetricsSnapshot | null;
  };
};

export type SystemSliceActions = {
  setSystemInfo: (info: SystemInfo) => void;
  setSystemDiskUsage: (usage: DiskUsageResponse) => void;
  setSystemDatabase: (stats: DatabaseStats) => void;
  setSystemBackups: (items: SnapshotInfo[]) => void;
  setSystemLogs: (files: LogFileInfo[]) => void;
  setSystemLogTail: (lines: string[]) => void;
  setSystemUpdates: (updates: UpdatesResponse) => void;
  upsertSystemJob: (job: SystemJob) => void;
  clearSystemJob: (jobId: string) => void;
  setSystemMetricsSnapshot: (snapshot: SystemMetricsSnapshot) => void;
};

export type SystemSlice = SystemSliceState & SystemSliceActions;
