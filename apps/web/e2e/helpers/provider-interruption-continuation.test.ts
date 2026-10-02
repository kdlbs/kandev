import { describe, expect, it, vi } from "vitest";
import { createContinuationFixture } from "./provider-interruption-continuation";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

describe("continuation fixture cleanup", () => {
  it.each(["listAgents", "createAgentProfile"])(
    "restores baseline after %s fails",
    async (stage) => {
      const failure = new Error("setup failed");
      const backend = { tmpDir: "/tmp", restart: vi.fn().mockResolvedValue(undefined) };
      const api = {
        listAgents: vi.fn().mockResolvedValue({ agents: [{ id: "mock", name: "mock-agent" }] }),
        createAgentProfile: vi.fn(),
      };
      api[stage as "listAgents" | "createAgentProfile"].mockRejectedValue(failure);
      await expect(
        createContinuationFixture(
          backend as unknown as BackendContext,
          api as unknown as ApiClient,
          {} as SeedData,
          "read",
        ),
      ).rejects.toBe(failure);
      expect(backend.restart).toHaveBeenLastCalledWith();
      expect(backend.restart).toHaveBeenCalledTimes(2);
    },
  );

  it("preserves the setup error when cleanup also fails", async () => {
    const failure = new Error("task creation failed");
    const backend = { tmpDir: "/tmp", restart: vi.fn().mockResolvedValue(undefined) };
    const api = {
      listAgents: vi.fn().mockResolvedValue({ agents: [{ id: "mock", name: "mock-agent" }] }),
      createAgentProfile: vi.fn().mockResolvedValue({ id: "profile" }),
      createTaskWithAgent: vi.fn().mockRejectedValue(failure),
      deleteAgentProfile: vi.fn().mockRejectedValue(new Error("cleanup failed")),
    };
    const log = vi.spyOn(console, "warn").mockImplementation(() => {});
    try {
      await expect(
        createContinuationFixture(
          backend as unknown as BackendContext,
          api as unknown as ApiClient,
          {} as SeedData,
          "read",
        ),
      ).rejects.toBe(failure);
      expect(backend.restart).toHaveBeenLastCalledWith();
    } finally {
      log.mockRestore();
    }
  });
});
