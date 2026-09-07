import { describe, expect, it, vi } from "vitest";
import { resetTaskForm, type FormResetters } from "./task-create-dialog-form-reset";

function makeResetters(): FormResetters {
  return {
    setTaskName: vi.fn(),
    setHasTitle: vi.fn(),
    setHasDescription: vi.fn(),
    setHasPendingAttachmentUploads: vi.fn(),
    setRepositories: vi.fn(),
    setRepositoriesDirty: vi.fn(),
    setRemoteRepos: vi.fn(),
    setAgentProfileId: vi.fn(),
    setExecutorId: vi.fn(),
    setExecutorProfileId: vi.fn(),
    setSelectedWorkflowId: vi.fn(),
    setFetchedSteps: vi.fn(),
    setDiscoveredRepositories: vi.fn(),
    setDiscoverReposLoaded: vi.fn(),
    setUseRemote: vi.fn(),
    setNoRepository: vi.fn(),
    setWorkspacePath: vi.fn(),
    setPreferLocalExecutor: vi.fn(),
    setAutopilot: vi.fn(),
    setPriority: vi.fn(),
    setGitHubUrlError: vi.fn(),
    setFreshBranchEnabled: vi.fn(),
    setCurrentLocalBranch: vi.fn(),
    setBlockedBy: vi.fn(),
  };
}

describe("resetTaskForm canvas source preset", () => {
  it("applies the repository-free local preference without storing a profile id", () => {
    const resetters = makeResetters();

    resetTaskForm(resetters, "Create a canvas", "Build a canvas", null, {
      title: "Create a canvas",
      description: "Build a canvas",
      noRepository: true,
      preferLocalExecutor: true,
    });

    expect(resetters.setNoRepository).toHaveBeenCalledWith(true);
    expect(resetters.setWorkspacePath).toHaveBeenCalledWith("");
    expect(resetters.setPreferLocalExecutor).toHaveBeenCalledWith(true);
    expect(resetters.setExecutorId).toHaveBeenCalledWith("");
    expect(resetters.setExecutorProfileId).toHaveBeenCalledWith("");
  });

  it("clears the launch-only source preference on an ordinary dialog reset", () => {
    const resetters = makeResetters();

    resetTaskForm(resetters, "", "", null);

    expect(resetters.setNoRepository).toHaveBeenCalledWith(false);
    expect(resetters.setWorkspacePath).toHaveBeenCalledWith("");
    expect(resetters.setPreferLocalExecutor).toHaveBeenCalledWith(false);
  });
});
