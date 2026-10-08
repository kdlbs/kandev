import { describe, it, expect } from "vitest";
import {
  applyExecutorRecheck,
  executorFailureForComposer,
  executorFailureTitleKey,
  providerRecoveryKey,
} from "./executor-failure";
import type { ExecutorFailureEpisode } from "./types/executor-failure";
const failure = {
  id: "first",
  revision: 2,
  state: "active",
  observation: { outcome: "terminated", reason: "Evicted", containers: [{ exit_code: 137 }] },
} as ExecutorFailureEpisode;
describe("executor evidence", () => {
  it("rejects a delayed recheck from a previous episode or revision", () => {
    expect(
      applyExecutorRecheck(failure, { ...failure, id: "old", revision: 99, state: "resolved" }),
    ).toBe(failure);
    expect(applyExecutorRecheck(failure, { ...failure, revision: 1, state: "resolved" })).toBe(
      failure,
    );
  });
  it("preserves eviction and uncertainty without inferring OOM from 137", () => {
    expect(executorFailureTitleKey(failure)).toBe("task:executorFailureEvicted");
    expect(executorFailureTitleKey({ ...failure, current_outcome: "unknown" })).toBe(
      "task:executorFailureUnverified",
    );
    expect(
      executorFailureTitleKey({
        ...failure,
        observation: { ...failure.observation, reason: "ContainerExited" },
      }),
    ).toBe("task:executorFailureStopped");
  });
  it("requires an explicit provider result for continuity", () => {
    expect(providerRecoveryKey(undefined)).toBe("task:executorFailureConversationUnknown");
    expect(providerRecoveryKey("fresh")).toBe("task:executorFailureConversationFresh");
    expect(providerRecoveryKey("restored")).toBe("task:executorFailureConversationRestored");
  });
});

it("gives a session-only failure one matching composer owner", async () => {
  const module = (await import("./executor-failure")) as unknown as {
    executorFailureForOwner?: (
      episode: ExecutorFailureEpisode | undefined,
      sessionId?: string,
    ) => ExecutorFailureEpisode | null;
  };
  expect(module.executorFailureForOwner).toBeTypeOf("function");
  const choose = module.executorFailureForOwner!;
  expect(choose({ ...failure, session_id: "session" })).toBeNull();
  expect(choose({ ...failure, session_id: "session" }, "other")).toBeNull();
  expect(choose({ ...failure, session_id: "session" }, "session")?.id).toBe(failure.id);
  expect(choose(failure, "session")).toBeNull();
  expect(choose(failure)?.id).toBe(failure.id);
  expect(choose({ ...failure, state: "resolved" })).toBeNull();
});

it("labels a worker outage as connection uncertainty rather than a stopped agent", () => {
  expect(
    executorFailureTitleKey({
      ...failure,
      current_outcome: "unknown",
      observation: { ...failure.observation, outcome: "unknown", reason: "WorkerUnavailable" },
    }),
  ).toBe("task:executorFailureConnectionLost");
});

it("presents shared failures in session composers while fencing session-only failures", () => {
  expect(executorFailureForComposer(failure, "session")?.id).toBe(failure.id);
  expect(executorFailureForComposer(failure, null)).toBeNull();
  expect(executorFailureForComposer({ ...failure, session_id: "owner" }, "other")).toBeNull();
  expect(executorFailureForComposer({ ...failure, session_id: "owner" }, "owner")?.id).toBe(
    failure.id,
  );
  expect(executorFailureForComposer({ ...failure, state: "resolved" }, "session")).toBeNull();
});
