import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  configureGitLabHost,
  configureGitLabToken,
  clearGitLabToken,
  fetchGitLabStatus,
} from "./gitlab-api";

const originalFetch = global.fetch;
const SELF_MANAGED_HOST = "https://gitlab.acme.corp";

function mockResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("gitlab-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("fetchGitLabStatus calls /api/v1/gitlab/status", async () => {
    fetchSpy.mockResolvedValueOnce(
      mockResponse({
        authenticated: true,
        username: "alice",
        auth_method: "pat",
        host: "https://gitlab.com",
        token_configured: true,
        required_scopes: ["api"],
      }),
    );
    const status = await fetchGitLabStatus();
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const url = fetchSpy.mock.calls[0]![0] as string;
    expect(url).toContain("/api/v1/gitlab/status");
    expect(status.username).toBe("alice");
    expect(status.auth_method).toBe("pat");
  });

  it("configureGitLabToken POSTs the token", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true }));
    const result = await configureGitLabToken("glpat-123");
    expect(result.configured).toBe(true);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ token: "glpat-123" });
  });

  it("clearGitLabToken issues DELETE", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ cleared: true }));
    await clearGitLabToken();
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(init.method).toBe("DELETE");
  });

  it("configureGitLabHost POSTs the host", async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse({ configured: true, host: SELF_MANAGED_HOST }));
    const result = await configureGitLabHost(SELF_MANAGED_HOST);
    expect(result.host).toBe(SELF_MANAGED_HOST);
    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(JSON.parse(init.body as string)).toEqual({ host: SELF_MANAGED_HOST });
  });
});
