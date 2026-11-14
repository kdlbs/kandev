/**
 * Wire shapes for the overview sections of the multi-workspace aggregate
 * (GET /api/v1/office/workspaces/aggregate) and its list routes. Mirrors the
 * snake_case JSON the backend serialises. Reasons are a code plus values so
 * the page phrases them in the viewer's language; `detail` is an agent's own
 * error text and is shown verbatim.
 */

export type OverviewScope = "office" | "reachable";
export type OverviewProjectionStatus = {
  state: "known" | "partial" | "unavailable";
  reasons: string[];
};
export type OverviewProjection = Partial<
  Record<"history" | "workspace_metrics" | "model_activity" | "blocks", OverviewProjectionStatus>
> & {
  history_kinds?: Record<string, OverviewProjectionStatus>;
};

/**
 * Workspace-card order on the overview. It is applied to the cards already in
 * the browser, so a card that arrives late still lands in its sorted place.
 */
export const OVERVIEW_SORT_OPTIONS = ["name", "recent", "problems"] as const;

export type OverviewSort = (typeof OVERVIEW_SORT_OPTIONS)[number];

export type OverviewStatus = "error" | "stalled" | "delayed" | "running" | "waiting" | "blocked";

/**
 * The periods the project-statistics block can cover, in hours. The rest of the
 * overview keeps its own fixed 24-hour window; only this block follows the
 * selection.
 */
export const OVERVIEW_STATS_WINDOW_HOURS = [24, 168, 720] as const;

export type OverviewStatsWindowHours = (typeof OVERVIEW_STATS_WINDOW_HOURS)[number];

export const OVERVIEW_STATS_WINDOW_DEFAULT: OverviewStatsWindowHours = 24;

/** Stable reason buckets for failed sessions, phrased by the client. */
export type OverviewFailureBucketCode = "no_response" | "limit" | "start_failed" | "other";

export type OverviewFailureBucket = {
  code: OverviewFailureBucketCode;
  count: number;
};

export type OverviewQueueStatus = "undeliverable" | "delayed" | "waiting";

export type OverviewReason = {
  code: string;
  values?: Record<string, string | number>;
  detail?: string;
};

export type OverviewSystem = {
  started_at?: string;
  active_tasks: number;
  running_sessions: number;
  waiting_input_sessions: number;
  /**
   * The configured session limits and the population each lane holds. Absent
   * means the reading did not arrive, which is not a reading of zero: the card
   * then shows the scope's own running count with no denominator. A limit of
   * zero means the general lane is unlimited, or the control lane is not
   * configured.
   */
  session_lanes?: OverviewSessionLanes;
  queued_messages: number;
  undeliverable_messages: number;
  needs_human: number;
  /** Provider-health blocks only. */
  blocked_accounts: number;
  /** Provider-health blocks plus open dynamic circuits; absent means unknown. */
  blocked_accounts_total?: number;
  earliest_unblock_at?: string;
  problems: number;
  /** The limits the status rules applied, so the UI explains the same rules. */
  problem_thresholds?: OverviewThresholds;
};

export type OverviewSessionLanes = {
  general_limit: number;
  control_limit: number;
  /** Absent together with the control count when the population was unread. */
  general_running_sessions?: number;
  control_running_sessions?: number;
  /**
   * Each lane's own share of the scope's waiting-input sessions. Absent together
   * when the populations are, and a measured zero is a zero rather than a
   * missing reading.
   */
  general_waiting_input_sessions?: number;
  control_waiting_input_sessions?: number;
};

/**
 * One workspace task that has a session executing right now. It is what a
 * project card's summary line reads, because the card answers "what is moving
 * here" from the same read that counted it.
 */
export type OverviewRunningTask = {
  task_id: string;
  title: string;
  status: OverviewStatus;
  step_name: string;
  agent_name?: string;
  running_sessions: number;
  waiting_input_sessions: number;
};

export type OverviewThresholds = {
  no_output_minutes: number;
  starting_minutes: number;
  not_advancing_minutes: number;
  queue_idle_minutes: number;
  queue_busy_minutes: number;
  dwell_in_progress_minutes: number;
  dwell_review_minutes: number;
  window_hours: number;
};

/**
 * The project-statistics block for one workspace: five period totals plus the
 * failure breakdown the owner asked to be able to expand. `window_hours` is the
 * period the backend actually applied, so the client labels the numbers with
 * the server's answer rather than its own guess.
 */
