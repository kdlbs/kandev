import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchJson } from "./client";

const interlockToken = "replayable-per-boot-value";

describe("fetchJson", () => {
  afterEach(() => {
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

    expect({
      isHeaders: headers instanceof Headers,
      contentTypes: [...new Headers(headers).entries()].filter(
        ([name]) => name.toLowerCase() === "content-type",
      ),
      callerHeader: new Headers(headers).get("X-Caller-Header"),
      interlock: new Headers(headers).get("X-Kandev-Interim-Settings-Interlock"),
    }).toEqual({
      isHeaders: true,
      contentTypes: [["Content-Type", "application/json"]],
      callerHeader: "preserved",
      interlock: interlockToken,
    });
  });
});
