export type TaskLspPolicy = "inherit" | "keep_warm" | "disabled";
export type TaskLspDetectionState = "unknown" | "scanning" | "complete" | "partial" | "unavailable";
export type TaskLspPhase =
  | "off"
  | "waiting_for_task"
  | "queued"
  | "installing"
  | "starting"
  | "process_started"
  | "initializing"
  | "ready"
  | "stopping"
  | "error"
  | "unsupported";
export type TaskLspAction = "" | "start" | "stop" | "restart" | "set_policy" | "reconcile";
export type TaskLspInitiator = "user" | "agent" | "automatic";
export type TaskLspActivity = "idle" | "server_work";

export type TaskLspWorkItem = {
  token: string;
  title: string;
  message?: string;
  percentage?: number;
  started_at: string;
};

export type TaskLspCompletedWorkItem = {
  token: string;
  title: string;
  message?: string;
  started_at: string;
  completed_at: string;
};

export type TaskLspLanguageSnapshot = {
  task_id: string;
  language: string;
  policy: TaskLspPolicy;
  detected: boolean;
  detection_state: TaskLspDetectionState;
  detection_scanned_at?: string;
  detection_truncated: boolean;
  phase: TaskLspPhase;
  generation: number;
  revision: number;
  process_started_at?: string;
  initialize_started_at?: string;
  ready_at?: string;
  last_transition_at: string;
  last_action: TaskLspAction;
  last_action_at?: string;
  last_stop_reason?: string;
  last_restart_reason?: string;
  last_initiator: TaskLspInitiator;
  restart_required: boolean;
  restart_required_reason?: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
  effective_policy: TaskLspPolicy;
  activity: TaskLspActivity;
  progress: TaskLspWorkItem[];
  last_completed_work?: TaskLspCompletedWorkItem;
};

export type TaskLspCapacity = { active: number; queued: number; limit: number };

export type TaskLspSnapshot = {
  task_id: string;
  languages: TaskLspLanguageSnapshot[];
  capacity: TaskLspCapacity;
};
