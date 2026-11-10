import {
  listAgentUpdateJobs,
  updateAgent,
  type AgentRuntimeReview,
  type AgentUpdateJob,
  type AgentUpdateStatus,
  type CopilotPluginCompatibility,
} from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import { refreshRuntimeUpdateStatuses } from "./runtime-update-statuses";

export type RuntimeBatchPhase =
  | "review"
  | "submitting"
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "unconfirmed";

export type RuntimeBatchErrorCode = "review_changed" | "maintenance" | "authority" | "error";

export type RuntimeBatchTarget = {
  key: string;
  agentName: string;
  displayName: string;
  runtimeId: string;
  package: string;
  currentVersion: string;
  effectiveVersion: string;
  targetVersion: string;
  reviewedRuntime: AgentRuntimeReview;
  compatibility?: CopilotPluginCompatibility;
};

export type RuntimeBatchRow = RuntimeBatchTarget & {
  phase: RuntimeBatchPhase;
  jobId?: string;
  error?: string;
  errorCode?: RuntimeBatchErrorCode;
  refreshError?: string;
};

export type RuntimeUpdateBatchState = {
  status: "idle" | "review" | "running" | "complete";
  authority: string;
  order: string[];
  rows: Record<string, RuntimeBatchRow>;
};

export const EMPTY_RUNTIME_UPDATE_BATCH: RuntimeUpdateBatchState = {
  status: "idle",
  authority: "",
  order: [],
  rows: {},
};

const ACTIVE_JOB_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "probing",
  "saving",
  "refreshing",
]);

export function runtimeBatchKey(status: AgentUpdateStatus): string {
  return `${status.agent_name}:${status.runtime_id}`;
}

export function isRuntimeReviewCurrent(
  status: AgentUpdateStatus,
  preview: {
    target_version: string;
    operation?: string;
    reviewed_runtime?: AgentRuntimeReview;
    copilot_plugin_compatibility?: CopilotPluginCompatibility;
  },
): boolean {
  const evidence = preview.reviewed_runtime;
  if (!evidence) return false;
  if (preview.operation !== "update") return false;
  if (preview.target_version !== status.latest_version) return false;
  if (evidence.runtime_id !== status.runtime_id) return false;
  if (evidence.package !== status.package) return false;
  if (evidence.effective_version !== status.effective_version) return false;
  if (evidence.current_version && status.current_version) {
    if (evidence.current_version !== status.current_version) return false;
  }
  if (status.runtime_source === "native") return false;
  if (evidence.source === "native") return false;
  if (
    typeof status.runtime_revision === "number" &&
    evidence.runtime_revision !== status.runtime_revision
  ) {
    return false;
  }
  return true;
}

export function runtimeBatchTarget(
  status: AgentUpdateStatus,
  preview: {
    target_version: string;
    operation?: string;
    reviewed_runtime?: AgentRuntimeReview;
    copilot_plugin_compatibility?: CopilotPluginCompatibility;
  },
): RuntimeBatchTarget | null {
  if (!isRuntimeReviewCurrent(status, preview) || !preview.reviewed_runtime) return null;
  return {
    key: runtimeBatchKey(status),
    agentName: status.agent_name,
    displayName: status.display_name || status.agent_name,
    runtimeId: status.runtime_id,
    package: status.package,
    currentVersion:
      preview.reviewed_runtime.current_version ||
      status.current_version ||
      status.effective_version,
    effectiveVersion: status.effective_version,
    targetVersion: preview.target_version,
    reviewedRuntime: preview.reviewed_runtime,
    compatibility: preview.copilot_plugin_compatibility,
  };
}

export type RuntimePreviewLoader = (
  agentName: string,
  targetVersion?: string,
) => Promise<{
  target_version: string;
  operation?: string;
  reviewed_runtime?: AgentRuntimeReview;
}>;

/**
 * Loads exact review evidence for bulk candidates with at most two active
 * preview requests. A failed or unsupported preview disables only its row.
 */
export async function loadRuntimeUpdateReviewRows(
  statuses: AgentUpdateStatus[],
  preview: RuntimePreviewLoader,
  concurrency = 2,
): Promise<{ rows: RuntimeBatchTarget[]; failures: Record<string, string> }> {
  const rows: RuntimeBatchTarget[] = [];
  const failures: Record<string, string> = {};
  let next = 0;
  const worker = async () => {
    while (next < statuses.length) {
      const index = next++;
      const status = statuses[index];
      const key = runtimeBatchKey(status);
      try {
        const result = await preview(status.agent_name, status.latest_version);
        const row = runtimeBatchTarget(status, result);
        if (row) rows.push(row);
        else failures[key] = "review_changed";
      } catch (error) {
        failures[key] = error instanceof Error ? error.message : String(error);
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(concurrency, statuses.length) }, () => worker()));
  return { rows, failures };
}

