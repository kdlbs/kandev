import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { testRemoteDockerConnection } from "./remote-docker-api";

describe("testRemoteDockerConnection", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    global.fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ success: true, fingerprint: "SHA256:abc", steps: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ) as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("posts the SSH target to the remote docker endpoint", async () => {
    const result = await testRemoteDockerConnection({ name: "build-box", host: "build-box" });

    expect(result.success).toBe(true);
    expect(result.fingerprint).toBe("SHA256:abc");

    const [url, init] = vi.mocked(global.fetch).mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/remote-docker/test");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toMatchObject({ host: "build-box" });
  });

  it("keeps the JSON content type when a caller passes headers", async () => {
    await testRemoteDockerConnection(
      { name: "build-box", host: "build-box" },
      { init: { headers: { "X-Test": "1" } } },
    );

    const [, init] = vi.mocked(global.fetch).mock.calls[0] as [string, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Test")).toBe("1");
  });
});
