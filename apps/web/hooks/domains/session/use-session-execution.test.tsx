import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { useSessionExecution } from "./use-session-execution";

const instant = "2026-10-24T00:00:00Z";
function executionSession(id: string): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId("task"),
    state: "RUNNING",
    started_at: instant,
    updated_at: instant,
    execution_snapshot: {
      schema_version: 1,
      attempt_id: id,
      session_id: id,
      task_id: "task",
      execution_id: "execution",
      sequence: 1,
      generation: 1,
      executor_id: "executor",
      executor_type: "local",
      agent_profile_id: "profile",
      transport: "chat",
      wrapper: null,
      agent: { executable: "agent", omitted_arguments: 0, references: [] },
      captured_at: instant,
      sandbox: {
        attempt_id: id,
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
    },
    execution_observation: {
      schema_version: 1,
      observer_generation: 1,
      revision: 1,
      session_id: id,
      task_id: "task",
      snapshot_sequence: 1,
      attempt_id: id,
      generation: 7,
      attempt_version: 3,
      status: "active",
      provider_health: "available",
      reason: "",
      mechanism: "fixture-kernel",
      checked_at: instant,
      evidence_at: instant,
      valid_until: "2026-10-24T00:00:30Z",
    },
  };
}
const wrapper = ({ children }: { children: ReactNode }) => (
  <StateProvider>{children}</StateProvider>
);
afterEach(() => vi.useRealTimers());

it("expires evidence without traffic and isolates the selected session", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(instant));
  const view = renderHook(
    ({ id }) => ({ execution: useSessionExecution(id), store: useAppStoreApi() }),
    { initialProps: { id: "one" }, wrapper },
  );
  act(() => {
    view.result.current.store.getState().setConnectionStatus("connected");
    view.result.current.store
      .getState()
      .upsertTaskSessionFromEvent("task", executionSession("one"));
    const sibling = executionSession("two");
    sibling.execution_snapshot!.sandbox = null;
    sibling.execution_observation = null;
    view.result.current.store.getState().upsertTaskSessionFromEvent("task", sibling);
  });
  expect(view.result.current.execution.status).toBe("active");
  act(() => vi.advanceTimersByTime(30_000));
  expect(view.result.current.execution.status).toBe("unknown");
  expect(view.result.current.execution.session?.execution_observation?.evidence_at).toBe(instant);
  view.rerender({ id: "two" });
  expect(view.result.current.execution.status).toBe("not_configured");
  view.rerender({ id: "missing" });
  expect(view.result.current.execution.status).toBe("unknown");
  expect(view.result.current.execution.session).toBeUndefined();
  view.unmount();
});

it("reflects connection invalidation and later fresh observations", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(instant));
  const view = renderHook(
    () => ({ execution: useSessionExecution("one"), store: useAppStoreApi() }),
    { wrapper },
  );
  const update = (revision: number) => {
    const session = executionSession("one");
    session.execution_observation!.revision = revision;
    view.result.current.store.getState().upsertTaskSessionFromEvent("task", session);
  };
  act(() => {
    view.result.current.store.getState().setConnectionStatus("connected");
    update(1);
  });
  expect(view.result.current.execution.status).toBe("active");
  act(() => view.result.current.store.getState().setConnectionStatus("disconnected"));
  expect(view.result.current.execution.status).toBe("unknown");
  act(() => view.result.current.store.getState().setConnectionStatus("connected"));
  expect(view.result.current.execution.status).toBe("unknown");
  act(() => update(2));
  expect(view.result.current.execution.status).toBe("active");
  view.unmount();
});

it("removes active status at a deadline between whole-second clock ticks", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(Date.parse(instant) + 29_250));
  const view = renderHook(
    () => ({ execution: useSessionExecution("one"), store: useAppStoreApi() }),
    { wrapper },
  );
  act(() => {
    view.result.current.store.getState().setConnectionStatus("connected");
    view.result.current.store
      .getState()
      .upsertTaskSessionFromEvent("task", executionSession("one"));
  });
  expect(view.result.current.execution.status).toBe("active");
  act(() => vi.advanceTimersByTime(750));
  expect(view.result.current.execution.status).toBe("unknown");
  view.unmount();
});
