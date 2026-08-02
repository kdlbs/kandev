import { afterEach, describe, expect, it, vi } from "vitest";

import { leaseDiagnosticBundle } from "./improve-kandev-api";

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
