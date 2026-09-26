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
