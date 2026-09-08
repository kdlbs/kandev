import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  getKubernetesTaskSession,
  getKubernetesSessionImpact,
  listKubernetesSessions,
  normalizeKubernetesSessions,
  normalizeKubernetesTestResult,
  testKubernetesConnection,
} from "./kubernetes-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("Kubernetes settings API", () => {
  it("normalizes absent diagnostic collections", () => {
    expect(
      normalizeKubernetesTestResult({
        success: true,
        server_version: "v1.30.1",
        namespace: "agents",
        steps: null,
        warnings: null,
      }),
    ).toEqual({
      success: true,
      server_version: "v1.30.1",
      namespace: "agents",
      steps: [],
      warnings: [],
    });
  });

  it("keeps causal steps and warnings from the final Task 03 response", () => {
    expect(
      normalizeKubernetesTestResult({
        success: false,
        steps: [
          {
            key: "rbac",
            success: false,
            duration_ms: 12,
            detail: "Denied permissions: create pods",
            error: "required Kubernetes permissions are denied",
          },
        ],
        warnings: [
          {
            path: "config.pod_template_yaml.template.spec.containers[0]",
            message: "privileged container is enabled",
          },
        ],
        error: "required Kubernetes permissions are denied",
      }),
    ).toMatchObject({
      success: false,
      steps: [{ key: "rbac", success: false, duration_ms: 12 }],
      warnings: [{ path: "config.pod_template_yaml.template.spec.containers[0]" }],
      error: "required Kubernetes permissions are denied",
    });
  });

  it("POSTs unsaved executor and profile config with JSON headers", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true, steps: [], warnings: [] }));
    const request = {
      config: { auth_mode: "in_cluster", namespace: "agents" },
      profile_config: { platform: "linux/amd64" },
    };

    await testKubernetesConnection(request, {
      init: { method: "GET", headers: { "X-Trace": "trace-1" } },
    });

    expect(lastCall().url).toBe("http://api.test/api/v1/kubernetes/test");
    expect(lastCall().init?.method).toBe("POST");
    expect(new Headers(lastCall().init?.headers).get("Content-Type")).toBe("application/json");
    expect(new Headers(lastCall().init?.headers).get("X-Trace")).toBe("trace-1");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual(request);
  });

  it.each([
    new Headers({ "X-Trace": "headers-object" }),
    [["X-Trace", "tuple-list"]] as [string, string][],
  ])("preserves every HeadersInit representation", async (headers) => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ success: true, steps: [], warnings: [] }));

    await testKubernetesConnection({ config: { auth_mode: "in_cluster" } }, { init: { headers } });

    expect(new Headers(lastCall().init?.headers).get("X-Trace")).toBe(
      headers instanceof Headers ? "headers-object" : "tuple-list",
    );
    expect(new Headers(lastCall().init?.headers).get("Content-Type")).toBe("application/json");
  });
});

describe("Kubernetes session API", () => {
  it("normalizes active sessions and rejects incomplete inventory rows", () => {
    expect(
      normalizeKubernetesSessions([
        { session_id: "session-1", task_id: "task-1", pod_name: "pod-1", restarts: 2 },
        { session_id: "session-2", restarts: 0 },
        null,
      ]),
    ).toEqual([{ session_id: "session-1", task_id: "task-1", pod_name: "pod-1", restarts: 2 }]);
  });

  it("GETs encoded executor session status and normalizes rows", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse([{ session_id: "session-1", task_id: "task-1", restarts: null }]),
    );

    const rows = await listKubernetesSessions("executor one/primary");

    expect(lastCall().url).toBe(
      "http://api.test/api/v1/kubernetes/executors/executor%20one%2Fprimary/sessions",
    );
    expect((lastCall().init?.method ?? "GET").toUpperCase()).toBe("GET");
    expect(rows).toEqual([{ session_id: "session-1", task_id: "task-1", restarts: 0 }]);
  });

  it("GETs one exact task session through encoded filters", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse([
        { session_id: "session one/active", task_id: "task one/primary", restarts: 0 },
      ]),
    );

    await expect(
      getKubernetesTaskSession("executor one/primary", "task one/primary", "session one/active"),
    ).resolves.toMatchObject({
      session_id: "session one/active",
      task_id: "task one/primary",
    });
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/kubernetes/executors/executor%20one%2Fprimary/sessions" +
        "?task_id=task+one%2Fprimary&session_id=session+one%2Factive",
    );
  });

  it("returns null when the filtered response is missing or malformed", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse([]))
      .mockResolvedValueOnce(jsonResponse([{ task_id: "task-1" }]))
      .mockResolvedValueOnce(
        jsonResponse([{ task_id: "task-other", session_id: "session-other" }]),
      );

    await expect(getKubernetesTaskSession("executor-1", "task-1", "session-1")).resolves.toBeNull();
    await expect(getKubernetesTaskSession("executor-1", "task-1", "session-1")).resolves.toBeNull();
    await expect(getKubernetesTaskSession("executor-1", "task-1", "session-1")).resolves.toBeNull();
  });

  it("GETs the authoritative encoded executor session impact", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ active_session_count: 3 }));

    await expect(getKubernetesSessionImpact("executor one/primary")).resolves.toEqual({
      active_session_count: 3,
    });
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/kubernetes/executors/executor%20one%2Fprimary/session-impact",
    );
    expect((lastCall().init?.method ?? "GET").toUpperCase()).toBe("GET");
  });
});
