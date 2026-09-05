import { useContext, type ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MarkdownFileLinkContext } from "./markdown-components";

const WORKSPACE_ID = vi.hoisted(() => "workspace-1");
const storeState = vi.hoisted(() => ({
  workspaces: { activeId: WORKSPACE_ID as string | null },
  taskSessions: {
    items: {
      "session-1": {
        repository_id: "repo-1",
        workspace_path: "/root/.kandev/tasks/example" as string | undefined,
        worktree_path: "/root/.kandev/tasks/example/kandev",
      },
    },
  },
  repositories: {
    itemsByWorkspaceId: {
      [WORKSPACE_ID]: [
        { id: "repo-1", local_path: "/home/jcfs/projects/kandev" },
        { id: "repo-2", local_path: "/home/jcfs/projects/plugin" },
      ],
    },
  },
  office: {
    tasks: { items: [] },
  },
}));
const taskState = vi.hoisted(() => ({
  workspaceId: WORKSPACE_ID as string | undefined,
  repositoryIds: ["repo-1", "repo-2"],
}));
const sessionWorktreeState = vi.hoisted(() => ({
  items: [
    { repositoryId: "repo-1", path: "/root/.kandev/tasks/example/kandev" },
    { repositoryId: "repo-2", path: "/root/.kandev/tasks/example/plugin" },
  ],
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

vi.mock("@/hooks/use-task", () => ({
  useTask: () => ({
    id: "task-1",
    workspaceId: taskState.workspaceId,
    repositoryId: taskState.repositoryIds[0],
    repositories: taskState.repositoryIds.map((repository_id, position) => ({
      repository_id,
      position,
    })),
  }),
}));

vi.mock("@/hooks/domains/session/use-session-worktrees", () => ({
  useSessionWorktrees: () => sessionWorktreeState.items,
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: (workspaceId?: string | null) => ({
    repositories:
      workspaceId === WORKSPACE_ID ? storeState.repositories.itemsByWorkspaceId[WORKSPACE_ID] : [],
  }),
}));

import { TaskMarkdownFileLinkProvider } from "./task-markdown-file-link-provider";

function ContextProbe() {
  const context = useContext(MarkdownFileLinkContext);
  return (
    <output data-testid="context" data-has-action={context.onOpenFile ? "true" : "false"}>
      {JSON.stringify(context.fileRootAliases)}
    </output>
  );
}

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <TaskMarkdownFileLinkProvider taskId="task-1" sessionId="session-1">
      {children}
    </TaskMarkdownFileLinkProvider>
  );
}

describe("TaskMarkdownFileLinkProvider", () => {
  afterEach(() => {
    cleanup();
    storeState.workspaces.activeId = WORKSPACE_ID;
    storeState.taskSessions.items["session-1"].workspace_path = "/root/.kandev/tasks/example";
    taskState.workspaceId = WORKSPACE_ID;
    taskState.repositoryIds = ["repo-1", "repo-2"];
    sessionWorktreeState.items = [
      { repositoryId: "repo-1", path: "/root/.kandev/tasks/example/kandev" },
      { repositoryId: "repo-2", path: "/root/.kandev/tasks/example/plugin" },
    ];
    vi.clearAllMocks();
  });

  it("provides repository aliases scoped to the rendered task and session", () => {
    render(
      <Wrapper>
        <ContextProbe />
      </Wrapper>,
    );

    expect(screen.getByTestId("context").textContent).toBe(
      JSON.stringify([
        {
          repositoryId: "repo-1",
          sourceRoot: "/home/jcfs/projects/kandev",
          workspaceRelativeRoot: "kandev",
        },
        {
          repositoryId: "repo-2",
          sourceRoot: "/home/jcfs/projects/plugin",
          workspaceRelativeRoot: "plugin",
        },
      ]),
    );
  });

  it("keeps the existing context action and inherits it for nested renderers", () => {
    const onOpenFile = vi.fn();
    render(
      <MarkdownFileLinkContext.Provider value={{ onOpenFile }}>
        <TaskMarkdownFileLinkProvider taskId="task-1" sessionId="session-1">
          <ContextProbe />
        </TaskMarkdownFileLinkProvider>
      </MarkdownFileLinkContext.Provider>,
    );

    expect(screen.getByTestId("context").dataset.hasAction).toBe("true");
  });

  it("uses the active workspace while the task projection omits its workspace id", () => {
    taskState.workspaceId = undefined;
    render(
      <Wrapper>
        <ContextProbe />
      </Wrapper>,
    );

    expect(screen.getByTestId("context").textContent).toContain('"repositoryId":"repo-2"');
  });

  it("uses the legacy session repository and worktree when no worktree list is available", () => {
    taskState.repositoryIds = ["repo-1"];
    sessionWorktreeState.items = [];
    storeState.taskSessions.items["session-1"].workspace_path = undefined;

    render(
      <Wrapper>
        <ContextProbe />
      </Wrapper>,
    );

    expect(screen.getByTestId("context").textContent).toBe(
      JSON.stringify([
        {
          repositoryId: "repo-1",
          sourceRoot: "/home/jcfs/projects/kandev",
          workspaceRelativeRoot: "",
        },
      ]),
    );
  });

  it("does not inherit aliases from an unrelated parent context", () => {
    taskState.workspaceId = undefined;
    taskState.repositoryIds = [];
    storeState.workspaces.activeId = null;
    render(
      <MarkdownFileLinkContext.Provider
        value={{
          fileRootAliases: [
            {
              repositoryId: "unrelated-repo",
              sourceRoot: "/home/other/project",
              workspaceRelativeRoot: "project",
            },
          ],
        }}
      >
        <TaskMarkdownFileLinkProvider taskId="task-1" sessionId="session-1">
          <ContextProbe />
        </TaskMarkdownFileLinkProvider>
      </MarkdownFileLinkContext.Provider>,
    );

    expect(screen.getByTestId("context").textContent).toBe("[]");
  });
});
