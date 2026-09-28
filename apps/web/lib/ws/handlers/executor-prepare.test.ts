import { describe, expect, it } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerExecutorPrepareHandlers } from "./executor-prepare";

const REMOTE_HELPER_PLATFORM = "linux/amd64";
const ACTION_PROGRESS = "executor.prepare.progress";
const ACTION_COMPLETED = "executor.prepare.completed";
const EXECUTION_ID = "execution-1";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";

function makePrepareStore() {
  let state = {
    prepareProgress: { bySessionId: {} },
  } as unknown as AppState;
  const store = {
    getState: () => state,
    setState: (updater: unknown) => {
      state =
        typeof updater === "function"
          ? (updater as (current: AppState) => AppState)(state)
          : ({ ...state, ...(updater as object) } as AppState);
    },
  } as unknown as StoreApi<AppState>;
  return { store, getState: () => state };
}

describe("executor.prepare progress mapping", () => {
  it("maps live progress and completion fields into PrepareStepInfo", () => {
    const fixture = makePrepareStore();
    const handlers = registerExecutorPrepareHandlers(fixture.store);

    handlers[ACTION_PROGRESS]?.({
      id: "progress-1",
      type: "notification",
      action: ACTION_PROGRESS,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        step_name: "",
        step_kind: "remote_helper_download",
        remote_platform: REMOTE_HELPER_PLATFORM,
        failure_code: "timeout",
        step_index: 0,
        total_steps: 1,
        status: "failed",
        error: "context deadline exceeded",
        timestamp: "2026-09-25T10:00:00Z",
      },
    } as never);

    expect(fixture.getState().prepareProgress.bySessionId[SESSION_ID]?.steps[0]).toMatchObject({
      name: "",
      kind: "remote_helper_download",
      remotePlatform: REMOTE_HELPER_PLATFORM,
      failureCode: "timeout",
      status: "failed",
      error: "context deadline exceeded",
    });

    handlers[ACTION_COMPLETED]?.({
      id: "completed-1",
      type: "notification",
      action: ACTION_COMPLETED,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        success: false,
        duration_ms: 1000,
        steps: [
          {
            name: "",
            kind: "remote_helper_download",
            remote_platform: REMOTE_HELPER_PLATFORM,
            failure_code: "timeout",
            status: "failed",
            error: "context deadline exceeded",
          },
        ],
        timestamp: "2026-09-25T10:00:00Z",
      },
    } as never);

    expect(fixture.getState().prepareProgress.bySessionId[SESSION_ID]?.steps[0]).toMatchObject({
      kind: "remote_helper_download",
      remotePlatform: REMOTE_HELPER_PLATFORM,
      failureCode: "timeout",
    });
  });
});

