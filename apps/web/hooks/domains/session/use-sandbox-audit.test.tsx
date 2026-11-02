import { createHash, generateKeyPairSync } from "node:crypto";
import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { sessionId, taskId } from "@/lib/types/ids";
import type { TaskSession } from "@/lib/types/http";
import { useSandboxAudit } from "./use-sandbox-audit";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
const fetchSpy = vi.fn<typeof fetch>();
let store: ReturnType<typeof useAppStoreApi>;
const trustSuffix = "/audit/trust";
const trustRevisionID = "d328b6f3-6744-4302-ae75-d153db19c804";
const verificationsSuffix = "/audit/verifications";
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
      finalized: true,
      final_audit: {
        binding_digest: "f".repeat(64),
        artifact_set_digest: "d".repeat(64),
        captured_at: capturedAt,
      },
      supported_classes: ["filesystem"],
      sources: [],
      records: [],
    },
  };
}

const trust = {
  schema_version: 1,
  id: "",
  version: 0,
  org_id: "org",
  workspace_id: "",
  installation_id: "installation",
  provider_key: "fixture",
  fingerprint: "",
  public_key_pem: "",
  revoked: true,
  operator_id: "",
  source: "unconfigured",
  created_at: capturedAt,
  digest: "",
};
function readAudit(request: string | URL | Request) {
  const url = String(request);
  if (url.endsWith(trustSuffix)) return Promise.resolve(response(trust));
  if (url.endsWith(verificationsSuffix)) return Promise.resolve(response({ verifications: [] }));
  return Promise.resolve(new Response("", { status: 404 }));
}
it("loads final audit trust and history without starting verification", async () => {
  fetchSpy.mockImplementation(readAudit);
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  expect(result.current.trust).toEqual(trust);
  expect(fetchSpy.mock.calls.every(([, init]) => !init?.method || init.method === "GET")).toBe(
    true,
  );
});

it("refuses a trust response from another provider installation", async () => {
  fetchSpy.mockImplementation((request) =>
    String(request).endsWith(trustSuffix)
      ? Promise.resolve(response({ ...trust, installation_id: "foreign" }))
      : readAudit(request),
  );
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.isPending).toBe(false));
  expect(result.current.failed).toBe(true);
  expect(result.current.canVerify).toBe(false);
  expect(result.current.trust).toBeNull();
});

it("refuses audit history rebound to another session or artifact", async () => {
  fetchSpy.mockImplementation((request) =>
    String(request).endsWith(verificationsSuffix)
      ? Promise.resolve(
          response({
            verifications: [
              {
                verification: {
                  schema_version: 1,
                  id: "58273fe0-cdae-4849-b359-4802a2b76615",
                  version: 2,
                  state: "failed",
                  attempt_id: "attempt",
                  session_id: "foreign",
                  report_digest: report().digest,
                  artifact_set_digest: "0".repeat(64),
                  binding_digest: report().data.final_audit!.binding_digest,
                  provider: report().provider,
                },
                signer_trusted_now: false,
              },
            ],
          }),
        )
      : readAudit(request),
  );
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.isPending).toBe(false));
  expect(result.current.failed).toBe(true);
  expect(result.current.canVerify).toBe(false);
  expect(result.current.history).toEqual([]);
});

function failedJob() {
  const captured = report();
  return {
    verification: {
      schema_version: 1,
      id: "58273fe0-cdae-4849-b359-4802a2b76615",
      version: 2,
      state: "failed" as const,
      attempt_id: captured.attempt_id,
      session_id: captured.session_id,
      provider_session_id: "provider-session",
      report_digest: captured.digest,
      artifact_set_digest: captured.data.final_audit!.artifact_set_digest,
      binding_digest: captured.data.final_audit!.binding_digest,
      provider: captured.provider,
      trust: {
        revision_id: "",
        version: 0,
        fingerprint: "",
        revoked: true,
        source: "unconfigured",
        operator_id: "",
        configured_at: capturedAt,
      },
      requested_by: "operator",
      requested_at: capturedAt,
      completed_at: capturedAt,
      failure_code: "unavailable" as const,
    },
    signer_trusted_now: false,
  };
}

