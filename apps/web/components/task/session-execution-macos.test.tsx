import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import type { ExecutionSandboxSnapshot } from "@/lib/types/execution";
import { SessionExecutionControl } from "./session-execution-indicator";

const instant = "2026-10-24T00:00:00Z";
type Scope = ExecutionSandboxSnapshot["ownership_scope"];

function session(scope: Scope): TaskSession {
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
      agent: { executable: "agent", omitted_arguments: 0, references: [] },
      wrapper: null,
      sandbox: {
        attempt_id: "attempt",
        generation: 1,
        installation_id: "install",
        provider_key: "nono",
        provider_version: "1",
        package_digest: "a".repeat(64),
        runtime_version: "0.80.0",
        runtime_digest: "b".repeat(64),
        revision_id: "revision",
        revision_number: 1,
        policy_name: "Work",
        policy_digest: "c".repeat(64),
        selection_source: "task",
        ceiling_revision_id: "ceiling",
        ceiling_digest: "d".repeat(64),
        ...(scope ? { ownership_scope: scope } : {}),
      },
    },
  };
}

function Probe({ value }: { value: TaskSession }) {
  const store = useAppStoreApi();
  store.getState().setConnectionStatus("connected");
  store.getState().upsertTaskSessionFromEvent("task", value);
  return <SessionExecutionControl sessionId="one" />;
}

async function openDetails(scope: Scope) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(async () => new Response("{}", { status: 404 })),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <StateProvider>
        <Probe value={session(scope)} />
      </StateProvider>
    </QueryClientProvider>,
  );
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: /First session/ }));
  });
  return screen.findByRole("dialog", { name: "Execution details" });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("states the process-group owner, unsupported approvals and unavailable denials on macOS", async () => {
  const dialog = await openDetails("process_tree");
  expect(within(dialog).getByText("Process ownership")).toBeTruthy();
  expect(
    within(dialog).getByText(/Only agentctl stops it; there is no kernel container/),
  ).toBeTruthy();
  expect(within(dialog).getByText(/Approvals are not supported on macOS/)).toBeTruthy();
  expect(within(dialog).getByText(/Filesystem denials are unavailable on macOS/)).toBeTruthy();
  expect(within(dialog).queryByTestId("sandbox-denial-report")).toBeNull();
  expect(within(dialog).queryByRole("button", { name: "Refresh report" })).toBeNull();
  expect(within(dialog).queryByText(/Approval records are unavailable/)).toBeNull();
});

it.each<Scope>(["kernel_container", undefined])(
  "keeps the Linux presentation for scope %s",
  async (scope) => {
    const dialog = await openDetails(scope);
    expect(within(dialog).getByText("Kernel container")).toBeTruthy();
    expect(within(dialog).queryByText(/Approvals are not supported on macOS/)).toBeNull();
    expect(within(dialog).queryByText(/Filesystem denials are unavailable on macOS/)).toBeNull();
    expect(within(dialog).getByTestId("sandbox-denial-report")).toBeTruthy();
  },
);
