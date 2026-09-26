import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { buildRemoteDockerImage, testRemoteDockerConnection } from "./remote-docker-api";

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

describe("buildRemoteDockerImage base URL", () => {
  // A relative URL posts to the web origin. That is invisible when the backend
  // serves the SPA, and a 404 the moment the two are separate origins -- the
  // Vite dev server, the desktop shell, or a reverse proxy on another port.
  it("resolves against the configured backend origin", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("window", {
      location: { protocol: "http:", hostname: "10.0.0.5", origin: "http://10.0.0.5:5173" },
      __KANDEV_API_PORT: "38429",
    });

    await buildRemoteDockerImage("executor-1", { dockerfile: "FROM alpine", tag: "demo:latest" });

    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toBe("http://10.0.0.5:38429/api/v1/remote-docker/executors/executor-1/build");

    vi.unstubAllGlobals();
  });
});
