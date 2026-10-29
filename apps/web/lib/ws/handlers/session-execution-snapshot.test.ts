import { expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { ExecutionSnapshot, ExecutionObservation } from "@/lib/types/execution";
import { registerTaskSessionHandlers } from "./agent-session";

const stateChanged = "session.state_changed";
const initialTime = "2026-10-24T00:00:00Z";

const snapshot: ExecutionSnapshot = {
  schema_version: 1,
  attempt_id: "attempt",
  session_id: "session",
  task_id: "task",
  execution_id: "execution",
  generation: 1,
  sequence: 2,
  executor_id: "executor",
  executor_type: "local",
  agent_profile_id: "profile",
  transport: "chat",
  wrapper: null,
  agent: { executable: "agent", omitted_arguments: 2, references: [] },
  sandbox: null,
  captured_at: initialTime,
};

it("carries immutable execution history through live and reconnect messages", () => {
  const store = createAppStore();
  const handler = registerTaskSessionHandlers(store)[stateChanged]!;

  const emit = (value: ExecutionSnapshot | null | undefined, incarnation?: string) =>
    handler({
      id: "message",
      type: "notification",
      action: stateChanged,
      payload: {
        task_id: "task",
        session_id: "session",
        new_state: "RUNNING",
        updated_at: initialTime,
        ...(incarnation !== undefined ? { queue_incarnation_id: incarnation } : {}),
        ...(value !== undefined ? { execution_snapshot: value } : {}),
      },
    });
  emit(snapshot, "owner-one");
  const current = () => store.getState().taskSessions.items.session?.execution_snapshot;
  expect(current()).toEqual(snapshot);
  for (const stale of [undefined, null, { ...snapshot, sequence: 1 }]) {
    emit(stale);
    expect(current()).toEqual(snapshot);
  }
  emit({ ...snapshot, sequence: 3, session_id: "sibling" });
  expect(current()).toEqual(snapshot);
  const next = { ...snapshot, sequence: 3, attempt_id: "next", generation: 2 };
  emit(next);
  expect(current()).toEqual(next);
  const recreated = { ...snapshot, sequence: 1, attempt_id: "recreated" };
  emit(recreated, "owner-two");
  expect(current()).toEqual(recreated);
});

it("updates execution details without changing task state and rejects old session owners", () => {
  const store = createAppStore();
  const handlers = registerTaskSessionHandlers(store);
  handlers[stateChanged]!({
    id: "state",
    type: "notification",
    action: stateChanged,
    payload: {
      task_id: "task",
      session_id: "session",
      new_state: "WAITING_FOR_INPUT",
      queue_incarnation_id: "owner",
      updated_at: "2026-10-24T01:00:00Z",
    },
  });
  const handler = handlers["session.execution_changed"];
  expect(handler).toBeTypeOf("function");
  const emit = (owner = "owner", task = "task", value = snapshot) =>
    handler!({
      id: "execution",
      type: "notification",
      action: "session.execution_changed",
      payload: {
        task_id: task,
        session_id: "session",
        queue_incarnation_id: owner,
        execution_snapshot: value,
      },
    });
  emit();
  const current = () => store.getState().taskSessions.items.session!;
  expect(current().execution_snapshot).toEqual(snapshot);
  expect(current().state).toBe("WAITING_FOR_INPUT");
  expect(current().updated_at).toBe("2026-10-24T01:00:00Z");
  expect(store.getState().taskSessionsByTask.itemsByTaskId.task?.[0]?.execution_snapshot).toEqual(
    snapshot,
  );
  emit("previous-owner", "task", { ...snapshot, sequence: 50 });
  emit("owner", "foreign-task", { ...snapshot, sequence: 50 });
  emit("owner", "task", { ...snapshot, sequence: 1 });
  emit("owner", "task", { ...snapshot, sequence: 50, session_id: "sibling" });
  expect(current().execution_snapshot).toEqual(snapshot);
  const next = { ...snapshot, attempt_id: "next", sequence: 3 };
  emit("owner", "task", next);
  expect(current().execution_snapshot).toEqual(next);
});

const selected: ExecutionSnapshot = {
  ...snapshot,
  sandbox: {
    attempt_id: "attempt",
    generation: 7,
    installation_id: "installation",
    provider_key: "fixture",
    provider_version: "1",
    package_digest: "a".repeat(64),
    runtime_version: "1",
    runtime_digest: "b".repeat(64),
    revision_id: "policy",
    revision_number: 1,
    policy_name: "Policy",
    policy_digest: "c".repeat(64),
    selection_source: "task",
    ceiling_revision_id: "ceiling",
    ceiling_digest: "d".repeat(64),
  },
};
const observation: ExecutionObservation = {
  schema_version: 1,
  observer_generation: 2,
  revision: 3,
  session_id: "session",
  task_id: "task",
  snapshot_sequence: 2,
  attempt_id: "attempt",
  generation: 7,
  attempt_version: 4,
  status: "active",
  provider_health: "available",
  reason: "",
  mechanism: "fixture-kernel",
  checked_at: "2026-10-24T00:00:01Z",
  evidence_at: initialTime,
  valid_until: "2026-10-24T00:00:30Z",
};

it("carries observations through reconnect and independent notifications", () => {
  const store = createAppStore();
  const handlers = registerTaskSessionHandlers(store);
  handlers[stateChanged]!({
    id: "state",
    type: "notification",
    action: stateChanged,
    payload: {
      task_id: "task",
      session_id: "session",
      new_state: "RUNNING",
      queue_incarnation_id: "owner",
      execution_snapshot: selected,
      execution_observation: observation,
    },
  });
  const current = () => store.getState().taskSessions.items.session!;
  expect(current().execution_observation).toEqual(observation);
  const stopped = {
    ...observation,
    revision: 4,
    attempt_version: 5,
    status: "stopped" as const,
    valid_until: undefined,
  };
  const emit = (value: ExecutionObservation, owner = "owner") =>
    handlers["session.execution_changed"]!({
      id: "execution",
      type: "notification",
      action: "session.execution_changed",
      payload: {
        task_id: "task",
        session_id: "session",
        queue_incarnation_id: owner,
        execution_snapshot: selected,
        execution_observation: value,
      },
    });
  emit(stopped, "old-owner");
  expect(current().execution_observation).toEqual(observation);
  emit(stopped);
  expect(current().execution_observation).toEqual(stopped);
  expect(
    store.getState().taskSessionsByTask.itemsByTaskId.task?.[0]?.execution_observation,
  ).toEqual(stopped);
  emit({ ...observation, revision: 5 });
  expect(current().execution_observation).toEqual(stopped);
  expect(current().state).toBe("RUNNING");
});

it("invalidates live observations on connection loss until a newer server observation arrives", () => {
  const store = createAppStore();
  const handlers = registerTaskSessionHandlers(store);
  const emit = (value: ExecutionObservation) =>
    handlers[stateChanged]!({
      id: "state",
      type: "notification",
      action: stateChanged,
      payload: {
        task_id: "task",
        session_id: "session",
        new_state: "RUNNING",
        execution_snapshot: selected,
        execution_observation: value,
      },
    });
  const current = () => store.getState().taskSessions.items.session?.execution_observation;
  store.getState().setConnectionStatus("connected");
  emit(observation);
  expect(current()?.status).toBe("active");
  store.getState().setConnectionStatus("disconnected");
  const invalidated = current();
  expect(invalidated?.status).toBe("unknown");
  expect(invalidated?.evidence_at).toBe(observation.evidence_at);
  expect(invalidated?.valid_until).toBeUndefined();
  expect(
    store.getState().taskSessionsByTask.itemsByTaskId.task?.[0]?.execution_observation?.status,
  ).toBe("unknown");
  store.getState().setConnectionStatus("connected");
  emit(observation);
  expect(current()?.status).toBe("unknown");
  emit({ ...observation, revision: 4 });
  expect(current()?.status).toBe("active");
  store.getState().setConnectionStatus("reconnecting");
  emit({ ...observation, revision: 5 });
  store.getState().setConnectionStatus("connected");
  expect(current()?.status).toBe("unknown");
  emit({ ...observation, revision: 6 });
  expect(current()?.status).toBe("active");
});
