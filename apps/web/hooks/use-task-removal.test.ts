import { describe, expect, it } from "vitest";
import { cachedSessionsAreFullyHydrated } from "./use-task-removal";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

function session(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-05-31T00:00:00Z",
    updated_at: "2026-05-31T00:00:00Z",
    is_primary: true,
    ...overrides,
  };
}

describe("cachedSessionsAreFullyHydrated", () => {
  it("treats an empty list as hydrated (nothing to fetch)", () => {
    expect(cachedSessionsAreFullyHydrated([])).toBe(true);
  });

  it("returns true when every session has an env ID and is_passthrough defined", () => {
    expect(
      cachedSessionsAreFullyHydrated([
        session({ task_environment_id: "env-1", is_passthrough: true }),
        session({ id: toSessionId("s2"), task_environment_id: "env-1", is_passthrough: false }),
      ]),
    ).toBe(true);
  });

  it("returns false when a session is missing its env ID", () => {
    expect(cachedSessionsAreFullyHydrated([session({ is_passthrough: true })])).toBe(false);
  });

  // Regression: the session-state bridge upserts a PARTIAL by-task entry from
  // agentctl WS events — these carry task_environment_id but OMIT is_passthrough.
  // The old env-ID-only guard accepted such a partial, so a client-side task
  // switch reused it, seeded the by-id observe cache without is_passthrough, and
  // the passthrough (PTY) terminal never mounted — chat rendered instead.
  // Requiring is_passthrough defined rejects the partial and forces a REST fetch.
  it("returns false for a bridge partial (env ID present, is_passthrough undefined)", () => {
    const partial = session({ task_environment_id: "env-1" });
    expect(partial.is_passthrough).toBeUndefined();
    expect(cachedSessionsAreFullyHydrated([partial])).toBe(false);
  });
});
