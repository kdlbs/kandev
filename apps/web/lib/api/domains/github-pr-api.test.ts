import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { getRepoMergeAccess, mergePR } from "./github-api";

afterEach(() => {
  vi.unstubAllGlobals();
});

it("reads target merge access without caching and supports cancellation", async () => {
  const fetchSpy = vi.fn(
    async () =>
      new Response(JSON.stringify({ state: "unknown", actor: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchSpy);
  const controller = new AbortController();
  await getRepoMergeAccess("ws/1", "acme org", "site/docs", {
    init: { signal: controller.signal },
  });
  expect(fetchSpy).toHaveBeenCalledWith(
    "http://api.test/api/v1/github/repos/acme%20org/site%2Fdocs/merge-access?workspace_id=ws%2F1",
    expect.objectContaining({ cache: "no-store", signal: controller.signal }),
  );
});

it("returns the queue-aware merge outcome", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ status: "queued" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );

  await expect(mergePR("workspace-1", "acme", "site", 42, "squash")).resolves.toEqual({
    status: "queued",
  });
});
