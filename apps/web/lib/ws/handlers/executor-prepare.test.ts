import { describe, expect, it } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerExecutorPrepareHandlers } from "./executor-prepare";

const REMOTE_HELPER_PLATFORM = "linux/amd64";

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

describe("executor.prepare typed helper progress", () => {
  it("maps live progress and completion fields into PrepareStepInfo", () => {
    const fixture = makePrepareStore();
    const handlers = registerExecutorPrepareHandlers(fixture.store);

    handlers["executor.prepare.progress"]?.({
      id: "progress-1",
      type: "notification",
      action: "executor.prepare.progress",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        execution_id: "execution-1",
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

    expect(fixture.getState().prepareProgress.bySessionId["session-1"]?.steps[0]).toMatchObject({
      name: "",
      kind: "remote_helper_download",
      remotePlatform: REMOTE_HELPER_PLATFORM,
      failureCode: "timeout",
      status: "failed",
      error: "context deadline exceeded",
    });

    handlers["executor.prepare.completed"]?.({
      id: "completed-1",
      type: "notification",
      action: "executor.prepare.completed",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        execution_id: "execution-1",
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

    expect(fixture.getState().prepareProgress.bySessionId["session-1"]?.steps[0]).toMatchObject({
      kind: "remote_helper_download",
      remotePlatform: REMOTE_HELPER_PLATFORM,
      failureCode: "timeout",
    });
  });
});
