import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";

/** Session states that mean the agent runtime is up and usable by a spec. */
const LIVE_SESSION_STATES = new Set(["RUNNING", "WAITING_FOR_INPUT", "IDLE", "COMPLETED"]);

/** Session states from which the agent will never become live. */
const TERMINAL_SESSION_STATES = new Set(["FAILED", "CANCELLED"]);

/**
 * Wait until the agent session for an onboarding-created office task is live.
 *
 * The office task's own `status` is workflow-owned and intentionally stays
 * `created` until an agent moves it, so it is *not* a launch signal — polling
 * it for `in_progress`/`done` can never succeed on this path. The session
 * state is the signal, and it is the same contract asserted directly by
 * `tests/office/onboarding-task-launch.spec.ts`.
 *
 * The 45s budget is 3x the 15s that `onboarding-task-launch.spec.ts` already
 * enforces for this same signal (it asserts after its loop, so CI has actually
 * been holding it to that budget). The extra margin is because this wait lives
 * in a worker-scoped fixture, where one timeout fails every test in the file.
 * It also has to stay comfortably under the 60s project timeout: worker fixture
 * setup is bounded by that, not by a test's own `test.setTimeout()`, so a
 * budget at or above 60s could never report its own diagnostic.
 */
export async function waitForOfficeTaskSessionLive(
  apiClient: ApiClient,
  taskId: string,
  timeoutMs = 45_000,
): Promise<void> {
  let observed = "none";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        // Captured every attempt so the failure below can describe the last
        // observed state without issuing an extra request.
        observed = sessions.map((s) => `${s.id}:${s.state}`).join(", ") || "none";
        const state = sessions[0]?.state ?? "";
        if (TERMINAL_SESSION_STATES.has(state)) {
          throw new Error(`agent session terminated before launch: ${observed}`);
        }
        return LIVE_SESSION_STATES.has(state);
      },
      { timeout: timeoutMs, message: `task ${taskId} agent session never reached a live state` },
    )
    .toBe(true)
    .catch((err: Error) => {
      throw new Error(`${err.message}; last observed sessions: ${observed}`);
    });
}
