import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import type { TaskRepositoryRoot } from "@/hooks/domains/session/use-task-repository-roots";

const pushMock = vi.fn();
const ROOT_TRACKER = ".claude/worktrees/fix-b";
const REPO_ID = "repo-1";
let mockRepoNames: string[] = [ROOT_TRACKER];
let mockTaskPRs: Array<Record<string, unknown>> = [];

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  ContextMenuContent: ({ children }: { children: ReactNode }) => <div role="menu">{children}</div>,
  ContextMenuItem: ({
    children,
    disabled,
    onSelect,
    "data-testid": testId,
  }: {
    children: ReactNode;
    disabled?: boolean;
    onSelect?: () => void;
    "data-testid"?: string;
  }) => (
    <button type="button" data-testid={testId} disabled={disabled} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
  ContextMenuSeparator: () => <hr />,
}));
vi.mock("@/hooks/domains/session/use-session-git", () => ({
  useSessionGit: () => ({ repoNames: mockRepoNames, push: pushMock }),
}));
vi.mock("@/components/vcs/vcs-dialogs", () => ({ useOptionalVcsDialogs: () => null }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/use-open-session-in-editor", () => ({
  useOpenSessionInEditor: () => ({ open: vi.fn() }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      taskPRs: {
        workspaceId: "ws",
        workspaceContextGeneration: 1,
        byTaskId: { "task-1": mockTaskPRs },
      },
      workspaces: { activeId: "ws" },
      workspaceContextGeneration: 1,
      addUserShell: vi.fn(),
    }),
}));

import { RootContextMenu } from "./root-context-menu";
import { FileContextMenu } from "./file-context-menu";

afterEach(() => {
  cleanup();
  mockRepoNames = [ROOT_TRACKER];
  mockTaskPRs = [];
});

function root(overrides: Partial<TaskRepositoryRoot> = {}): TaskRepositoryRoot {
  return {
    key: "tr-adopted",
    taskId: "task-1",
    sessionId: "session-1",
    taskRepositoryId: "tr-adopted",
    repositoryId: REPO_ID,
    repositoryName: "kandev",
    trackerName: ROOT_TRACKER,
    path: "/task-root/.claude/worktrees/fix-b",
    relativePath: ROOT_TRACKER,
    branch: "fix-b",
    worktreeId: "wt-adopted",
    environmentId: "env-1",
    isPrimary: false,
    removable: true,
    adopted: true,
    displayLabel: "kandev · fix-b",
    ...overrides,
  };
}

function renderMenu(candidate = root()) {
  return render(
    <RootContextMenu root={candidate} surface="changes">
      <button type="button">kandev · fix-b</button>
    </RootContextMenu>,
  );
}

describe("RootContextMenu", () => {
  it("renders the shared root actions and Remove from task last", () => {
    renderMenu();
    const items = screen.getAllByRole("button").map((item) => item.textContent);
    expect(items).toContain("Open in editor");
    expect(items).toContain("Open terminal here");
    expect(items).toContain("Show in Files");
    expect(items).toContain("Copy full path");
    expect(items).toContain("Copy relative path");
    expect(items).toContain("Copy branch name");
    expect(items).toContain("Push");
    expect(items.some((item) => item?.includes("Create PR"))).toBe(true);
    expect(items[items.length - 1]).toContain("Remove from task");
  });

  it("omits Remove from task for the primary root", () => {
    renderMenu(root({ isPrimary: true, removable: false }));
    expect(screen.queryByText("Remove from task")).toBeNull();
  });

  it("shows a visible reason instead of hiding unavailable actions", () => {
    renderMenu(root({ worktreeId: "" }));
    expect(screen.getByText("No worktree for this root")).toBeTruthy();
  });

  it("disables Push and Create PR when the root has no matching tracker", () => {
    mockRepoNames = [];
    renderMenu();
    const push = screen.getByTestId("root-menu-push") as HTMLButtonElement;
    const createPR = screen.getByTestId("root-menu-create-pr") as HTMLButtonElement;
    expect(push.disabled).toBe(true);
    expect(createPR.disabled).toBe(true);
    expect(screen.getAllByText("This root has no Git tracker yet").length).toBeGreaterThan(0);
  });

  it("matches a linked PR by branch, not repository alone", () => {
    mockTaskPRs = [
      {
        repository_id: REPO_ID,
        head_branch: "other-branch",
        pr_url: "https://github.com/acme/repo/pull/1",
        pr_number: 1,
      },
    ];
    renderMenu();
    expect(screen.getByTestId("root-menu-create-pr")).toBeTruthy();
    expect(screen.queryByText("Open PR #1")).toBeNull();
    cleanup();

    mockTaskPRs = [
      {
        repository_id: REPO_ID,
        head_branch: "fix-b",
        pr_url: "https://github.com/acme/repo/pull/1",
        pr_number: 1,
      },
    ];
    renderMenu();
    expect(screen.getByText("Open PR #1")).toBeTruthy();
    expect(screen.queryByTestId("root-menu-create-pr")).toBeNull();
  });

  it("renders the root menu instead of Delete and Rename for a Files root row", () => {
    render(
      <FileContextMenu
        node={{ name: "fix-b", path: ROOT_TRACKER, is_dir: true }}
        tree={{ name: "root", path: "", is_dir: true }}
        setTree={vi.fn()}
        onStartRename={vi.fn()}
        rootDescriptor={root()}
      >
        <div>fix-b</div>
      </FileContextMenu>,
    );
    expect(screen.getByText("Open terminal here")).toBeTruthy();
    expect(screen.queryByText("Delete")).toBeNull();
    expect(screen.queryByText("Rename")).toBeNull();
  });

  it("swaps Show in Files for Show in Changes on the Files surface", () => {
    render(
      <RootContextMenu root={root()} surface="files">
        <button type="button">fix-b</button>
      </RootContextMenu>,
    );
    expect(screen.getByText("Show in Changes")).toBeTruthy();
    expect(screen.queryByText("Show in Files")).toBeNull();
  });
});
