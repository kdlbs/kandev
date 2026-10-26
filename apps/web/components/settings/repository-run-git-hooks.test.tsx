import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { RepositoryCard } from "./repository-card";
import { SettingsSaveProvider } from "./settings-save-provider";
import type { Repository, RepositoryScript } from "@/lib/types/http";

afterEach(cleanup);

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

function repository(overrides: Partial<RepositoryWithScripts> = {}): RepositoryWithScripts {
  return {
    id: "repo-1",
    workspace_id: "ws-1",
    name: "kandev",
    source_type: "local",
    local_path: "/home/dev/kandev",
    default_branch: "main",
    worktree_branch_prefix: "feature/",
    worktree_branch_template: "feature/{title}-{suffix}",
    pull_before_worktree: true,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    scripts: [],
    ...overrides,
  } as RepositoryWithScripts;
}

function renderOpenCard(repo: RepositoryWithScripts, onUpdate = vi.fn()) {
  render(
    <StateProvider>
      <ToastProvider>
        <SettingsSaveProvider>
          <RepositoryCard
            repository={repo}
            workspaceId="workspace-1"
            savedRepository={repo}
            isRepositoryDirty={false}
            areScriptsDirty={false}
            autoOpen
            onUpdate={onUpdate}
            onAddScript={vi.fn()}
            onUpdateScript={vi.fn()}
            onDeleteScript={vi.fn()}
            onSave={vi.fn()}
            onDelete={vi.fn()}
          />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return onUpdate;
}

describe("RepositoryCard Run Git hooks", () => {
  it("is off by default and states the trust boundary", () => {
    renderOpenCard(repository());

    const toggle = screen.getByTestId("repository-run-git-hooks-switch");
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(screen.getByText(/run with Kandev's permissions/)).toBeTruthy();
    expect(screen.getByText(/submodule recursion/)).toBeTruthy();
  });

  it("reports the toggled value through onUpdate", () => {
    const onUpdate = renderOpenCard(repository());

    fireEvent.click(screen.getByTestId("repository-run-git-hooks-switch"));

    expect(onUpdate).toHaveBeenCalledWith("repo-1", { run_git_hooks: true });
  });

  it("reflects a saved opt-in", () => {
    renderOpenCard(repository({ run_git_hooks: true }));

    expect(screen.getByTestId("repository-run-git-hooks-switch").getAttribute("aria-checked")).toBe(
      "true",
    );
  });
});
