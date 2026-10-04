import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { useProjectFormData } from "./agent-project-form-fields";
import type { AgentProject } from "@/lib/types/http-agent-projects";

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  generateUUID: vi.fn(),
}));

vi.mock("@/hooks/domains/agent-projects/use-agent-projects", () => ({
  useAgentProjectMutations: () => ({ create: mocks.create, update: mocks.update }),
}));
vi.mock("@/lib/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/utils")>()),
  generateUUID: mocks.generateUUID,
}));

import { useAgentProjectFormController } from "./agent-project-form-controller";

const formData = {
  repositories: [{ id: "repo-1" }],
  profiles: [{ id: "coordinator" }, { id: "economy" }, { id: "frontier" }],
  executorReady: true,
  repositoriesLoaded: true,
} as ReturnType<typeof useProjectFormData>;

describe("useAgentProjectFormController", () => {
  beforeEach(() => {
    mocks.create.mockReset();
    mocks.update.mockReset();
    mocks.generateUUID.mockReset();
    mocks.generateUUID.mockReturnValueOnce("request-key-1").mockReturnValue("request-key-2");
  });

  it("reuses the same create request key when retrying a draft after an error", async () => {
    mocks.create.mockRejectedValueOnce(new Error("response lost")).mockResolvedValue({ id: "p1" });
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useAgentProjectFormController(true, "ws-1", undefined, formData, onOpenChange),
    );
    act(() => {
      result.current.updateDraft({
        name: "Release migration",
        repositoryIds: ["repo-1"],
        primaryRepositoryId: "repo-1",
        coordinatorProfileId: "coordinator",
        economyProfileId: "economy",
        frontierProfileId: "frontier",
      });
    });

    const event = { preventDefault: vi.fn() } as never;
    await act(async () => result.current.submit(event));
    await act(async () => result.current.submit(event));

    expect(mocks.create).toHaveBeenCalledTimes(2);
    expect(mocks.create.mock.calls[0][1].requestKey).toBe("request-key-1");
    expect(mocks.create.mock.calls[1][1].requestKey).toBe("request-key-1");
    expect(mocks.generateUUID).toHaveBeenCalledTimes(1);
  });

  it("keeps the edited draft and exposes a stale-revision failure", async () => {
    mocks.update.mockRejectedValue(new Error("project was changed by another request"));
    const project = {
      id: "project-1",
      name: "Original name",
      revision: 1,
      repository_ids: ["repo-1"],
      primary_repository_id: "repo-1",
      coordinator_profile_id: "coordinator",
      economy_profile_id: "economy",
      frontier_profile_id: "frontier",
    } as AgentProject;
    const onOpenChange = vi.fn();
    const { result } = renderHook(() =>
      useAgentProjectFormController(true, "ws-1", project, formData, onOpenChange),
    );
    act(() => result.current.updateDraft({ name: "Keep this edit" }));
    await act(async () => result.current.submit({ preventDefault: vi.fn() } as never));
    expect(result.current.draft.name).toBe("Keep this edit");
    expect(result.current.error).toBe("project was changed by another request");
    expect(result.current.saving).toBe(false);
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
