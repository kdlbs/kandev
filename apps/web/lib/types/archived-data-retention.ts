export type ArchivedDataAge = { days: number };
export type ArchivedDataPolicy = {
  enabled: boolean;
  archived_age: ArchivedDataAge;
  cleanup_age: ArchivedDataAge;
  revision: number;
};
export type ArchivedDataBackupChoice = "backup" | "skip";
export type ArchivedDataTarget = {
  rows: number;
  reduced: number;
  bytes: number;
  eligible: number;
  oversized: number;
  total_bytes: number;
};
export type ArchivedDataOperation = {
  id: string;
  kind: "analysis" | "cleanup" | "backup";
  state: "running" | "succeeded" | "failed" | "partial" | "cancelled";
  messages: ArchivedDataTarget;
  cleanup_jobs: ArchivedDataTarget;
  skipped: Record<string, number>;
  archived_cutoff: string;
  cleanup_cutoff: string;
  policy_revision: number;
  started_at: string;
  finished_at?: string;
  error?: string;
  analysis_only?: boolean;
  complete?: boolean;
  completed_target?: string;
};
export type ArchivedDataRetentionStatus = {
  supported: boolean;
  policy: ArchivedDataPolicy;
  preparation: {
    state: "none" | "pending" | "running" | "failed" | "ready";
    choice: ArchivedDataBackupChoice | "";
    error?: string;
  };
  operation?: ArchivedDataOperation;
  last_analysis?: ArchivedDataOperation;
  last_run?: ArchivedDataOperation;
  next_due_at?: string;
};
export type ArchivedDataPolicyUpdate = ArchivedDataPolicy & {
  backup_choice?: ArchivedDataBackupChoice;
};
