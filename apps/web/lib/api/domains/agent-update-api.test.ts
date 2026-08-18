import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { previewAgentUpdate, updateAgent } from "./agent-update-api";

const fetchSpy = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockResolvedValue(
    new Response(
      JSON.stringify({
        agent_name: "opencode-acp",
        package: "opencode-ai",
        target_version: "1.18.5",
        operation: "rollback",
        available_versions: [],
        command: [],
        command_string: "",
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ),
  );
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("managed runtime update API", () => {
  it("encodes an optional exact target in the preview query", async () => {
    await previewAgentUpdate("opencode-acp", "1.18.5", { cache: "no-store" });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(
      "http://api.test/api/v1/agent-update/opencode-acp/preview?target_version=1.18.5",
    );
    expect(init?.cache).toBe("no-store");
  });

  it("sends only the exact target version in the update body", async () => {
    await updateAgent("opencode-acp", "1.18.5");

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ target_version: "1.18.5" }));
  });
});
