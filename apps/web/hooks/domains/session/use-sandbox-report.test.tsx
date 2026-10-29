import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { useSandboxReport } from "./use-sandbox-report";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
const fetchSpy = vi.fn<typeof fetch>();
let store: ReturnType<typeof useAppStoreApi>;
const capturedAt = "2026-10-24T00:00:00Z";
const user = {
  id: "operator",
  email: "operator@example.test",
  display_name: "Operator",
  role: "admin",
  status: "active",
};
function session(): TaskSession {
  return {
    id: sessionId("session"),
    task_id: taskId("task"),
    state: "RUNNING",
    started_at: capturedAt,
    updated_at: capturedAt,
    queue_incarnation_id: "owner",
    execution_snapshot: {
      schema_version: 1,
      attempt_id: "attempt",
      session_id: "session",
      task_id: "task",
      execution_id: "execution",
      generation: 1,
      sequence: 1,
      executor_id: "local",
      executor_type: "local",
      agent_profile_id: "profile",
      transport: "acp",
      captured_at: capturedAt,
      agent: { executable: "agent", omitted_arguments: 0, references: [] },
      wrapper: null,
      sandbox: {
        attempt_id: "attempt",
        generation: 1,
        installation_id: "installation",
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
      },
    },
  };
}
function ReadStore({ children }: { children: ReactNode }) {
  store = useAppStoreApi();
  return children;
}
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <StateProvider
        initialState={{
          auth: { mode: "enabled", authenticated: true, user },
          features: { ...defaultState.features, sandboxPolicies: true },
          taskSessions: { items: { session: session() } },
        }}
      >
        <ReadStore>{children}</ReadStore>
      </StateProvider>
    </QueryClientProvider>
  );
}
function response(value: unknown) {
  return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
}
function connect() {
  act(() => store.getState().setConnectionStatus("connected"));
}
beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function report(): SandboxReport {
  return {
    schema_version: 1,
    attempt_id: "attempt",
    session_id: "session",
    revision_id: "revision",
    policy_digest: "c".repeat(64),
    ceiling_digest: "d".repeat(64),
    version: 1,
    digest: "e".repeat(64),
    captured_at: capturedAt,
    provider: {
      plugin_id: "plugin",
      installation_id: "installation",
      key: "fixture",
      version: "1",
      package_digest: "a".repeat(64),
      runtime_version: "1",
      runtime_digest: "b".repeat(64),
    },
    data: {
      status: "available",
      integrity: "partial",
      complete: false,
      finalized: false,
      supported_classes: ["filesystem"],
      sources: [],
      records: [],
    },
  };
}

it("reads a bound report without caching credentials or retrying failed refreshes", async () => {
  fetchSpy.mockImplementation(async () => response(report()));
  const { result } = renderHook(() => useSandboxReport("session"), { wrapper });
  expect(fetchSpy).not.toHaveBeenCalled();
  connect();
  await waitFor(() => expect(result.current.report).toEqual(report()));
  expect(fetchSpy.mock.calls[0][0]).toContain("/sessions/session/attempts/attempt/report");
  expect(fetchSpy.mock.calls[0][1]?.cache).toBe("no-store");
  fetchSpy.mockImplementation(async () => new Response("{}", { status: 503 }));
  await act(async () => {
    await result.current.refresh();
  });
  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(result.current.report).toEqual(report());
  expect(fetchSpy).toHaveBeenCalledTimes(2);
});

it.each(["attempt_id", "session_id", "policy_digest", "ceiling_digest", "revision_id"] as const)(
  "rejects a foreign %s",
  async (key) => {
    fetchSpy.mockImplementation(async () => response({ ...report(), [key]: "foreign" }));
    const { result } = renderHook(() => useSandboxReport("session"), { wrapper });
    connect();
    await waitFor(() => expect(result.current.isPending).toBe(false));
    expect(result.current.report).toBeNull();
    expect(result.current.isError).toBe(true);
  },
);

it("discards late reads after disconnect or an account change", async () => {
  const pending: Array<(value: Response) => void> = [];
  fetchSpy.mockImplementation(() => new Promise((resolve) => pending.push(resolve)));
  const { result } = renderHook(() => useSandboxReport("session"), { wrapper });
  connect();
  await waitFor(() => expect(pending).toHaveLength(1));
  act(() => store.getState().setConnectionStatus("disconnected"));
  await act(async () => pending[0](response(report())));
  expect(result.current.report).toBeNull();
  connect();
  await waitFor(() => expect(pending).toHaveLength(2));
  act(() =>
    store
      .getState()
      .setAuthState({ mode: "enabled", authenticated: true, user: { ...user, id: "replacement" } }),
  );
  await waitFor(() => expect(pending).toHaveLength(3));
  await act(async () => pending[1](response(report())));
  expect(result.current.report).toBeNull();
  await act(async () => pending[2](response(report())));
  await waitFor(() => expect(result.current.report).toEqual(report()));
});
