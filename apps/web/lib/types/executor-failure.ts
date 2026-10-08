export type ExecutorObservation = {
  outcome: "healthy" | "terminated" | "missing" | "restarted" | "unknown";
  runtime: string;
  observed_at: string;
  occurred_at?: string;
  reason?: string;
  message?: string;
  pod_phase?: string;
  container_ready: boolean;
  restarts?: number;
  workspace: "retained" | "unknown";
  pod_conditions?: Array<{
    type: string;
    status: string;
    reason?: string;
    message?: string;
    transition_at?: string;
  }>;
  secondary?: Array<{ operation: string; reason: string; occurred_at: string }>;
  containers?: Array<{
    name: string;
    state: string;
    ready: boolean;
    restarts?: number;
    reason?: string;
    exit_code?: number;
    signal?: number;
    started_at?: string;
    finished_at?: string;
    last_exit_code?: number;
    last_finished_at?: string;
  }>;
};

export type ExecutorFailureEpisode = {
  id: string;
  task_id: string;
  environment_id?: string;
  session_id?: string;
  revision: number;
  state: "active" | "resolved";
  current_outcome?: ExecutorObservation["outcome"];
  first_observed_at: string;
  last_observed_at: string;
  resolved_at?: string;
  observation: ExecutorObservation;
};
