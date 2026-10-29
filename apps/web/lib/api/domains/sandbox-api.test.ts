import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));

import {
  checkSandboxAvailability,
  fetchSandboxPolicies,
  previewSandboxSelection,
  updateSandboxBinding,
} from "./sandbox-api";

const fetchSpy = vi.fn<typeof fetch>();
beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});
afterEach(() => vi.unstubAllGlobals());

function response(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("sandbox policy API", () => {
  it("checks availability only through an explicit cancellable context request", async () => {
    fetchSpy.mockResolvedValueOnce(response({ available: true }));
    const controller = new AbortController();
    const context = { executor_id: "local", agent_profile_id: "agent" };
    const result = await checkSandboxAvailability(context, { init: { signal: controller.signal } });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(String(url)).toBe("http://api.test/api/v1/sandbox/availability");
    expect(init?.signal).toBe(controller.signal);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual(context);
    expect(result).toEqual({ available: true });
  });
  it("reads every catalog page in the requested workspace without caching", async () => {
    fetchSpy.mockResolvedValueOnce(
      response({ policies: [{ id: "revision-one" }], next_cursor: "policy one" }),
    );
    fetchSpy.mockResolvedValueOnce(response({ policies: [{ id: "revision-two" }] }));
    const policies = await fetchSandboxPolicies("workspace/a");
    expect(policies.map((policy) => policy.id)).toEqual(["revision-one", "revision-two"]);
    const second = new URL(String(fetchSpy.mock.calls[1][0]));
    expect(second.searchParams.get("workspace_id")).toBe("workspace/a");
    expect(second.searchParams.get("after")).toBe("policy one");
    expect(fetchSpy.mock.calls.every(([, init]) => init?.cache === "no-store")).toBe(true);
  });

  it("rejects a repeated pagination cursor instead of spinning", async () => {
    fetchSpy.mockResolvedValue(response({ policies: [], next_cursor: "same" }));
    await expect(fetchSandboxPolicies("")).rejects.toThrow();
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });

  it("preserves conflict responses and sends the exact binding version", async () => {
    fetchSpy.mockResolvedValueOnce(response({ error: "sandbox_revision_conflict" }, 409));
    await expect(
      updateSandboxBinding({
        scope: "agent",
        target_id: "agent/a",
        workspace_id: "",
        revision_id: "revision",
        version: 7,
      }),
    ).rejects.toThrow();
    const [url, init] = fetchSpy.mock.calls[0];
    expect(String(url)).toBe("http://api.test/api/v1/sandbox/bindings/agent/agent%2Fa");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      binding: {
        scope: "agent",
        target_id: "agent/a",
        workspace_id: "",
        revision_id: "revision",
        version: 7,
      },
      expected_version: 7,
    });
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("sends only prospective context without organization or launch authority", async () => {
    fetchSpy.mockResolvedValueOnce(response({ resolved: null, eligible: [] }));
    await previewSandboxSelection({
      executor_id: "local",
      agent_profile_id: "agent",
      workspace_id: "workspace",
    });
    const [, init] = fetchSpy.mock.calls[0];
    expect(JSON.parse(String(init?.body))).toEqual({
      executor_id: "local",
      agent_profile_id: "agent",
      workspace_id: "workspace",
    });
    expect(init?.method).toBe("POST");
  });
});
