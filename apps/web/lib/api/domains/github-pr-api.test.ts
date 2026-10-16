import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { mergePR } from "./github-api";

afterEach(() => {
  vi.unstubAllGlobals();
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

it("binds a historical PR merge to its original host", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>(
    async () =>
      new Response(JSON.stringify({ status: "merged" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  await mergePR("workspace-1", "acme", "site", 42, {
    mergeMethod: "squash",
    expectedHost: "git.company.example",
  });
  expect(new URL(String(fetch.mock.calls[0]?.[0])).searchParams.get("expected_host")).toBe(
    "git.company.example",
  );
});
