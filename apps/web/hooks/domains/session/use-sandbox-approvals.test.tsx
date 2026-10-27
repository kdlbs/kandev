import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import type { SandboxApproval } from "@/lib/api/domains/sandbox-approvals-api";
import { registerTaskSessionHandlers } from "@/lib/ws/handlers/agent-session";
import { useSandboxApprovals } from "./use-sandbox-approvals";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
const fetchSpy = vi.fn<typeof fetch>();
let store: ReturnType<typeof useAppStoreApi>;
const user = {
  id: "operator",
  email: "operator@example.test",
  display_name: "Operator",
  role: "admin",
  status: "active",
};
const approval: SandboxApproval = {
  request_id: "request",
  attempt_id: "attempt",
  session_id: "session",
  version: 1,
  state: "pending",
  access: "read",
  target: "/workspace/file",
  policy_digest: "c".repeat(64),
  expires_at: "2099-01-01T00:00:00Z",
  can_decide: true,
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
        policy_digest: approval.policy_digest,
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

it("expires each pending request at its own deadline", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(new Date(capturedAt));
  const early = { ...approval, expires_at: "2026-10-24T00:00:01Z" };
  const later = { ...approval, request_id: "later", expires_at: "2026-10-24T00:00:02Z" };
  fetchSpy.mockImplementation(async () => response([early, later]));
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canDecide(early)).toBe(true));
  await act(async () => vi.advanceTimersByTimeAsync(1000));
  expect(result.current.canDecide(early)).toBe(false);
  expect(result.current.canDecide(later)).toBe(true);
  await act(async () => vi.advanceTimersByTimeAsync(1000));
  expect(result.current.canDecide(later)).toBe(false);
});

it("keeps member reads read-only even if an old response offers a decision", async () => {
  fetchSpy.mockImplementation(async () => response([approval]));
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  act(() =>
    store
      .getState()
      .setAuthState({ mode: "enabled", authenticated: true, user: { ...user, role: "member" } }),
  );
  await waitFor(() => expect(result.current.approvals).toEqual([approval]));
  expect(result.current.canDecide(approval)).toBe(false);
  await act(async () => {
    expect(await result.current.decide(approval, "allow_once")).toBe(false);
  });
  expect(fetchSpy.mock.calls.every(([, init]) => init?.method !== "POST")).toBe(true);
});

it("settles a failed decision without retrying it", async () => {
  fetchSpy.mockImplementation(async (_url, init) =>
    init?.method === "POST"
      ? new Response(JSON.stringify({ error: "sandbox_revision_conflict" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        })
      : response([approval]),
  );
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canDecide(approval)).toBe(true));
  await act(async () => {
    expect(await result.current.decide(approval, "deny")).toBe(false);
  });
  await waitFor(() => expect(result.current.isBusy).toBe(false));
  expect(result.current.decisionFailed).toBe(true);
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
});

it("shares pending reads, filters foreign attempts, and refreshes after a live event", async () => {
  fetchSpy.mockImplementation(async () =>
    response([approval, { ...approval, request_id: "foreign", attempt_id: "foreign" }]),
  );
  const { result } = renderHook(
    () => [useSandboxApprovals("session"), useSandboxApprovals("session")],
    { wrapper },
  );
  connect();
  await waitFor(() => expect(result.current[0].approvals).toEqual([approval]));
  expect(result.current[1].canDecide(approval)).toBe(true);
  expect(fetchSpy).toHaveBeenCalledOnce();
  fetchSpy.mockImplementation(async () =>
    response([{ ...approval, state: "sent", version: 4, can_decide: false }]),
  );
  act(() =>
    registerTaskSessionHandlers(store)["session.sandbox_approvals_changed"]!({
      id: "event",
      type: "notification",
      action: "session.sandbox_approvals_changed",
      payload: { task_id: "task", session_id: "session", queue_incarnation_id: "owner" },
    }),
  );
  await waitFor(() => expect(result.current[0].approvals[0]?.state).toBe("sent"));
  expect(result.current[1].canDecide(approval)).toBe(false);
});

