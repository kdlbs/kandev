import { describe, expect, it, vi } from "vitest";
import {
  createContinuationFixture,
  hasAttestedContinuationRefusal,
} from "./provider-interruption-continuation";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

describe("continuation fixture cleanup", () => {
  it("requires the persisted refusal reason and actual attempt count", () => {
    expect(
      hasAttestedContinuationRefusal(
        {
          metadata: {
            runtime_retained: true,
            recovery_disposition: "refused",
            recovery_reason: "unsafe_work",
            attempts_started: 0,
          },
        } as Parameters<typeof hasAttestedContinuationRefusal>[0],
        "unsafe_work",
      ),
    ).toBe(true);
    expect(
      hasAttestedContinuationRefusal(
        {
          metadata: {
            runtime_retained: true,
            recovery_disposition: "refused",
            recovery_reason: "unsafe_work",
            attempts_started: 1,
          },
        } as Parameters<typeof hasAttestedContinuationRefusal>[0],
        "unsafe_work",
      ),
    ).toBe(false);
  });

  it("prepares a stable session without submitting the interruption before browser readiness", async () => {
    const backend = { tmpDir: "/tmp", restart: vi.fn().mockResolvedValue(undefined) };
    const api = {
      listAgents: vi.fn().mockResolvedValue({ agents: [{ id: "mock", name: "mock-agent" }] }),
      createAgentProfile: vi.fn().mockResolvedValue({ id: "profile" }),
      createTaskWithAgent: vi.fn().mockResolvedValue({ id: "task" }),
      launchSession: vi.fn().mockResolvedValue({ session_id: "prepared-session" }),
      deleteTask: vi.fn().mockResolvedValue(undefined),
      deleteAgentProfile: vi.fn().mockResolvedValue(undefined),
    };
    const fixture = await createContinuationFixture(
      backend as unknown as BackendContext,
      api as unknown as ApiClient,
      { workspaceId: "workspace" } as SeedData,
      "read-hold",
      { prepareOnly: true, executorProfileId: "executor" },
    );
    expect(api.createTaskWithAgent.mock.calls[0]?.[3]).toMatchObject({
      description: "",
      start_agent: false,
    });
    expect(api.launchSession).toHaveBeenCalledWith({
      task_id: "task",
      agent_profile_id: "profile",
      executor_profile_id: "executor",
      intent: "prepare",
      launch_workspace: true,
    });
    expect(api.launchSession.mock.calls[0]?.[0]).not.toHaveProperty("prompt");
    expect(fixture.sessionId).toBe("prepared-session");
    await fixture.dispose();
    expect(api.deleteTask).toHaveBeenCalledWith("task");
    expect(backend.restart).toHaveBeenLastCalledWith();
  });

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
      expect(backend.restart.mock.calls[0][0]).not.toHaveProperty(
        "KANDEV_FEATURES_PROVIDER_INTERRUPTION_CONTINUATION",
      );
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
