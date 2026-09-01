import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { Repository } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import type { RepositorySetDraft } from "./use-workspace-repository-sets";

const mockUseBranches = vi.fn();

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("@/hooks/domains/workspace/use-repository-branches", () => ({
  useBranches: (...args: unknown[]) => mockUseBranches(...args),
}));

import { RepositorySetEditorDialog } from "./workspace-repository-set-editor";

const repository: Repository = {
  id: repositoryId("repo-web"),
  workspace_id: workspaceId("ws-1"),
  name: "web",
  source_type: "local",
  local_path: "/repos/web",
  provider: "",
  provider_repo_id: "",
  provider_owner: "",
  provider_name: "",
  default_branch: "main",
  worktree_branch_prefix: "feature/",
  pull_before_worktree: false,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: "2026-08-17T09:00:00Z",
  updated_at: "2026-08-17T09:00:00Z",
};

function draft(baseBranch = ""): RepositorySetDraft {
  return {
    setId: "set-1",
    name: "Full-stack",
    description: "",
    members: [{ repositoryId: "repo-web", baseBranch }],
  };
}

function renderEditor(currentDraft = draft(), onChange = vi.fn()) {
  return render(
    <TooltipProvider>
      <RepositorySetEditorDialog
        workspaceId="ws-1"
        draft={currentDraft}
        repositories={[repository]}
        error={null}
        saving={false}
        onClose={vi.fn()}
        onChange={onChange}
        onSubmit={vi.fn()}
      />
    </TooltipProvider>,
  );
}

beforeEach(() => {
  mockUseBranches.mockReset();
  mockUseBranches.mockImplementation((_source: unknown, enabled: boolean) => ({
    branches: enabled
      ? [
          { name: "main", type: "local", remote: "" },
          { name: "develop", type: "local", remote: "" },
        ]
      : [],
    isLoaded: enabled,
    isLoading: false,
    refresh: vi.fn(),
  }));
});

afterEach(() => cleanup());

describe("RepositorySetEditorDialog base branch picker", () => {
  it("does not load branches until a member picker opens", () => {
    renderEditor();

    expect(mockUseBranches).toHaveBeenCalledWith(
      { kind: "id", workspaceId: "ws-1", repositoryId: "repo-web" },
      false,
    );
    expect(mockUseBranches).not.toHaveBeenCalledWith(
      { kind: "id", workspaceId: "ws-1", repositoryId: "repo-web" },
      true,
    );

    fireEvent.click(screen.getByTestId("repository-set-base-repo-web"));

    expect(mockUseBranches).toHaveBeenCalledWith(
      { kind: "id", workspaceId: "ws-1", repositoryId: "repo-web" },
      true,
    );
  });

  it("keeps an unavailable saved branch visible and disabled after loading", () => {
    renderEditor(draft("retired"));

    fireEvent.click(screen.getByTestId("repository-set-base-repo-web"));

    const unavailable = screen
      .getAllByRole("option")
      .find((option) => option.textContent?.includes("retired"));
    expect(unavailable).toBeDefined();
    expect(unavailable?.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByTestId("repository-set-base-repo-web").textContent).toContain("retired");
  });
});
