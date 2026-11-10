import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdateStatus } from "@/lib/api";
import { ApiError } from "@/lib/api/client";
import {
  EMPTY_RUNTIME_UPDATE_BATCH,
  batchCounts,
  getRuntimeUpdateBatchCoordinator,
  loadRuntimeUpdateReviewRows,
  recoverAgentUpdateJobs,
  runtimeBatchTarget,
  type RuntimeBatchTarget,
  type RuntimeUpdateBatchStore,
} from "./runtime-update-batch";

const CLAUDE = "claude-acp";
const CODEX = "codex-acp";
const JOB_CLAUDE = "job-claude";
const JOB_CODEX = "job-codex";
const updateAgent = vi.fn();
const listAgentUpdateJobs = vi.fn();
const refreshStatuses = vi.fn();

vi.mock("@/lib/api", () => ({
  updateAgent: (...args: unknown[]) => updateAgent(...args),
  listAgentUpdateJobs: (...args: unknown[]) => listAgentUpdateJobs(...args),
}));
vi.mock("./runtime-update-statuses", () => ({
  refreshRuntimeUpdateStatuses: (...args: unknown[]) => refreshStatuses(...args),
}));

function status(overrides: Partial<AgentUpdateStatus> = {}): AgentUpdateStatus {
  return {
    agent_name: CLAUDE,
    display_name: "Claude",
    runtime_id: "npm:@example/claude",
    owner: "kandev",
    mechanism: "npm_candidate",
    management: "managed",
    update_mode: "pinned",
    package: "@example/claude",
    source: "@example/claude",
    available: true,
    enabled: true,
    auto_update_supported: true,
    auto_update: false,
    default_version: "1.0.0",
    effective_version: "1.0.0",
    current_version: "1.0.0",
    latest_version: "1.1.0",
    check_state: "update_available",
    ...overrides,
  };
}

function target(key: string): RuntimeBatchTarget {
  return {
    key,
    agentName: key,
    displayName: key,
    runtimeId: `npm:${key}`,
    package: key,
    currentVersion: "1.0.0",
    effectiveVersion: "1.0.0",
    targetVersion: "1.1.0",
    reviewedRuntime: {
      runtime_id: `npm:${key}`,
      package: key,
      current_version: "1.0.0",
      effective_version: "1.0.0",
      active_version: "1.0.0",
    },
  };
}

function createStore() {
  let batch = { ...EMPTY_RUNTIME_UPDATE_BATCH };
  let auth: {
    mode: "disabled" | "setup" | "enabled";
    authenticated: boolean;
    user: { id: string; role: string } | null;
  } = { mode: "disabled", authenticated: true, user: null };
  const upsertAgentUpdateJob = vi.fn();
  const store: RuntimeUpdateBatchStore = {
    getState: () => ({
      runtimeUpdateBatch: batch,
      setRuntimeUpdateBatch: (next) => {
        batch = next;
      },
      upsertAgentUpdateJob,
      agentRuntimeUpdates: { byAgent: {}, checkedAt: 0, loading: false },
      setAgentRuntimeUpdateStatuses: vi.fn(),
      setAgentRuntimeUpdateLoading: vi.fn(),
      auth,
    }),
  };
  const setAuth = (next: typeof auth) => {
    auth = next;
  };
  return { store, batch: () => batch, upsertAgentUpdateJob, setAuth };
}

