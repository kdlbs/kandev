import { afterEach, describe, expect, it, vi } from "vitest";

import { bootstrapImproveKandev, leaseDiagnosticBundle } from "./improve-kandev-api";

describe("bootstrapImproveKandev", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("opts into background preparation when requested", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ job_id: "job-1" }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await bootstrapImproveKandev("workspace-1", {
      async: true,
      createWorkspace: true,
    });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/system/improve-kandev/bootstrap");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      workspace_id: "workspace-1",
      create_workspace: true,
      async: true,
    });
    expect(result).toEqual({ job_id: "job-1" });
  });

  it("reads an owner-scoped bootstrap job without caching its status", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ job_id: "job-1" }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const api = await import("./improve-kandev-api");

    expect(typeof api.getImproveKandevBootstrapJob).toBe("function");
    await api.getImproveKandevBootstrapJob("job/1");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/system/improve-kandev/bootstrap/job%2F1");
    expect(init.method).toBe("GET");
    expect(init.cache).toBe("no-store");
  });
});

describe("leaseDiagnosticBundle", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts the server bootstrap directory and owned bundle ID", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          path: "/tmp/kandev-improve-1/diagnostic-bundle.zip",
          status: "ready",
          sources: ["backend", "frontend"],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await leaseDiagnosticBundle("/tmp/kandev-improve-1", "bundle-1");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/system/improve-kandev/bundle/lease");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      bundle_dir: "/tmp/kandev-improve-1",
      bundle_id: "bundle-1",
    });
    expect(result.status).toBe("ready");
  });
});