export type OverviewWorkspaceActivity = {
  window_hours: OverviewStatsWindowHours;
  completed: number;
  sessions_started: number;
  sessions_failed: number;
  agent_turns: number;
  step_moves: number;
  /** Automation-execution tasks created in the same window, shown separately. */
  automation: number;
  failure_buckets?: OverviewFailureBucket[];
  /** The agent's own first-line errors behind the buckets, shown verbatim. */
  failure_samples?: { kind: string; count: number }[];
};

export type OverviewWorkspaceMetrics = {
  status: OverviewStatus;
  active_tasks: number;
  running_sessions: number;
  waiting_input_sessions: number;
  last_output_at?: string;
  last_output_task_id?: string;
  queued_messages: number;
  completed_24h: number;
  open_tasks: number;
  waiting_tasks: number;
  blocked_tasks: number;
  blocked_by_tasks: number;
  problems: { error: number; stalled: number; delayed: number };
  top_warning?: {
    task_id: string;
    task_title: string;
    status: OverviewStatus;
    reason?: OverviewReason;
  };
  /** The period statistics block; present whenever the overview answered. */
  activity?: OverviewWorkspaceActivity;
};

export type OverviewParentTask = {
  task_id: string;
  title: string;
  status: OverviewStatus;
  children: number;
  open_children: number;
};

export type OverviewModelKind = "concrete" | "dynamic";

export type OverviewModel = {
  agent_profile_id: string;
  agent_id: string;
  agent_name: string;
  name: string;
  kind: OverviewModelKind;
  /**
   * The provider account this model authenticates with, so models of one
   * account can be shown together. Absent when the account is not identifiable,
   * and for a dynamic profile, which routes through concrete profiles. A
   * payload from a server without the field groups models by kind alone, which
   * is how this section behaved before the field existed.
   */
  account_id?: string;
  sessions_24h: number;
  running: number;
  failed_24h: number;
  errors: { kind: string; count: number }[] | null;
};

/** One dynamic-routing resource circuit the overview was told about. */
export type OverviewBlockedCircuit = {
  resource_key: string;
  scope: string;
  scope_value: string;
  state: string;
  code?: string;
  until?: string;
  strikes?: number;
  updated_at?: string;
  /**
   * False once the router recovered, so the circuit is not a current block. A
   * payload from a server without the field keeps its circuits listed, which is
   * how this screen behaved before the field existed.
   */
  blocking?: boolean;
  profile_id?: string;
  profile_name?: string;
  model_name?: string;
};

export type OverviewBlockedAccount = {
  workspace_id: string;
  provider_id: string;
  scope: string;
  scope_value: string;
  state: string;
  error_code?: string;
  retry_at?: string;
};

export type OverviewEventKind = string;

export type OverviewPullRequest = {
  owner: string;
  repo: string;
  number: number;
};

export type OverviewStepMove = {
  /** The step the task left to arrive here; absent when the move had no source. */
  from_step_name?: string;
  step_name?: string;
  at: string;
  actor?: string;
  trigger?: string;
  stopped?: boolean;
  /** How many movements of the same pair of steps this one line stands for. */
  repeat?: number;
  /** Arriving at a step this run had already left: work sent back. */
  sent_back?: boolean;
  /** Arriving from a step that starts nothing: finished or held work opened again. */
  reopened?: boolean;
};

export type OverviewFollowupSession = {
  session_id: string;
  model_name?: string;
  state: string;
  started_at: string;
  started_ago_minutes: number;
  later_sessions?: number;
};

/**
 * What a failed session was followed by, read on the same pass that reported the
 * failure so the line advances whenever the screen refreshes. `has_no_action`
 * is the server's own verdict that no later session, no routing fallback, and no
 * terminal task state exist, which is what the screen highlights rather than
 * leaving an uneventful row to read as healthy.
 */
export type OverviewFailure = {
  failed_ago_minutes: number;
  has_no_action: boolean;
  next_session?: OverviewFollowupSession;
  route_reason?: string;
  route_attempts?: number;
  task_state?: string;
};