function job(key: string, overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    job_id: `job-${key}`,
    agent_name: key,
    status: "queued",
    started_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

afterEach(() => {
  vi.clearAllMocks();
  updateAgent.mockReset();
  listAgentUpdateJobs.mockReset();
});

describe("runtime update review evidence", () => {
  it("accepts only an ordinary update with matching trusted evidence", () => {
    const preview = {
      target_version: "1.1.0",
      operation: "update",
      reviewed_runtime: {
        runtime_id: "npm:@example/claude",
        package: "@example/claude",
        current_version: "1.0.0",
        effective_version: "1.0.0",
        active_version: "1.0.0",
      },
    };
    expect(runtimeBatchTarget(status(), preview)?.targetVersion).toBe("1.1.0");
    expect(runtimeBatchTarget(status(), { ...preview, operation: "rollback" })).toBeNull();
    expect(
      runtimeBatchTarget(status(), {
        ...preview,
        reviewed_runtime: { ...preview.reviewed_runtime, runtime_id: "npm:other" },
      }),
    ).toBeNull();
    expect(
      runtimeBatchTarget(status(), {
        ...preview,
        reviewed_runtime: { ...preview.reviewed_runtime, source: "native" },
      }),
    ).toBeNull();
  });

  it("loads exact previews with at most two active requests", async () => {
    let active = 0;
    let maxActive = 0;
    const statuses = ["a", "b", "c", "d"].map((name) =>
      status({ agent_name: name, runtime_id: `npm:${name}`, package: name }),
    );
    const result = await loadRuntimeUpdateReviewRows(statuses, async (name) => {
      active += 1;
      maxActive = Math.max(maxActive, active);
      await Promise.resolve();
      active -= 1;
      return {
        target_version: "1.1.0",
        operation: "update",
        reviewed_runtime: {
          runtime_id: `npm:${name}`,
          package: name,
          current_version: "1.0.0",
          effective_version: "1.0.0",
          active_version: "1.0.0",
        },
      };
    });
    expect(result.rows).toHaveLength(4);
    expect(maxActive).toBeLessThanOrEqual(2);
  });
});

describe("runtime update batch coordinator", () => {
  it("submits each approved target once and keeps partial failures retryable", async () => {
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE), target(CODEX)]);
    updateAgent
      .mockResolvedValueOnce(job(CLAUDE, { job_id: JOB_CLAUDE }))
      .mockRejectedValueOnce(new ApiError("changed", 409, { code: "runtime_review_changed" }));

    await coordinator.start(new Set([CLAUDE, CODEX]));

    expect(updateAgent).toHaveBeenCalledTimes(2);
    expect(updateAgent).toHaveBeenNthCalledWith(
      1,
      CLAUDE,
      expect.objectContaining({
        update_mode: "pinned",
        target_version: "1.1.0",
        reviewed_runtime: expect.objectContaining({ runtime_id: "npm:claude-acp" }),
      }),
    );
    expect(batch().rows[CLAUDE]?.phase).toBe("queued");
    expect(batch().rows[CODEX]?.phase).toBe("failed");
    expect(batch().rows[CODEX]?.errorCode).toBe("review_changed");

    coordinator.observeJobs({ [CLAUDE]: job(CLAUDE, { job_id: JOB_CLAUDE, status: "succeeded" }) });
    expect(batch().status).toBe("complete");
    expect(batchCounts(batch()).succeeded).toBe(1);

    expect(coordinator.retryFailed()).toBe(true);
    expect(batch().status).toBe("review");
    expect(batch().order).toEqual([CLAUDE, CODEX]);
    expect(batch().rows[CLAUDE]?.phase).toBe("succeeded");
    expect(batch().rows[CODEX]?.phase).toBe("review");
  });

  it("does not submit twice while a run is latched", async () => {
    const pending = Promise.withResolvers<AgentUpdateJob>();
    updateAgent.mockReturnValueOnce(pending.promise);
    const { store } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE)]);

    const first = coordinator.start(new Set([CLAUDE]));
    const second = await coordinator.start(new Set([CLAUDE]));
    expect(second).toBe(false);
    pending.resolve(job(CLAUDE, { job_id: JOB_CLAUDE }));
    await first;
    expect(updateAgent).toHaveBeenCalledOnce();
  });

  it("keeps a lost POST response unconfirmed and out of retry", async () => {
    updateAgent.mockRejectedValueOnce(new TypeError("network down"));
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE)]);

    await coordinator.start(new Set([CLAUDE]));
    expect(batch().rows[CLAUDE]?.phase).toBe("unconfirmed");
    expect(batch().status).toBe("complete");
    expect(coordinator.retryFailed()).toBe(false);
  });

  it("never adopts a different active job as the requested result", async () => {
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE)]);
    updateAgent.mockResolvedValueOnce(job(CLAUDE, { job_id: JOB_CLAUDE }));
    await coordinator.start(new Set([CLAUDE]));

    coordinator.observeJobs({
      [CLAUDE]: job(CLAUDE, { job_id: "other-job", status: "succeeded" }),
    });
    expect(batch().rows[CLAUDE]?.phase).toBe("queued");
    coordinator.observeJobs({
      [CLAUDE]: job(CLAUDE, { job_id: JOB_CLAUDE, status: "failed", error: "boom" }),
    });
    expect(batch().rows[CLAUDE]?.phase).toBe("failed");
    expect(batch().rows[CLAUDE]?.error).toBe("boom");
  });
});

describe("retained job recovery", () => {
  it("shares one bounded read between observers and never cancels it", async () => {
    const pending = Promise.withResolvers<{ jobs: AgentUpdateJob[] }>();
    listAgentUpdateJobs.mockReturnValueOnce(pending.promise);
    const { store, upsertAgentUpdateJob } = createStore();
    const first = recoverAgentUpdateJobs(store);
    const second = recoverAgentUpdateJobs(store);
    expect(listAgentUpdateJobs).toHaveBeenCalledOnce();
    pending.resolve({ jobs: [job(CLAUDE)] });
    await Promise.all([first, second]);
    expect(upsertAgentUpdateJob).toHaveBeenCalledOnce();

    listAgentUpdateJobs.mockResolvedValueOnce({ jobs: [] });
    await recoverAgentUpdateJobs(store, true);
    expect(listAgentUpdateJobs).toHaveBeenCalledTimes(2);
  });
});

