import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import { registerRemoteRepositoryAction } from "./workspaces";

describe("registerRemoteRepositoryAction", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ id: "repo-1", remote_url: "https://github.com/acme/api.git" }),
            {
              status: 201,
              headers: { "Content-Type": "application/json" },
            },
          ),
      ),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("posts the locator to the workspace remote repository endpoint", async () => {
    const repository = await registerRemoteRepositoryAction("ws-1", {
      remote_url: "https://github.com/acme/api",
      default_branch: "main",
    });

    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://backend.test/api/v1/workspaces/ws-1/repositories/remote");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      remote_url: "https://github.com/acme/api",
      default_branch: "main",
    });
    expect(repository.id).toBe("repo-1");
  });
});