it("refreshes an event received while the initial empty read is still pending", async () => {
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  fetchSpy.mockImplementation(async () => response([approval]));
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce());
  act(() =>
    registerTaskSessionHandlers(store)["session.sandbox_approvals_changed"]!({
      id: "event",
      type: "notification",
      action: "session.sandbox_approvals_changed",
      payload: { task_id: "task", session_id: "session", queue_incarnation_id: "owner" },
    }),
  );
  await act(async () => finish(response([])));
  await waitFor(() => expect(result.current.approvals).toEqual([approval]));
  expect(result.current.canDecide(approval)).toBe(true);
});

it("requires a fresh read after reconnect and hides signed-out deferred responses", async () => {
  fetchSpy.mockImplementation(async () => response([approval]));
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canDecide(approval)).toBe(true));
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  act(() => {
    store.getState().setConnectionStatus("disconnected");
    store.getState().setConnectionStatus("connected");
  });
  expect(result.current.canDecide(approval)).toBe(false);
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(2));
  act(() => store.getState().clearAuthenticated());
  await act(async () => finish(response([approval])));
  expect(result.current.approvals).toEqual([]);
  expect(result.current.canDecide(approval)).toBe(false);
});

it("rechecks authority before a queued decision reaches the network", async () => {
  fetchSpy.mockImplementation(async () => response([approval]));
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canDecide(approval)).toBe(true));
  let submitted!: Promise<boolean>;
  act(() => {
    submitted = result.current.decide(approval, "allow_once");
    store.getState().clearAuthenticated();
  });
  await act(async () => expect(await submitted).toBe(false));
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(0);
  expect(result.current.approvals).toEqual([]);
});

it("isolates a late decision failure from the replacement operator", async () => {
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementation(async (_url, init) =>
    init?.method === "POST"
      ? new Promise((resolve) => {
          finish = resolve;
        })
      : response([approval]),
  );
  const { result } = renderHook(() => useSandboxApprovals("session"), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canDecide(approval)).toBe(true));
  let submitted!: Promise<boolean>;
  act(() => {
    submitted = result.current.decide(approval, "deny");
  });
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  act(() =>
    store.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: { ...user, id: "replacement" },
    }),
  );
  await waitFor(() => expect(result.current.canDecide(approval)).toBe(true));
  await act(async () => {
    finish(new Response("{}", { status: 409 }));
    expect(await submitted).toBe(false);
  });
  expect(result.current.isBusy).toBe(false);
  expect(result.current.failedRequestId).toBeUndefined();
  expect(result.current.decisionFailed).toBe(false);
});

it("submits one explicit versioned decision across two consumers without retries", async () => {
  let finish!: (value: Response) => void;
  fetchSpy.mockImplementation(async (_url, init) =>
    init?.method === "POST"
      ? new Promise((resolve) => {
          finish = resolve;
        })
      : response([approval]),
  );
  const { result } = renderHook(
    () => [useSandboxApprovals("session"), useSandboxApprovals("session")],
    { wrapper },
  );
  connect();
  await waitFor(() => expect(result.current[0].canDecide(approval)).toBe(true));
  let first!: Promise<unknown>, second!: Promise<unknown>;
  act(() => {
    first = result.current[0].decide(approval, "allow_once");
    second = result.current[1].decide(approval, "allow_once");
  });
  await waitFor(() =>
    expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1),
  );
  const [url, init] = fetchSpy.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(String(url)).toBe(
    "http://api.test/api/v1/sandbox/attempts/attempt/approvals/request/decision",
  );
  expect(JSON.parse(String(init?.body))).toEqual({ expected_version: 1, decision: "allow_once" });
  expect(result.current.every((model) => model.isBusy)).toBe(true);
  await act(async () => {
    finish(response({ ...approval, state: "approved", version: 2, can_decide: false }));
    await Promise.all([first, second]);
  });
  await waitFor(() => expect(result.current.every((model) => !model.isBusy)).toBe(true));
});

const capturedAt = "2026-10-24T00:00:00Z";