describe("runtime update batch authority", () => {
  it("stops remaining admissions when the authenticated identity changes", async () => {
    const pending = Promise.withResolvers<AgentUpdateJob>();
    updateAgent.mockReturnValueOnce(pending.promise);
    const { store, batch, setAuth, upsertAgentUpdateJob } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE), target(CODEX)]);
    const run = coordinator.start(new Set([CLAUDE, CODEX]));
    await expect.poll(() => updateAgent.mock.calls.length).toBe(1);

    setAuth({
      mode: "enabled",
      authenticated: true,
      user: { id: "user-2", role: "member" },
    });
    pending.resolve(job(CLAUDE, { job_id: JOB_CLAUDE }));
    await run;

    expect(updateAgent).toHaveBeenCalledOnce();
    expect(upsertAgentUpdateJob).not.toHaveBeenCalled();
    expect(batch().rows[CLAUDE]?.phase).toBe("unconfirmed");
    expect(batch().rows[CODEX]?.phase).toBe("failed");
    expect(batch().rows[CODEX]?.errorCode).toBe("authority");
  });

  it("stops on permission loss and does not publish later jobs", async () => {
    updateAgent
      .mockRejectedValueOnce(new ApiError("forbidden", 403, { error: "forbidden" }))
      .mockResolvedValueOnce(job(CODEX, { job_id: JOB_CODEX }));
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE), target(CODEX)]);

    await coordinator.start(new Set([CLAUDE, CODEX]));

    expect(updateAgent).toHaveBeenCalledOnce();
    expect(batch().rows[CLAUDE]?.errorCode).toBe("authority");
    expect(batch().rows[CODEX]?.phase).toBe("failed");
  });

  it("drops a late recovery response from a replaced identity", async () => {
    const pending = Promise.withResolvers<{ jobs: AgentUpdateJob[] }>();
    listAgentUpdateJobs.mockReturnValueOnce(pending.promise);
    const { store, setAuth, upsertAgentUpdateJob } = createStore();
    const read = recoverAgentUpdateJobs(store);
    setAuth({
      mode: "enabled",
      authenticated: true,
      user: { id: "user-2", role: "admin" },
    });
    pending.resolve({ jobs: [job(CLAUDE)] });
    await read;
    expect(upsertAgentUpdateJob).not.toHaveBeenCalled();
  });

  it("treats a malformed successful POST response as unconfirmed", async () => {
    updateAgent.mockRejectedValueOnce(new ApiError("invalid json", 202, null));
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE)]);

    await coordinator.start(new Set([CLAUDE]));

    expect(batch().rows[CLAUDE]?.phase).toBe("unconfirmed");
    expect(coordinator.retryFailed()).toBe(false);
  });
});

describe("runtime update batch retry review", () => {
  it("replaces rejected evidence with a fresh reviewed target while preserving success", async () => {
    updateAgent
      .mockResolvedValueOnce(job(CLAUDE, { job_id: JOB_CLAUDE }))
      .mockRejectedValueOnce(new ApiError("changed", 409, { code: "runtime_review_changed" }))
      .mockResolvedValueOnce(job(CODEX, { job_id: JOB_CODEX }));
    const { store, batch } = createStore();
    const coordinator = getRuntimeUpdateBatchCoordinator(store);
    coordinator.beginReview([target(CLAUDE), target(CODEX)]);
    await coordinator.start(new Set([CLAUDE, CODEX]));
    coordinator.observeJobs({
      [CLAUDE]: job(CLAUDE, { job_id: JOB_CLAUDE, status: "succeeded" }),
    });
    expect(batch().rows[CODEX]?.errorCode).toBe("review_changed");

    const fresh = {
      ...target(CODEX),
      targetVersion: "1.2.0",
      reviewedRuntime: {
        ...target(CODEX).reviewedRuntime,
        active_version: "1.1.0",
        effective_version: "1.1.0",
        current_version: "1.1.0",
      },
    };
    expect(coordinator.beginRetryReview([fresh])).toBe(true);
    expect(batch().rows[CLAUDE]?.phase).toBe("succeeded");
    expect(batch().rows[CODEX]?.phase).toBe("review");
    expect(batch().rows[CODEX]?.targetVersion).toBe("1.2.0");

    await coordinator.start(new Set([CODEX]));
    expect(updateAgent).toHaveBeenLastCalledWith(
      CODEX,
      expect.objectContaining({
        target_version: "1.2.0",
        reviewed_runtime: expect.objectContaining({ active_version: "1.1.0" }),
      }),
    );
  });
});