describe("executor.prepare attempt ordering", () => {
  it("replaces only MCP rows for a newer attempt and rejects unseen stale completions", () => {
    const fixture = makePrepareStore();
    const handlers = registerExecutorPrepareHandlers(fixture.store);
    const progress = (attemptId: string, startedAt: string, stepName: string) =>
      handlers[ACTION_PROGRESS]?.({
        id: `progress-${attemptId}`,
        type: "notification",
        action: ACTION_PROGRESS,
        payload: {
          task_id: TASK_ID,
          session_id: SESSION_ID,
          execution_id: EXECUTION_ID,
          step_name: stepName,
          step_kind: stepName === "Environment" ? "executor_environment" : "agent_mcp_verification",
          mcp_server_id: stepName === "Environment" ? undefined : "server-a",
          mcp_provider: stepName === "Environment" ? undefined : "cursor",
          step_command: stepName === "Environment" ? undefined : "must-not-be-stored",
          step_index: stepName === "Environment" ? 0 : 1,
          total_steps: 2,
          status: "completed",
          output: stepName === "Environment" ? undefined : "secret output",
          error: stepName === "Environment" ? undefined : "raw error",
          preparation_id: attemptId,
          preparation_started_at: startedAt,
          timestamp: startedAt,
        },
      } as never);

    progress("attempt-old", "2026-09-28T18:00:00.123456788Z", "Environment");
    progress("attempt-old", "2026-09-28T18:00:00.123456788Z", "Old verification");
    progress("attempt-new", "2026-09-28T18:00:00.123456789Z", "New verification");

    handlers[ACTION_COMPLETED]?.({
      id: "stale-completion",
      type: "notification",
      action: ACTION_COMPLETED,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        preparation_id: "attempt-unseen-old",
        preparation_started_at: "2026-09-28T18:00:00.123456787Z",
        success: true,
        duration_ms: 100,
        steps: [{ name: "stale", kind: "agent_mcp_verification", status: "completed" }],
        timestamp: "2026-09-28T18:00:00.123456787Z",
      },
    } as never);

    const prepare = fixture.getState().prepareProgress.bySessionId[SESSION_ID];
    expect(prepare).toMatchObject({
      preparationId: "attempt-new",
      preparationStartedAt: "2026-09-28T18:00:00.123456789Z",
      status: "preparing",
    });
    expect(prepare?.steps).toMatchObject([
      { name: "Environment", kind: "executor_environment", status: "completed" },
      {
        name: "",
        kind: "agent_mcp_verification",
        mcpServerId: "server-a",
        mcpProvider: "cursor",
      },
    ]);
    expect(prepare?.steps[1]?.command).toBeUndefined();
    expect(prepare?.steps[1]?.output).toBeUndefined();
    expect(prepare?.steps[1]?.error).toBeUndefined();
  });
});

describe("executor.prepare in-flight readiness and late progress", () => {
  it("does not report preparation complete from agent readiness while MCP verification runs", () => {
    const fixture = makePrepareStore();
    const handlers = registerExecutorPrepareHandlers(fixture.store);
    handlers[ACTION_PROGRESS]?.({
      id: "progress-running",
      type: "notification",
      action: ACTION_PROGRESS,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        step_name: "Verify server",
        step_kind: "agent_mcp_verification",
        mcp_server_id: "server-a",
        mcp_provider: "cursor",
        step_index: 0,
        total_steps: 1,
        status: "running",
        preparation_id: "attempt-current",
        preparation_started_at: "2026-09-28T18:00:00Z",
        timestamp: "2026-09-28T18:00:00Z",
      },
    } as never);

    const status = fixture.getState().prepareProgress.bySessionId[SESSION_ID]?.status;
    expect(status).toBe("preparing");
  });

  it("ignores delayed progress after the final snapshot for the same attempt", () => {
    const fixture = makePrepareStore();
    const handlers = registerExecutorPrepareHandlers(fixture.store);
    const attempt = {
      preparation_id: "attempt-current",
      preparation_started_at: "2026-09-28T18:00:00Z",
    };
    handlers[ACTION_COMPLETED]?.({
      id: "completed-current",
      type: "notification",
      action: ACTION_COMPLETED,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        ...attempt,
        success: true,
        duration_ms: 20,
        steps: [{ name: "Verified", kind: "agent_mcp_verification", status: "completed" }],
        timestamp: attempt.preparation_started_at,
      },
    } as never);
    handlers[ACTION_PROGRESS]?.({
      id: "delayed-progress",
      type: "notification",
      action: ACTION_PROGRESS,
      payload: {
        task_id: TASK_ID,
        session_id: SESSION_ID,
        execution_id: EXECUTION_ID,
        ...attempt,
        step_name: "Still verifying",
        step_kind: "agent_mcp_verification",
        step_index: 0,
        total_steps: 1,
        status: "running",
        timestamp: attempt.preparation_started_at,
      },
    } as never);

    expect(fixture.getState().prepareProgress.bySessionId[SESSION_ID]).toMatchObject({
      status: "completed",
      steps: [{ name: "", kind: "agent_mcp_verification", status: "completed" }],
    });
  });
});
