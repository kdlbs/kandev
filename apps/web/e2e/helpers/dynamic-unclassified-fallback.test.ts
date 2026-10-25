import { describe, expect, it, vi } from "vitest";
import type { ApiClient } from "./api-client";
vi.mock("./session", () => ({ openTaskSession: vi.fn() }));
import { cleanupDynamicFallbackProfiles } from "./dynamic-unclassified-fallback";

describe("cleanupDynamicFallbackProfiles", () => {
  it("deletes dynamic profiles before their candidate profiles", async () => {
    const deleteAgentProfile = vi.fn().mockResolvedValue(undefined);
    const api = { deleteAgentProfile } as unknown as ApiClient;
    await cleanupDynamicFallbackProfiles(api, ["dynamic-a", "dynamic-b"], ["candidate"]);
    expect(deleteAgentProfile.mock.calls).toEqual([
      ["dynamic-a", true],
      ["dynamic-b", true],
      ["candidate", true],
    ]);
  });

  it("attempts every cleanup and reports all failures", async () => {
    const first = new Error("first deletion failed");
    const last = new Error("candidate deletion failed");
    const deleteAgentProfile = vi
      .fn()
      .mockRejectedValueOnce(first)
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(last);
    const api = { deleteAgentProfile } as unknown as ApiClient;
    await expect(
      cleanupDynamicFallbackProfiles(api, ["dynamic-a", "dynamic-b"], ["candidate"]),
    ).rejects.toMatchObject({ errors: [first, last] });
    expect(deleteAgentProfile).toHaveBeenCalledTimes(3);
  });
});
