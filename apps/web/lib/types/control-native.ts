export type NativeDashboard = {
  workspace_id: string;
  generated_at: string;
  since: string;
  basis: "recorded_task_session_queue_transition_state";
  inference_state: "unknown";
  tasks: Record<string, number>;
  /** Full workspace workflow-stage groups, separate from recorded task states. */
  task_stages?: Array<{
    state: string;
    step_id: string;
    step_name: string;
    stage_type: string;
    count: number;
  }>;
  sessions: Record<string, number>;
  /** Unarchived, unfinished tasks whose primary session waits for input. */
  waiting_input_tasks: number;
  queued_messages: number;
  transitions_24h: number;
  completed_tasks_24h: number;
};

/** One recorded task row of the per-task control projection. */
export type NativeTask = {
  task_id: string;
  workspace_id: string;
  title: string;
  state: string;
  step_id: string;
  step_name: string;
  stage_type: string;
  session_id: string;
  session_state: string;
  archived: boolean;
  running_sessions: number;
  queued_messages: number;
  /** Per-message session policy. Missing means the policy source is unavailable. */
  queued_paused_messages?: number;
  queued_auto_run_messages?: number;
  /** Active native tree-hold mode. Missing means the source could not be read. */
  hold_mode?: string;
  updated_at: string;
};
export type NativeArchive = "active" | "archived" | "all";
export type NativeTaskList = {
  kind: "tasks";
  filter: string;
  archive: NativeArchive;
  total: number;
  truncated: boolean;
  generated_at: string;
  tasks: NativeTask[];
};
