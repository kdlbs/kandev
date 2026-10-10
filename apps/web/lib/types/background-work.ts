export type WorkloadKind = "shell" | "subagent" | "monitor" | "custom" | "unknown";

export type RunState =
  | "running"
  | "waiting"
  | "completed"
  | "failed"
  | "interrupted"
  | "ended"
  | "unknown";

export type WorkloadActionKind = "stop" | "interrupt" | "write_input" | "close_input";

export type ActionAvailabilityReason =
  | "unsupported"
  | "disconnected"
  | "not_running"
  | "ownership_unknown"
  | "operation_pending";

export type ActionCapability = {
  supported: boolean;
  available: boolean;
  reason?: ActionAvailabilityReason | string;
};

export type WorkloadCapabilities = {
  discovery: "snapshot" | "events_only" | string;
  output: "stream" | "snapshot" | "none" | string;
  transcript: boolean;
  parentage: boolean;
  reasoning_summary: boolean;
  attributable_usage: boolean;
  actions?: Partial<Record<WorkloadActionKind, ActionCapability>>;
};

export type WorkloadRunObservation = {
  session_id?: string;
  work_id: string;
  run_id?: string;
  kind: WorkloadKind;
  title: string;
  state: RunState;
  parent_work_id?: string;
  origin_turn_id?: string;
  source_message_id?: string;
  source_call_id?: string;
  exit_code?: number | null;
  capabilities: WorkloadCapabilities;
  output?: string;
  output_truncated?: boolean;
  output_offset?: number;
  started_at?: string | null;
  finished_at?: string | null;
  revision: number;
};

export type BackgroundWorkSnapshot = {
  workloads: WorkloadRunObservation[];
  captured_at: string;
};

export type BackgroundWorkActionRequest = {
  work_id: string;
  run_id?: string;
  action: WorkloadActionKind;
  data?: string;
  operation_id?: string;
};

export type BackgroundWorkActionResponse = {
  success: boolean;
  work_id: string;
  run_id?: string;
  action: WorkloadActionKind;
  error?: string;
  uncertain?: boolean;
};

export type WorkloadOutputChunk = {
  session_id?: string;
  work_id: string;
  run_id?: string;
  chunk: string;
  offset: number;
  truncated?: boolean;
  stream?: "stdout" | "stderr";
};
