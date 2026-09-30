import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
import { createStandardProfile } from "./git-helper";

describe("createStandardProfile", () => {
  it("creates a mock profile when disabled and virtual agents precede the mock agent", async () => {
    const createAgentProfile = vi.fn().mockResolvedValue({ id: "profile-id" });
    const apiClient = {
      listAgents: vi.fn().mockResolvedValue({
        agents: [
          { id: "disabled-id", name: "openai-compatible" },
          { id: "dynamic", name: "dynamic" },
          { id: "mock-id", name: "mock-agent" },
        ],
      }),
      createAgentProfile,
    } as unknown as ApiClient;

    await expect(createStandardProfile(apiClient, "file-viewer")).resolves.toEqual({
      id: "profile-id",
    });
    expect(createAgentProfile).toHaveBeenCalledWith("mock-id", "file-viewer", {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: false,
    });
  });

  it("reports a missing mock agent instead of creating a profile for another family", async () => {
    const createAgentProfile = vi.fn();
    const apiClient = {
      listAgents: vi.fn().mockResolvedValue({
        agents: [{ id: "dynamic", name: "dynamic" }],
      }),
      createAgentProfile,
    } as unknown as ApiClient;

    await expect(createStandardProfile(apiClient, "file-viewer")).rejects.toThrow(
      "Mock agent unavailable",
    );
    expect(createAgentProfile).not.toHaveBeenCalled();
  });
});
