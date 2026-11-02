import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { verifyAutomaticRestartReconciliation } from "./restart-reconciliation";

vi.mock("@playwright/test", () => ({ expect: vi.fn(), test: { info: vi.fn() } }));

describe("restart reconciliation cleanup", () => {
  it.each(["task", "profile"])("restores settings when %s deletion fails", async (stage) => {
    const cleanupFailure = new Error("resource deletion failed");
    const release = vi.fn().mockResolvedValue(undefined);
    const backend = { useEnv: vi.fn().mockResolvedValue(release) };
    const page = { goto: vi.fn().mockRejectedValue(new Error("navigation failed")) };
    const api = {
      listAgents: vi.fn().mockResolvedValue({ agents: [{ id: "mock", name: "mock-agent" }] }),
      createAgentProfile: vi.fn().mockResolvedValue({ id: "profile" }),
      createTaskWithAgent: vi.fn().mockResolvedValue({ id: "task" }),
      deleteTask: vi.fn().mockResolvedValue(undefined),
      deleteAgentProfile: vi.fn().mockResolvedValue(undefined),
    };
    if (stage === "task") api.deleteTask.mockRejectedValue(cleanupFailure);
    else api.deleteAgentProfile.mockRejectedValue(cleanupFailure);

    await expect(
      verifyAutomaticRestartReconciliation(
        page as unknown as Page,
        api as unknown as ApiClient,
        {} as SeedData,
        backend as unknown as BackendContext,
        "40s",
      ),
    ).rejects.toBe(cleanupFailure);
    expect(release).toHaveBeenCalledTimes(1);
  });
});
