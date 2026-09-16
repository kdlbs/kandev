export type ToolPayloadAge = { value: number; unit: "weeks" | "months" };
export type ToolPayloadPolicy = { enabled: boolean; age: ToolPayloadAge; revision: number };
export type ToolPayloadBackupChoice = "backup" | "skip";
export type ToolPayloadOperation = {
  id: string;
  kind: "analysis" | "cleanup" | "backup";
  state: "running" | "succeeded" | "failed" | "partial" | "cancelled";
  scanned: number;
  eligible_tasks: number;
  eligible_messages: number;
  removed_messages: number;
  payload_bytes: number;
  skipped: Record<string, number>;
  cutoff: string;
  started_at: string;
  finished_at?: string;
  error?: string;
  age: ToolPayloadAge;
};
export type ToolPayloadRetentionStatus = {
  supported: boolean;
  policy: ToolPayloadPolicy;
  preparation: {
    state: "none" | "pending" | "running" | "failed" | "ready";
    choice: ToolPayloadBackupChoice | "";
    error?: string;
  };
  operation?: ToolPayloadOperation;
  last_analysis?: ToolPayloadOperation;
  last_run?: ToolPayloadOperation;
  next_due_at?: string;
};
export type ToolPayloadPolicyUpdate = ToolPayloadPolicy & {
  backup_choice?: ToolPayloadBackupChoice;
};
