import { describe, expect, it } from "vitest";
import { mergeTaskSession } from "./session-merge";
import type { ExecutionSnapshot, ExecutionObservation } from "@/lib/types/execution";
import type { TaskSession } from "@/lib/types/http";

function snapshot(sequence: number): ExecutionSnapshot {
  return {
    schema_version: 1,
    attempt_id: `attempt-${sequence}`,
    session_id: "session",
    task_id: "task",
    execution_id: "execution",
    generation: sequence,
    sequence,
    executor_id: "executor",
    executor_type: "local",
    agent_profile_id: "profile",
    transport: "chat",
    wrapper: null,
    agent: { executable: "agent", omitted_arguments: 1, references: [] },
    sandbox: null,
    captured_at: capturedAt,
  };
}

function session(executionSnapshot?: ExecutionSnapshot | null): TaskSession {
  return {
    id: "session",
    task_id: "task",
    state: "RUNNING",
    started_at: capturedAt,
    updated_at: capturedAt,
    ...(executionSnapshot !== undefined ? { execution_snapshot: executionSnapshot } : {}),
  } as TaskSession;
}

describe("immutable execution history reconciliation", () => {
  it("preserves known history across older, partial, and unknown refreshes", () => {
    const current = session(snapshot(3));
    for (const incoming of [session(snapshot(2)), session(), session(null)]) {
      expect(mergeTaskSession(current, incoming).execution_snapshot).toEqual(snapshot(3));
    }
  });

  it("accepts a new launch across an execution replacement", () => {
    const newer = { ...snapshot(4), execution_id: "replacement", generation: 1 };
    expect(mergeTaskSession(session(snapshot(3)), session(newer)).execution_snapshot).toEqual(
      newer,
    );
  });

  it.each([
    { session_id: "sibling" },
    { task_id: "foreign" },
    { sequence: 0 },
    { generation: 0 },
    { sequence: Number.NaN },
    { schema_version: 2 },
  ])("rejects foreign or invalid history: %j", (patch) => {
    const incoming = { ...snapshot(4), ...patch } as ExecutionSnapshot;
    expect(mergeTaskSession(session(snapshot(3)), session(incoming)).execution_snapshot).toEqual(
      snapshot(3),
    );
  });

  it("does not rewrite an already captured sequence", () => {
    const changed = { ...snapshot(3), agent_profile_id: "later-settings" };
    expect(mergeTaskSession(session(snapshot(3)), session(changed)).execution_snapshot).toEqual(
      snapshot(3),
    );
  });

  it("admits valid first history and leaves legacy history unknown", () => {
    expect(mergeTaskSession(session(), session(snapshot(1))).execution_snapshot).toEqual(
      snapshot(1),
    );
    expect(mergeTaskSession(session(), session(null)).execution_snapshot).toBeNull();
  });

  it("discards history when a session identifier is recreated", () => {
    const previous = { ...session(snapshot(3)), queue_incarnation_id: "old-owner" };
    const recreated = { ...session(null), queue_incarnation_id: "new-owner" };
    expect(mergeTaskSession(previous, recreated).execution_snapshot).toBeNull();
    const captured = { ...session(snapshot(1)), queue_incarnation_id: "new-owner" };
    expect(mergeTaskSession(previous, captured).execution_snapshot).toEqual(snapshot(1));
  });
});

function observation(revision = 1): ExecutionObservation {
  return {
    schema_version: 1,
    observer_generation: 1,
    revision,
    session_id: "session",
    task_id: "task",
    snapshot_sequence: 1,
    attempt_id: "attempt-1",
    generation: 7,
    attempt_version: 3,
    status: "active",
    provider_health: "available",
    reason: "",
    mechanism: "fixture-kernel",
    checked_at: "2026-10-24T00:00:01Z",
    evidence_at: capturedAt,
    valid_until: "2026-10-24T00:00:30Z",
  };
}

function sandboxSession(): TaskSession {
  const captured = snapshot(1);
  captured.sandbox = {
    attempt_id: "attempt-1",
    generation: 7,
    installation_id: "installed",
    provider_key: "fixture",
    provider_version: "1",
    package_digest: "a".repeat(64),
    runtime_version: "1",
    runtime_digest: "b".repeat(64),
    revision_id: "revision",
    revision_number: 1,
    policy_name: "Policy",
    policy_digest: "c".repeat(64),
    selection_source: "task",
    ceiling_revision_id: "ceiling",
    ceiling_digest: "d".repeat(64),
  };
  return {
    ...session(captured),
    queue_incarnation_id: "owner",
    execution_observation: observation(3),
  };
}

describe("execution observation reconciliation", () => {
  it("retains current observations across partial and old messages", () => {
    const current = sandboxSession();
    for (const incoming of [undefined, null, observation(2)]) {
      expect(
        mergeTaskSession(current, { ...session(), execution_observation: incoming })
          .execution_observation,
      ).toEqual(current.execution_observation);
    }
  });
  it.each([
    { session_id: "sibling" },
    { task_id: "foreign" },
    { attempt_id: "other" },
    { generation: 8 },
    { snapshot_sequence: 2 },
    { observer_generation: 0 },
    { revision: Number.NaN },
  ])("rejects unbound observations: %j", (patch) => {
    const current = sandboxSession();
    expect(
      mergeTaskSession(current, {
        ...session(),
        execution_observation: { ...observation(4), ...patch },
      }).execution_observation,
    ).toEqual(current.execution_observation);
  });
  it("orders backend generations before revisions", () => {
    const current = sandboxSession();
    const replacement = {
      ...observation(1),
      observer_generation: 2,
      status: "unknown" as const,
      valid_until: undefined,
    };
    const merged = mergeTaskSession(current, { ...session(), execution_observation: replacement });
    expect(merged.execution_observation).toEqual(replacement);
    expect(
      mergeTaskSession(merged, { ...session(), execution_observation: observation(999) })
        .execution_observation,
    ).toEqual(replacement);
  });
  it("does not restore active status after a terminal observation", () => {
    const current = sandboxSession();
    const stopped = { ...observation(4), status: "stopped" as const, valid_until: undefined };
    current.execution_observation = stopped;
    expect(
      mergeTaskSession(current, { ...session(), execution_observation: observation(5) })
        .execution_observation,
    ).toEqual(stopped);
  });
  it("clears old evidence when a new launch has no observation yet", () => {
    const current = sandboxSession();
    expect(mergeTaskSession(current, session(snapshot(2))).execution_observation).toBeNull();
    expect(
      mergeTaskSession(current, { ...session(), queue_incarnation_id: "recreated" })
        .execution_observation,
    ).toBeNull();
  });
});

const capturedAt = "2026-10-24T00:00:00Z";
