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