it("submits one explicit verification with exact captured guards", async () => {
  let accepted = false;
  fetchSpy.mockImplementation((request, init) => {
    if (init?.method === "POST") {
      accepted = true;
      return Promise.resolve(response(failedJob()));
    }
    if (String(request).endsWith(verificationsSuffix))
      return Promise.resolve(response({ verifications: accepted ? [failedJob()] : [] }));
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  let outcomes: boolean[] = [];
  await act(async () => {
    outcomes = await Promise.all([result.current.verify(), result.current.verify()]);
  });
  expect(outcomes.filter(Boolean)).toHaveLength(1);
  const posts = fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST");
  expect(posts).toHaveLength(1);
  expect(String(posts[0]![0])).toBe(
    "http://api.test/api/v1/sandbox/sessions/session/attempts/attempt/audit/verifications",
  );
  const body = JSON.parse(posts[0]![1]!.body as string);
  expect(body).toMatchObject({
    expected_report_digest: report().digest,
    expected_artifact_digest: report().data.final_audit!.artifact_set_digest,
    expected_trust_version: 0,
  });
  expect(body.request_id).toMatch(
    /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/,
  );
  await waitFor(() => expect(result.current.history).toEqual([failedJob()]));
});

it("refuses an old action after operator authority changes", async () => {
  fetchSpy.mockImplementation((request, init) =>
    init?.method === "POST" ? Promise.resolve(response(failedJob())) : readAudit(request),
  );
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  const oldVerify = result.current.verify;
  act(() =>
    store
      .getState()
      .setAuthState({ mode: "enabled", authenticated: true, user: { ...user, role: "member" } }),
  );
  expect(result.current.canVerify).toBe(false);
  await act(async () => expect(await oldVerify()).toBe(false));
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(0);
});

it("reads admitted history after an uncertain response without resubmitting", async () => {
  let admitted = false;
  fetchSpy.mockImplementation((request, init) => {
    if (init?.method === "POST") {
      admitted = true;
      return Promise.reject(new TypeError("lost response"));
    }
    if (String(request).endsWith(verificationsSuffix))
      return Promise.resolve(response({ verifications: admitted ? [failedJob()] : [] }));
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  await act(async () => expect(await result.current.verify()).toBe(false));
  await waitFor(() => expect(result.current.history).toEqual([failedJob()]));
  expect(result.current.isBusy).toBe(false);
  expect(result.current.failed).toBe(true);
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
});

it("cannot verify a live report or a final report with missing artifacts", async () => {
  fetchSpy.mockImplementation(readAudit);
  const captured = report();
  captured.data = { ...captured.data, finalized: false, final_audit: undefined };
  const { result } = renderHook(() => useSandboxAudit("session", captured), { wrapper });
  connect();
  await waitFor(() => expect(result.current.isPending).toBe(false));
  expect(result.current.canVerify).toBe(false);
  await act(async () => expect(await result.current.verify()).toBe(false));
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(0);
});

it("reads a pending job until it reaches its stored terminal result", async () => {
  let admitted = false;
  let jobReads = 0;
  const finished = failedJob();
  const pending = {
    ...finished,
    verification: {
      ...finished.verification,
      state: "pending" as const,
      version: 1,
      completed_at: undefined,
      failure_code: undefined,
    },
  };
  fetchSpy.mockImplementation((request, init) => {
    const url = String(request);
    if (init?.method === "POST") {
      admitted = true;
      return Promise.resolve(response(pending));
    }
    if (url.endsWith(verificationsSuffix + "/" + finished.verification.id)) {
      jobReads++;
      return Promise.resolve(response(jobReads === 1 ? pending : finished));
    }
    if (url.endsWith(verificationsSuffix))
      return Promise.resolve(response({ verifications: admitted ? [pending] : [] }));
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  await act(async () => expect(await result.current.verify()).toBe(true));
  await waitFor(() => expect(result.current.history).toEqual([finished]), { timeout: 2500 });
  expect(jobReads).toBe(2);
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
});

it("publishes one operator trust revision with the displayed expected version", async () => {
  let current = trust;
  const revoked = {
    ...trust,
    id: trustRevisionID,
    version: 1,
    source: "operator",
    operator_id: "operator",
    digest: "f".repeat(64),
  };
  fetchSpy.mockImplementation((request, init) => {
    if (init?.method === "PUT") {
      current = revoked;
      return Promise.resolve(response(revoked));
    }
    if (String(request).endsWith(trustSuffix)) return Promise.resolve(response(current));
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canVerify).toBe(true));
  let outcomes: boolean[] = [];
  await act(async () => {
    outcomes = await Promise.all([
      result.current.saveTrust({ expected_version: 0, revoke: true }),
      result.current.saveTrust({ expected_version: 0, revoke: true }),
    ]);
  });
  expect(outcomes.filter(Boolean)).toHaveLength(1);
  const writes = fetchSpy.mock.calls.filter(([, init]) => init?.method === "PUT");
  expect(writes).toHaveLength(1);
  expect(JSON.parse(writes[0]![1]!.body as string)).toEqual({ expected_version: 0, revoke: true });
  await waitFor(() => expect(result.current.trust?.version).toBe(1));
  expect(result.current.isBusy).toBe(false);
  await act(async () =>
    expect(await result.current.saveTrust({ expected_version: 0, revoke: true })).toBe(false),
  );
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "PUT")).toHaveLength(1);
});

it("does not expose a trust response that contains private key material", async () => {
  const invalid = {
    ...trust,
    id: trustRevisionID,
    version: 1,
    revoked: false,
    source: "operator",
    operator_id: "operator",
    digest: "f".repeat(64),
    fingerprint: "a".repeat(64),
    public_key_pem: "-----BEGIN PRIVATE KEY-----\nfixture-only\n-----END PRIVATE KEY-----",
  };
  fetchSpy.mockImplementation((request) =>
    String(request).endsWith(trustSuffix) ? Promise.resolve(response(invalid)) : readAudit(request),
  );
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.isPending).toBe(false));
  expect(result.current.trust).toBeNull();
  expect(result.current.canManageTrust).toBe(false);
  expect(result.current.failed).toBe(true);
});

it("loads bounded history pages once without duplicate records", async () => {
  const rows = Array.from({ length: 22 }, (_, index) => {
    const row = failedJob();
    row.verification.id = `58273fe0-cdae-4849-b359-${(40 - index).toString(16).padStart(12, "0")}`;
    return row;
  });
  const cursor = rows[19]!.verification.id;
  fetchSpy.mockImplementation((request) => {
    const url = new URL(String(request));
    if (url.pathname.endsWith(verificationsSuffix))
      return Promise.resolve(
        response(
          url.searchParams.get("cursor") === cursor
            ? { verifications: rows.slice(20) }
            : { verifications: rows.slice(0, 20), next_cursor: cursor },
        ),
      );
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canLoadMore).toBe(true));
  await act(async () => {
    await Promise.all([result.current.loadMore(), result.current.loadMore()]);
  });
  await waitFor(() => expect(result.current.history).toEqual(rows));
  expect(result.current.canLoadMore).toBe(false);
  expect(
    fetchSpy.mock.calls.filter(
      ([request]) => new URL(String(request)).searchParams.get("cursor") === cursor,
    ),
  ).toHaveLength(1);
});

it("stores the selected public key without converting it into a verification request", async () => {
  const { publicKey } = generateKeyPairSync("ec", { namedCurve: "prime256v1" });
  const pem = String(publicKey.export({ type: "spki", format: "pem" }));
  const fingerprint = createHash("sha256")
    .update(publicKey.export({ type: "spki", format: "der" }))
    .digest("hex");
  const saved = {
    ...trust,
    id: trustRevisionID,
    version: 1,
    source: "operator",
    operator_id: "operator",
    digest: "f".repeat(64),
    revoked: false,
    public_key_pem: pem,
    fingerprint,
  };
  let current = trust;
  fetchSpy.mockImplementation((request, init) => {
    if (init?.method === "PUT") {
      current = saved;
      return Promise.resolve(response(saved));
    }
    if (String(request).endsWith(trustSuffix)) return Promise.resolve(response(current));
    return readAudit(request);
  });
  const { result } = renderHook(() => useSandboxAudit("session", report()), { wrapper });
  connect();
  await waitFor(() => expect(result.current.canManageTrust).toBe(true));
  await act(async () =>
    expect(await result.current.saveTrust({ expected_version: 0, public_key_pem: pem })).toBe(true),
  );
  await waitFor(() => expect(result.current.trust?.fingerprint).toBe(fingerprint));
  expect(result.current.trust?.public_key_pem).toBe(pem);
  expect(fetchSpy.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(0);
});