export function createRuntimeBatchReview(targets: RuntimeBatchTarget[]): RuntimeUpdateBatchState {
  return {
    status: "review",
    authority: "",
    order: targets.map((target) => target.key),
    rows: Object.fromEntries(
      targets.map((target) => [target.key, { ...target, phase: "review" as const }]),
    ),
  };
}

export function batchRowProgress(
  row: RuntimeBatchRow,
  job: AgentUpdateJob | undefined,
): RuntimeBatchRow {
  if (!row.jobId || !job || job.job_id !== row.jobId) return row;
  if (ACTIVE_JOB_STATUSES.has(job.status)) {
    return withRowProgress(row, {
      phase: job.status === "queued" ? "queued" : "running",
    });
  }
  if (job.status === "succeeded") {
    return withRowProgress(row, { phase: "succeeded", refreshError: job.refresh_error });
  }
  return withRowProgress(row, {
    phase: "failed",
    error: job.error,
    errorCode: "error",
  });
}

function withRowProgress(
  row: RuntimeBatchRow,
  next: Pick<RuntimeBatchRow, "phase"> &
    Partial<Pick<RuntimeBatchRow, "error" | "errorCode" | "refreshError">>,
): RuntimeBatchRow {
  if (
    row.phase === next.phase &&
    row.error === next.error &&
    row.errorCode === next.errorCode &&
    row.refreshError === next.refreshError
  ) {
    return row;
  }
  return { ...row, ...next };
}

export function applyRuntimeBatchJobs(
  state: RuntimeUpdateBatchState,
  jobs: Record<string, AgentUpdateJob>,
): RuntimeUpdateBatchState {
  let changed = false;
  const rows = { ...state.rows };
  for (const [key, row] of Object.entries(rows)) {
    const next = batchRowProgress(row, jobs[row.agentName]);
    if (next !== row) {
      rows[key] = next;
      changed = true;
    }
  }
  if (!changed) return state;
  return settleRuntimeBatch({ ...state, rows });
}

export function batchCounts(state: RuntimeUpdateBatchState) {
  let succeeded = 0;
  let failed = 0;
  let pending = 0;
  let unconfirmed = 0;
  for (const row of Object.values(state.rows)) {
    if (row.phase === "succeeded") succeeded += 1;
    else if (row.phase === "failed") failed += 1;
    else if (row.phase === "unconfirmed") unconfirmed += 1;
    else pending += 1;
  }
  return { succeeded, failed, pending, unconfirmed };
}

export function settleRuntimeBatch(state: RuntimeUpdateBatchState): RuntimeUpdateBatchState {
  if (state.status !== "running") return state;
  const { pending, unconfirmed } = batchCounts(state);
  if (pending > 0) return state;
  if (unconfirmed > 0) return { ...state, status: "complete" };
  return { ...state, status: "complete" };
}

export function retryFailedRuntimeBatch(state: RuntimeUpdateBatchState): RuntimeUpdateBatchState {
  if (state.status === "running") return state;
  const rows = { ...state.rows };
  let retryable = 0;
  for (const key of state.order) {
    const row = rows[key];
    if (!row || row.phase !== "failed") continue;
    rows[key] = {
      ...row,
      phase: "review",
      jobId: undefined,
      error: undefined,
      errorCode: undefined,
    };
    retryable += 1;
  }
  if (retryable === 0) return state;
  return { status: "review", authority: state.authority, order: state.order, rows };
}

function reviewChanged(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 409) return false;
  return (
    Boolean(error.body) &&
    typeof error.body === "object" &&
    (error.body as Record<string, unknown>).code === "runtime_review_changed"
  );
}

function maintenanceConflict(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 409) return false;
  return (
    Boolean(error.body) &&
    typeof error.body === "object" &&
    typeof (error.body as Record<string, unknown>).active_job_id === "string"
  );
}

type BatchStateAccess = {
  runtimeUpdateBatch: RuntimeUpdateBatchState;
  setRuntimeUpdateBatch: (state: RuntimeUpdateBatchState) => void;
  upsertAgentUpdateJob: (job: AgentUpdateJob) => void;
  agentRuntimeUpdates: {
    byAgent: Record<string, AgentUpdateStatus>;
    checkedAt: number;
    loading: boolean;
  };
  setAgentRuntimeUpdateStatuses: (statuses: AgentUpdateStatus[], checkedAt: number) => void;
  setAgentRuntimeUpdateLoading: (loading: boolean) => void;
  auth: {
    mode: "disabled" | "setup" | "enabled";
    authenticated: boolean;
    user: { id: string; role: string } | null;
  };
};

export type RuntimeUpdateBatchStore = { getState(): BatchStateAccess };

