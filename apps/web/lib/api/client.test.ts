import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, fetchBlob, fetchJson, setOnUnauthorized } from "./client";

const interlockToken = "replayable-per-boot-value";
const BACKEND_URL = "http://backend.test";

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
      baseUrl: BACKEND_URL,
      init: { method: "POST", body: "{}" },
    });

    expect(fetcher).toHaveBeenCalledWith(`${BACKEND_URL}/api/v1/agents`, expect.any(Object));
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
      baseUrl: BACKEND_URL,
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

describe("ApiError response classification", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves machine-readable error codes from the backend", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ error: "invalid branch policy", error_code: "branch_policy_stale" }),
            { status: 400, headers: { "Content-Type": "application/json" } },
          ),
        ),
    );

    await expect(fetchJson("/api/v1/tasks", { baseUrl: BACKEND_URL })).rejects.toMatchObject({
      name: "ApiError",
      errorCode: "branch_policy_stale",
    });
  });
});

describe("fetchBlob", () => {
  afterEach(() => {
    setOnUnauthorized(null);
    vi.unstubAllGlobals();
  });

  it("returns the response body as a Blob on success", async () => {
    const body = new Blob(["zip-bytes"], { type: "application/zip" });
    const fetcher = vi.fn().mockResolvedValue(new Response(body, { status: 200 }));
    vi.stubGlobal("fetch", fetcher);

    const result = await fetchBlob("/api/v1/workspaces/ws-1/automations/export/zip", {
      baseUrl: BACKEND_URL,
    });

    expect(fetcher).toHaveBeenCalledWith(
      `${BACKEND_URL}/api/v1/workspaces/ws-1/automations/export/zip`,
      expect.any(Object),
    );
    expect(result).toBeInstanceOf(Blob);
    expect(await result.text()).toBe("zip-bytes");
  });

  it("throws ApiError and never returns a Blob on a non-2xx status", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "workspace not found" }), {
          status: 404,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(
      fetchBlob("/api/v1/workspaces/ws-missing/automations/export/zip", {
        baseUrl: BACKEND_URL,
      }),
    ).rejects.toMatchObject({ name: "ApiError", status: 404, message: "workspace not found" });
  });

  it("notifies the app for a Kandev session challenge, like fetchJson", async () => {
    const unauthorized = vi.fn();
    setOnUnauthorized(unauthorized);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "authentication required" }), {
          status: 401,
          headers: { "Content-Type": "application/json", "WWW-Authenticate": "Bearer" },
        }),
      ),
    );

    await expect(
      fetchBlob("/api/v1/workspaces/ws-1/automations/export/zip", {
        baseUrl: BACKEND_URL,
      }),
    ).rejects.toBeInstanceOf(ApiError);
    expect(unauthorized).toHaveBeenCalledOnce();
  });
});
