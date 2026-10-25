import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { SessionExecutionControl } from "./session-execution-indicator";

const instant = "2026-10-24T00:00:00Z";
function fixture(): TaskSession {
  return {
    id: sessionId("one"),
    task_id: taskId("task"),
    name: "First session",
    state: "RUNNING",
    started_at: instant,
    updated_at: instant,
    execution_snapshot: {
      schema_version: 1,
      attempt_id: "attempt",
      session_id: "one",
      task_id: "task",
      execution_id: "execution",
      generation: 1,
      sequence: 1,
      executor_id: "executor",
      executor_type: "local",
      agent_profile_id: "profile",
      transport: "acp",
      captured_at: instant,
      agent: { executable: "agent", omitted_arguments: 2, references: [] },
      wrapper: {
        source: "agent_profile",
        digest: "a".repeat(64),
        command: { executable: "nono", omitted_arguments: 3, references: [] },
      },
      sandbox: null,
    },
  };
}
let store: ReturnType<typeof useAppStoreApi>;
function Controls({ onTabAction }: { onTabAction: () => void }) {
  store = useAppStoreApi();
  return (
    <div onClick={onTabAction} onDoubleClick={onTabAction} onPointerDown={onTabAction}>
      <SessionExecutionControl sessionId="one" />
      <SessionExecutionControl sessionId="two" labelled />
    </div>
  );
}
function mount() {
  const onTabAction = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <StateProvider>
        <Controls onTabAction={onTabAction} />
      </StateProvider>
    </QueryClientProvider>,
  );
  act(() => {
    store.getState().setConnectionStatus("connected");
    store.getState().upsertTaskSessionFromEvent("task", fixture());
  });
  return onTabAction;
}
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("shows changed saved wrapper settings without rewriting the captured launch", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          supported: true,
          composition: {
            agent: { executable: "next-agent", omitted_arguments: 2, references: [] },
            wrapper: {
              source: "agent_profile",
              digest: "b".repeat(64),
              command: { executable: "next-wrapper", omitted_arguments: 1, references: [] },
            },
          },
        }),
        { headers: { "Content-Type": "application/json" } },
      ),
    ),
  );
  mount();
  fireEvent.click(screen.getByRole("button", { name: /First session.*Not configured/ }));
  const dialog = await screen.findByRole("dialog", { name: "Execution details" });
  await within(dialog).findByText("next-wrapper");
  expect(within(dialog).getByText("nono")).toBeTruthy();
  expect(within(dialog).getByText("The wrapper differs from this attempt.")).toBeTruthy();
  expect(
    store.getState().taskSessions.items.one.execution_snapshot?.wrapper?.command.executable,
  ).toBe("nono");
});

it("opens the captured wrapper without claiming confinement or activating the tab", async () => {
  const onTabAction = mount();
  const button = screen.getByRole("button", { name: /First session.*Not configured/ });
  expect(button.querySelector(".tabler-icon-shield-check")).toBeNull();
  fireEvent.pointerDown(button, { button: 0 });
  fireEvent.click(button);
  fireEvent.doubleClick(button);
  const dialog = await screen.findByRole("dialog", { name: "Execution details" });
  expect(within(dialog).getByText("nono")).toBeTruthy();
  expect(within(dialog).getByText(/3 arguments hidden/)).toBeTruthy();
  expect(within(dialog).getByText(/User terminals, setup commands/)).toBeTruthy();
  expect(onTabAction).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(document.activeElement).toBe(button));
});

it("keeps legacy and sibling history unknown", async () => {
  mount();
  fireEvent.click(screen.getByRole("button", { name: /two.*Unknown/ }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("No launch record is available for this session.")).toBeTruthy();
  expect(within(dialog).queryByText("nono")).toBeNull();
});

it("removes the shield at evidence expiry while retaining captured policy history", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(instant));
  mount();
  const session = fixture();
  session.execution_snapshot!.sandbox = {
    attempt_id: "attempt",
    generation: 2,
    installation_id: "install",
    provider_key: "fixture",
    provider_version: "1",
    package_digest: "a".repeat(64),
    runtime_version: "2",
    runtime_digest: "b".repeat(64),
    revision_id: "revision",
    revision_number: 3,
    policy_name: "Captured policy",
    policy_digest: "c".repeat(64),
    selection_source: "task",
    ceiling_revision_id: "ceiling",
    ceiling_digest: "d".repeat(64),
  };
  session.execution_observation = {
    schema_version: 1,
    observer_generation: 1,
    revision: 1,
    session_id: "one",
    task_id: "task",
    snapshot_sequence: 1,
    attempt_id: "attempt",
    generation: 2,
    attempt_version: 2,
    status: "active",
    provider_health: "available",
    reason: "",
    mechanism: "fixture-kernel",
    checked_at: instant,
    evidence_at: instant,
    valid_until: "2026-10-24T00:00:30Z",
  };
  // A new immutable launch replaces the earlier wrapper-only record.
  session.execution_snapshot!.sequence = 2;
  session.execution_observation.snapshot_sequence = 2;
  act(() => store.getState().upsertTaskSessionFromEvent("task", session));
  const button = screen.getByRole("button", { name: /First session.*Active/ });
  expect(button.querySelector(".tabler-icon-shield-check")).toBeTruthy();
  act(() => vi.advanceTimersByTime(30_000));
  expect(
    screen
      .getByRole("button", { name: /First session.*Unknown/ })
      .querySelector(".tabler-icon-shield-check"),
  ).toBeNull();
});
