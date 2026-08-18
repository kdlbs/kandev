import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../client", () => ({
  fetchJson: vi.fn(),
}));

import { fetchJson } from "../client";
import { listAgentConfigBundles } from "./agent-config-api";

beforeEach(() => {
  vi.mocked(fetchJson).mockResolvedValue({ bundles: [] });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("agent configuration API", () => {
  it("does not reuse a stale availability response", async () => {
    await listAgentConfigBundles();

    expect(fetchJson).toHaveBeenCalledWith("/api/v1/agent-config-bundles", {
      cache: "no-store",
    });
  });
});
