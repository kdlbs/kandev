import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { useSandboxLearning } from "./use-sandbox-learning";
import type { SandboxLearningProposal } from "@/lib/api/domains/sandbox-learning-api";
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

const request = { name: "Reviewed policy", scope: "workspace" as const, record_ids: ["denial"] };
function proposal(): SandboxLearningProposal {
  return {
    id: "proposal",
    schema_version: 1,
    attempt_id: "attempt",
    session_id: "session",
    report_digest: report().digest,
    report_version: 1,
    report_sources: [],
    report_integrity: "partial",
    report_complete: false,
    base_revision_id: "revision",
    base_digest: "c".repeat(64),
    ceiling_revision_id: "ceiling",
    ceiling_digest: "d".repeat(64),
    provider: report().provider,
    executor_id: "local",
    binding_version: 1,
    selected_records: [],
    added_grants: {
      filesystem: [{ path: "/workspace/file", writable: true }],
      network: [],
      kandev_mcp: false,
    },
    policy: {
      id: "learned",
      workspace_id: "workspace",
      name: request.name,
      installation_id: "installation",
      provider_key: "fixture",
      sources: [],
      grants: { filesystem: [], network: [], kandev_mcp: false },
    },
    operator_id: "operator",
    created_at: capturedAt,
    digest: "f".repeat(64),
  };
}

it("requires an explicit second action to publish the exact reviewed proposal", async () => {
  fetchSpy.mockImplementation(async (url) =>
    response(
      String(url).endsWith("/publish")
        ? { id: "published", policy: proposal().policy }
        : proposal(),
    ),
  );
  const { result } = renderHook(() => useSandboxLearning("session", report()), { wrapper });
  connect();
  expect(result.current.canPropose).toBe(true);
  expect(result.current.canPublish).toBe(false);
  await act(async () => {
    expect(await result.current.propose(request)).toBe(true);
  });
  await waitFor(() => expect(result.current.proposal?.id).toBe("proposal"));
  expect(fetchSpy).toHaveBeenCalledTimes(1);
  expect(JSON.parse(String(fetchSpy.mock.calls[0][1]?.body))).toEqual({
    ...request,
    report_digest: report().digest,
  });
  await act(async () => {
    expect(await result.current.publish()).toBe(true);
  });
  await waitFor(() => expect(result.current.revision?.id).toBe("published"));
  expect(JSON.parse(String(fetchSpy.mock.calls[1][1]?.body))).toEqual({
    expected_digest: proposal().digest,
  });
  expect(result.current.canPublish).toBe(false);
});

it("rejects a queued action after operator authority changes", async () => {
  fetchSpy.mockImplementation(async () => response(proposal()));
  const { result } = renderHook(() => useSandboxLearning("session", report()), { wrapper });
  connect();
  let sent: Promise<boolean>;
  act(() => {
    sent = result.current.propose(request);
    store
      .getState()
      .setAuthState({ mode: "enabled", authenticated: true, user: { ...user, role: "member" } });
  });
  await act(async () => {
    expect(await sent).toBe(false);
  });
  expect(fetchSpy).not.toHaveBeenCalled();
  expect(result.current.canPropose).toBe(false);
});

it("discards a late proposal for a replaced report", async () => {
  let resolve: (value: Response) => void = () => {};
  fetchSpy.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result, rerender } = renderHook(({ value }) => useSandboxLearning("session", value), {
    wrapper,
    initialProps: { value: report() },
  });
  connect();
  let sent: Promise<boolean>;
  act(() => {
    sent = result.current.propose(request);
  });
  await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
  rerender({ value: { ...report(), digest: "new-report", version: 2 } });
  await act(async () => {
    resolve(response(proposal()));
    await sent;
  });
  expect(result.current.proposal).toBeNull();
  expect(result.current.canPublish).toBe(false);
});

it("sends one publication for duplicate clicks and does not retry a conflict", async () => {
  fetchSpy.mockImplementation(async (url) =>
    String(url).endsWith("/publish") ? new Response("{}", { status: 409 }) : response(proposal()),
  );
  const { result } = renderHook(() => useSandboxLearning("session", report()), { wrapper });
  connect();
  await act(async () => {
    await result.current.propose(request);
  });
  await waitFor(() => expect(result.current.canPublish).toBe(true));
  await act(async () => {
    expect(await Promise.all([result.current.publish(), result.current.publish()])).toEqual([
      false,
      false,
    ]);
  });
  await waitFor(() => expect(result.current.failed).toBe(true));
  expect(fetchSpy).toHaveBeenCalledTimes(2);
  expect(result.current.canPublish).toBe(false);
});
