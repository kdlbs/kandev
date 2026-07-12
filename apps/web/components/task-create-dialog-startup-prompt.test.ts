import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useRepositoryStartupPromptPrefillEffect } from "./task-create-dialog-startup-prompt";
import type { DialogFormState } from "./task-create-dialog-types";
import { repositoryId, workspaceId, type Repository } from "@/lib/types/http";

// Minimal DialogFormState stub. Only the fields the pre-fill hook reads
// (descriptionInputRef, setHasDescription, repositories rows) are populated;
// the rest is cast through `unknown` to keep the test surface small.
type FsFake = {
  repositoryRows?: Array<{ repositoryId?: string }>;
  initialDescription?: string;
  setHasDescription?: ReturnType<typeof vi.fn>;
};

function makeFs(overrides: FsFake = {}) {
  let currentValue = overrides.initialDescription ?? "";
  const setValue = vi.fn((v: string) => {
    currentValue = v;
  });
  const setHasDescription = overrides.setHasDescription ?? vi.fn();
  const descriptionInputRef = {
    current: {
      getValue: () => currentValue,
      setValue,
    },
  };
  const fs = {
    descriptionInputRef,
    setHasDescription,
    repositories: overrides.repositoryRows ?? [{ repositoryId: "repo-1" }],
  } as unknown as DialogFormState;
  return { fs, setValue, setHasDescription };
}

const NOW = "2026-07-12T00:00:00Z";
function makeRepo(id: string, startupPrompt: string): Repository {
  return {
    id: repositoryId(id),
    workspace_id: workspaceId("ws-1"),
    name: "repo",
    source_type: "local",
    local_path: "",
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    startup_prompt: startupPrompt,
    created_at: NOW,
    updated_at: NOW,
  };
}

describe("useRepositoryStartupPromptPrefillEffect", () => {
  const repos = [makeRepo("repo-1", "Start with {{TASK_TITLE}}.")];

  it("pre-fills the description in create mode with the resolved prompt", () => {
    const { fs, setValue, setHasDescription } = makeFs();
    renderHook(() =>
      useRepositoryStartupPromptPrefillEffect(fs, /*open*/ true, repos, "Refactor billing", true),
    );
    expect(setValue).toHaveBeenCalledWith("Start with Refactor billing.");
    expect(setHasDescription).toHaveBeenCalledWith(true);
  });

  it("does NOT pre-fill in edit mode (isCreateMode=false), even with an empty description", () => {
    // Regression: opening an existing task in edit mode with an empty
    // description previously injected the repository default and would
    // persist it on save — silently rewriting user-owned state.
    const { fs, setValue, setHasDescription } = makeFs();
    renderHook(() =>
      useRepositoryStartupPromptPrefillEffect(fs, /*open*/ true, repos, "Existing task", false),
    );
    expect(setValue).not.toHaveBeenCalled();
    expect(setHasDescription).not.toHaveBeenCalled();
  });

  it("does NOT pre-fill when the dialog is closed", () => {
    const { fs, setValue } = makeFs();
    renderHook(() =>
      useRepositoryStartupPromptPrefillEffect(fs, /*open*/ false, repos, "Anything", true),
    );
    expect(setValue).not.toHaveBeenCalled();
  });

  it("does NOT pre-fill in session mode (isCreateMode=false) either", () => {
    const { fs, setValue } = makeFs();
    renderHook(() =>
      useRepositoryStartupPromptPrefillEffect(fs, /*open*/ true, repos, "Session prompt", false),
    );
    expect(setValue).not.toHaveBeenCalled();
  });
});
