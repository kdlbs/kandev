import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, fetchJson, setOnUnauthorized } from "./client";

const interlockToken = "replayable-per-boot-value";

describe("fetchJson", () => {
  afterEach(() => {
    setOnUnauthorized(null);
    vi.unstubAllGlobals();
    delete (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__;
  });

  it("attaches the replayable interim settings interlock to mutations", async () => {
    (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__ = {
      interimSettingsInterlockToken: interlockToken,
    };
    const fetcher = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetcher);

    await fetchJson("/api/v1/agents", {
      baseUrl: "http://backend.test",
      init: { method: "POST", body: "{}" },
    });

    expect(fetcher).toHaveBeenCalledWith("http://backend.test/api/v1/agents", expect.any(Object));
    expect(
      new Headers(fetcher.mock.calls[0][1]?.headers).get("X-Kandev-Interim-Settings-Interlock"),
    ).toBe(interlockToken);
  });

  it("replaces a lowercase content type without losing mutation headers", async () => {
    (window as unknown as { __KANDEV_BOOT_PAYLOAD__?: unknown }).__KANDEV_BOOT_PAYLOAD__ = {
      interimSettingsInterlockToken: interlockToken,
    };
    const fetcher = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetcher);

    await fetchJson("/api/v1/agents", {
      baseUrl: "http://backend.test",
      init: {
        method: "POST",
        body: "{}",
        headers: { "content-type": "text/plain", "X-Caller-Header": "preserved" },
      },
    });

    const headers = fetcher.mock.calls[0][1]?.headers;
    const normalizedHeaders = new Headers(headers);

    expect({
      isHeaders: headers instanceof Headers,
      contentType: normalizedHeaders.get("content-type"),
      contentTypeCount: [...normalizedHeaders.entries()].filter(
        ([name]) => name.toLowerCase() === "content-type",
      ).length,
      callerHeader: normalizedHeaders.get("X-Caller-Header"),
      interlock: normalizedHeaders.get("X-Kandev-Interim-Settings-Interlock"),
    }).toEqual({
      isHeaders: true,
      contentType: "application/json",
      contentTypeCount: 1,
      callerHeader: "preserved",
      interlock: interlockToken,
    });
  });

  it("notifies the app for a Kandev session challenge", async () => {
    const unauthorized = vi.fn();
    setOnUnauthorized(unauthorized);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "authentication required" }), {
          status: 401,
          headers: {
            "Content-Type": "application/json",
            "WWW-Authenticate": "Bearer",
          },
        }),
      ),
    );

    await expect(
      fetchJson("/api/v1/workspaces", { baseUrl: "http://kandev.test" }),
    ).rejects.toBeInstanceOf(ApiError);
    expect(unauthorized).toHaveBeenCalledOnce();
  });

  it("keeps an unchallenged provider 401 in the calling integration", async () => {
    const unauthorized = vi.fn();
    setOnUnauthorized(unauthorized);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "GitHub credentials are invalid" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    const request = fetchJson("/api/v1/github/user/prs", { baseUrl: "http://kandev.test" });
    await expect(request).rejects.toMatchObject({
      name: "ApiError",
      status: 401,
      message: "GitHub credentials are invalid",
    });
    expect(unauthorized).not.toHaveBeenCalled();
  });
});