export function runtimeBatchAuthority(state: { auth: BatchStateAccess["auth"] }): string {
  return [
    state.auth.mode,
    state.auth.authenticated ? "1" : "0",
    state.auth.user?.id ?? "",
    state.auth.user?.role ?? "",
  ].join("|");
}

const coordinators = new WeakMap<RuntimeUpdateBatchStore, RuntimeUpdateBatchCoordinator>();
const recoveryReads = new WeakMap<RuntimeUpdateBatchStore, Promise<void>>();

function phaseFromJob(status: AgentUpdateJob["status"]): RuntimeBatchPhase {
  if (status === "queued") return "queued";
  if (ACTIVE_JOB_STATUSES.has(status)) return "running";
  if (status === "succeeded") return "succeeded";
  return "failed";
}

function batchStartOrder(
  state: RuntimeUpdateBatchState,
  keys: ReadonlySet<string>,
  eligibleKeys?: ReadonlySet<string>,
): string[] {
  return state.order.filter(
    (key) =>
      state.rows[key]?.phase === "review" &&
      keys.has(key) &&
      (!eligibleKeys || eligibleKeys.has(key)),
  );
}

function batchRunningRows(
  state: RuntimeUpdateBatchState,
  order: string[],
): { rows: Record<string, RuntimeBatchRow>; order: string[] } {
  const rows: Record<string, RuntimeBatchRow> = {};
  for (const key of state.order) {
    const row = state.rows[key];
    if (row?.phase === "succeeded") rows[key] = row;
  }
  for (const key of order) rows[key] = { ...state.rows[key], phase: "submitting" };
  const hasSettledRows = Object.values(rows).some((row) => row.phase === "succeeded");
  return {
    rows,
    order: hasSettledRows ? state.order.filter((key) => Boolean(rows[key])) : order,
  };
}

/**
 * Store-owned bulk coordinator. It survives manager dismissal and SPA
 * navigation because its state and in-flight loop belong to the app store.
 */
export class RuntimeUpdateBatchCoordinator {
  private running = false;

  constructor(private readonly store: RuntimeUpdateBatchStore) {}

  get state(): RuntimeUpdateBatchState {
    return this.store.getState().runtimeUpdateBatch;
  }

  private setState(state: RuntimeUpdateBatchState) {
    this.store.getState().setRuntimeUpdateBatch(state);
  }

  private patchRow(key: string, patch: Partial<RuntimeBatchRow>) {
    const state = this.state;
    const row = state.rows[key];
    if (!row) return;
    this.setState({
      ...state,
      rows: { ...state.rows, [key]: { ...row, ...patch } },
    });
  }

  beginReview(targets: RuntimeBatchTarget[]): boolean {
    if (this.running || this.state.status === "running") return false;
    this.setState({
      ...createRuntimeBatchReview(targets),
      authority: runtimeBatchAuthority(this.store.getState()),
    });
    return true;
  }

  /**
   * Retry review keeps successful rows and replaces failed targets with fresh
   * preview evidence. It never reuses evidence the server already rejected.
   */
  beginRetryReview(targets: RuntimeBatchTarget[]): boolean {
    const state = this.state;
    if (this.running || state.status === "running") return false;
    const rows: Record<string, RuntimeBatchRow> = {};
    const order: string[] = [];
    for (const key of state.order) {
      const row = state.rows[key];
      if (row?.phase === "succeeded") {
        rows[key] = row;
        order.push(key);
      }
    }
    for (const target of targets) {
      if (rows[target.key]?.phase === "succeeded") continue;
      rows[target.key] = { ...target, phase: "review" };
      if (!order.includes(target.key)) order.push(target.key);
    }
    this.setState({
      status: "review",
      authority: runtimeBatchAuthority(this.store.getState()),
      order,
      rows,
    });
    return true;
  }

  private authorityMatches(authority: string): boolean {
    return runtimeBatchAuthority(this.store.getState()) === authority;
  }

  private stopUnsubmitted(authority: string) {
    const state = this.state;
    const rows = { ...state.rows };
    for (const [key, row] of Object.entries(rows)) {
      if (row.phase === "submitting") {
        rows[key] = { ...row, phase: "failed", errorCode: "authority" };
      }
    }
    this.setState({ ...state, authority, rows });
  }

  async start(keys: ReadonlySet<string>, eligibleKeys?: ReadonlySet<string>): Promise<boolean> {
    const state = this.state;
    if (this.running || state.status === "running") return false;
    const authority = runtimeBatchAuthority(this.store.getState());
    if (state.authority && state.authority !== authority) return false;
    const order = batchStartOrder(state, keys, eligibleKeys);
    if (order.length === 0) return false;
    this.running = true;
    const running = batchRunningRows(state, order);
    this.setState({ status: "running", authority, order: running.order, rows: running.rows });
    try {
      for (const key of order) {
        if (!this.authorityMatches(authority)) {
          this.stopUnsubmitted(authority);
          break;
        }
        const outcome = await this.submit(key, authority);
        if (outcome === "authority" || !this.authorityMatches(authority)) {
          this.stopUnsubmitted(authority);
          break;
        }
      }
    } finally {
      this.running = false;
      this.setState(settleRuntimeBatch(this.state));
    }
    return true;
  }

