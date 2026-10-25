import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useSubtaskParentDefaults } from "./use-subtask-parent-defaults";

describe("useSubtaskParentDefaults", () => {
  it("seeds the selected parent's defaults when its context arrives after opening", () => {
    const setRepositories = vi.fn();
    const setAgentProfileId = vi.fn();
    const { rerender } = renderHook(
      ({ parentRepositoryId, defaultProfileId }) =>
        useSubtaskParentDefaults({
          parentRepositoryId,
          defaultProfileId,
          baseBranch: "release",
          repositoriesDirty: false,
          agentProfileId: "",
          setRepositories,
          setAgentProfileId,
        }),
      { initialProps: { parentRepositoryId: null as string | null, defaultProfileId: "" } },
    );
    expect(setRepositories).not.toHaveBeenCalled();
    expect(setAgentProfileId).not.toHaveBeenCalled();
    rerender({ parentRepositoryId: "selected-parent-repo", defaultProfileId: "parent-profile" });
    expect(setRepositories).toHaveBeenCalledExactlyOnceWith([
      { key: "subtask-row-1", repositoryId: "selected-parent-repo", branch: "release" },
    ]);
    expect(setAgentProfileId).toHaveBeenCalledExactlyOnceWith("parent-profile");
    rerender({ parentRepositoryId: "refreshed-repo", defaultProfileId: "refreshed-profile" });
    expect(setRepositories).toHaveBeenCalledTimes(1);
    expect(setAgentProfileId).toHaveBeenCalledTimes(1);
  });

  it("preserves edits made before the parent's context arrives", () => {
    const setRepositories = vi.fn();
    const setAgentProfileId = vi.fn();
    renderHook(() =>
      useSubtaskParentDefaults({
        parentRepositoryId: "parent-repo",
        defaultProfileId: "parent-profile",
        baseBranch: null,
        repositoriesDirty: true,
        agentProfileId: "chosen-profile",
        setRepositories,
        setAgentProfileId,
      }),
    );
    expect(setRepositories).not.toHaveBeenCalled();
    expect(setAgentProfileId).not.toHaveBeenCalled();
  });
});
