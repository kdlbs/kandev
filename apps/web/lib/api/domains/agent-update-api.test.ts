import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  listAgentUpdateStatuses,
  previewAgentUpdate,
  previewAgentUpdateUseDefault,
  updateAgent,
  updateAgentUseDefault,
} from "./agent-update-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";
const AGENT_NAME = "opencode-acp";
const TARGET_VERSION = "1.18.5";

beforeEach(() => {
  fetchSpy.mockReset();
  fetchSpy.mockResolvedValue(
    new Response(
      JSON.stringify({
        agent_name: AGENT_NAME,
        package: "opencode-ai",
        target_version: TARGET_VERSION,
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
    await previewAgentUpdate(AGENT_NAME, TARGET_VERSION, { cache: "no-store" });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(
      `${API_BASE_URL}/api/v1/agent-update/${AGENT_NAME}/preview?target_version=${TARGET_VERSION}`,
    );
    expect(init?.cache).toBe("no-store");
  });

  it("sends only the exact target version in the update body", async () => {
    await updateAgent(AGENT_NAME, TARGET_VERSION);

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ target_version: TARGET_VERSION }));
  });

  it("previews returning to the Kandev default with a structural query", async () => {
    await previewAgentUpdateUseDefault(AGENT_NAME, { cache: "no-store" });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(
      `${API_BASE_URL}/api/v1/agent-update/${AGENT_NAME}/preview?use_default=true`,
    );
    expect(init?.cache).toBe("no-store");
  });

  it("sends a structural use_default update request", async () => {
    await updateAgentUseDefault(AGENT_NAME);

    const [, init] = fetchSpy.mock.calls[0] ?? [];
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(JSON.stringify({ use_default: true }));
  });

  it("loads the read-only batch status endpoint without mutation", async () => {
    await listAgentUpdateStatuses({ cache: "no-store" });

    const [input, init] = fetchSpy.mock.calls[0] ?? [];
    expect(String(input)).toBe(`${API_BASE_URL}/api/v1/agent-update/status`);
    expect(init?.cache).toBe("no-store");
  });
});