  private async submit(key: string, authority: string): Promise<"ok" | "authority"> {
    const row = this.state.rows[key];
    if (!row) return "ok";
    try {
      const job = await updateAgent(row.agentName, {
        update_mode: "pinned",
        target_version: row.targetVersion,
        reviewed_runtime: row.reviewedRuntime,
      });
      if (!this.authorityMatches(authority)) {
        this.patchRow(key, { phase: "unconfirmed" });
        return "authority";
      }
      if (!job.job_id) {
        this.patchRow(key, {
          phase: job.operation === "up_to_date" ? "succeeded" : "failed",
          error: job.operation === "up_to_date" ? undefined : job.error,
        });
        if (job.operation === "up_to_date") {
          void refreshRuntimeUpdateStatuses(this.store, true, () =>
            this.authorityMatches(authority),
          );
        }
        return "ok";
      }
      this.store.getState().upsertAgentUpdateJob(job);
      const phase = phaseFromJob(job.status);
      this.patchRow(key, {
        jobId: job.job_id,
        phase,
        error: phase === "failed" ? job.error : undefined,
      });
      return "ok";
    } catch (error) {
      return this.submitFailure(key, error, authority);
    }
  }

  private submitFailure(key: string, error: unknown, authority: string): "ok" | "authority" {
    if (!this.authorityMatches(authority)) {
      this.patchRow(key, { phase: "unconfirmed" });
      return "authority";
    }
    if (reviewChanged(error)) {
      this.patchRow(key, { phase: "failed", errorCode: "review_changed" });
      return "ok";
    }
    if (maintenanceConflict(error)) {
      this.patchRow(key, { phase: "failed", errorCode: "maintenance" });
      return "ok";
    }
    if (!(error instanceof ApiError)) {
      this.patchRow(key, { phase: "unconfirmed" });
      return "ok";
    }
    if (error.status === 401 || error.status === 403) {
      this.patchRow(key, { phase: "failed", errorCode: "authority" });
      return "authority";
    }
    this.patchRow(key, {
      phase: error.status >= 200 && error.status < 300 ? "unconfirmed" : "failed",
      error: error instanceof Error ? error.message : String(error),
    });
    return "ok";
  }

  observeJobs(jobs: Record<string, AgentUpdateJob>) {
    if (this.state.status === "idle") return;
    if (!this.authorityMatches(this.state.authority)) return;
    this.setState(applyRuntimeBatchJobs(this.state, jobs));
  }

  retryFailed(): boolean {
    const next = retryFailedRuntimeBatch(this.state);
    if (next === this.state) return false;
    this.setState({ ...next, authority: runtimeBatchAuthority(this.store.getState()) });
    return true;
  }

  async refreshStatus(): Promise<void> {
    const authority = runtimeBatchAuthority(this.store.getState());
    await recoverAgentUpdateJobs(this.store, true);
    await refreshRuntimeUpdateStatuses(
      this.store,
      true,
      () => runtimeBatchAuthority(this.store.getState()) === authority,
    );
  }
}

export function getRuntimeUpdateBatchCoordinator(
  store: RuntimeUpdateBatchStore,
): RuntimeUpdateBatchCoordinator {
  let coordinator = coordinators.get(store);
  if (!coordinator) {
    coordinator = new RuntimeUpdateBatchCoordinator(store);
    coordinators.set(store, coordinator);
  }
  return coordinator;
}

/**
 * One bounded retained-job read per store. An unmounted observer cannot cancel
 * a read shared by another observer, and old list snapshots never replace a
 * newer live job because the store upsert is merge-safe.
 */
export function recoverAgentUpdateJobs(
  store: RuntimeUpdateBatchStore,
  force = false,
): Promise<void> {
  const pending = recoveryReads.get(store);
  if (pending && !force) return pending;
  if (pending && force) {
    return pending.then(() => recoverAgentUpdateJobs(store, true));
  }
  const authority = runtimeBatchAuthority(store.getState());
  const read = listAgentUpdateJobs({ cache: "no-store" })
    .then((response) => {
      if (runtimeBatchAuthority(store.getState()) !== authority) return;
      for (const job of response.jobs) store.getState().upsertAgentUpdateJob(job);
    })
    .catch(() => {})
    .finally(() => {
      if (recoveryReads.get(store) === read) recoveryReads.delete(store);
    });
  recoveryReads.set(store, read);
  return read;
}