export type OverviewEvent = {
  id?: string;
  recorded_state?: string;
  kind: OverviewEventKind;
  at: string;
  workspace_id?: string;
  automation_id?: string;
  task_id?: string;
  session_id?: string;
  title?: string;
  detail?: string;
  /** The running build on a server-start row; absent when nothing reported one. */
  version?: string;
  /** When a block is expected to lift; absent when no clear time is known. */
  clears_at?: string;
  /**
   * A model block's subject, as the blocked-circuits card carries it. Absent for
   * a provider limit, which is already named by the provider it carries, and
   * `reason` carries the circuit's classification code for the client to phrase.
   */
  scope?: string;
  scope_value?: string;
  profile_id?: string;
  profile_name?: string;
  model_name?: string;
  reason?: string;
  pull_request?: OverviewPullRequest;
  /** When a person answered; absent while the question is still open. */
  decided_at?: string;
  from?: string;
  to?: string;
  moves?: OverviewStepMove[];
  /** Every committed transition in the run, counted before moves were folded. */
  move_total?: number;
  /** Moves that returned the task to a step this run had already left. */
  sent_back?: number;
  /** Moves that opened finished or held work again. */
  reopened?: number;
  /** The run ended parked on a step that starts nothing. */
  held?: boolean;
  failure?: OverviewFailure;
};

export type OverviewHumanItem = {
  kind: "question" | "approval";
  id: string;
  workspace_id: string;
  workspace_name: string;
  task_id?: string;
  task_title?: string;
  session_id?: string;
  approval_type?: string;
  /** How many occurrences this row stands for; always at least one. */
  count: number;
  created_at: string;
};

export type OverviewTaskItem = {
  task_id: string;
  title: string;
  step_name: string;
  state: string;
  workspace_id: string;
  workspace_name: string;
  status?: OverviewStatus;
  reason?: OverviewReason;
  session_id?: string;
  session_state?: string;
  agent_profile_id?: string;
  model_name?: string;
  failures_24h: number;
  last_output_at?: string;
  step_entered_at: string;
  /**
   * When the task entered a completing step. Only a completed row carries it,
   * and it is what the 24-hour figure counts rather than the task's last write.
   */
  completed_at?: string;
  /**
   * The task has left the board. A completed list is mostly archived rows, so
   * this is what explains why a task the reader just watched finish is not on
   * any board.
   */
  archived?: boolean;
  queued_messages: number;
  /**
   * How many of this task's own sessions wait on a person, so an expanded list
   * answers that per row instead of leaving the reader to add up the scope's
   * single waiting figure.
   */
  waiting_input_sessions: number;
  /**
   * What followed this row's failed session. Present only for a row that has one,
   * so a task row answers "and what happened to it?" where the operator is
   * already looking rather than only in the events list.
   */
  failure?: OverviewFailure;
};

export type OverviewSessionItem = {
  session_id: string;
  task_id: string;
  task_title: string;
  workspace_id: string;
  workspace_name: string;
  agent_profile_id?: string;
  model_name?: string;
  session_state: string;
  status: OverviewStatus;
  reason?: OverviewReason;
  started_at: string;
  last_output_at?: string;
  /** What followed this session's failure; absent for a live session. */
  failure?: OverviewFailure;
};

export type OverviewQueueItem = {
  session_id: string;
  task_id: string;
  task_title: string;
  workspace_id: string;
  workspace_name: string;
  status: OverviewQueueStatus;
  reason?: OverviewReason;
  count: number;
  oldest_at: string;
  sender: "user" | "agent" | "workflow" | "system";
  session_state: string;
  first_line?: string;
};

export type OverviewTaskFilter =
  | "problems"
  | "active"
  | "sessions"
  | "queued"
  | "completed"
  | "hold"
  | "all";

export type OverviewRunningKind = "tasks" | "sessions" | "queue";

export type OverviewListResponse = {
  kind: OverviewRunningKind;
  filter?: OverviewTaskFilter;
  total: number;
  tasks?: OverviewTaskItem[];
  sessions?: OverviewSessionItem[];
  queue?: OverviewQueueItem[];
};

/** Overview sections carried on the aggregate response next to its counts. */
export type OverviewSections = {
  scope: OverviewScope;
  projection?: OverviewProjection;
  generated_at?: string;
  compute_ms?: number;
  system?: OverviewSystem;
  models?: OverviewModel[];
  blocked_accounts?: OverviewBlockedAccount[];
  blocked_circuits?: OverviewBlockedCircuit[];
  last_24h?: OverviewEvent[];
  needs_human?: OverviewHumanItem[];
};
