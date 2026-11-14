/* eslint-disable sonarjs/no-duplicate-string -- Workspace fixtures repeat across session-base cases. */
import type { ReactNode } from "react";
import { useEffect } from "react";
import ReactMarkdown from "react-markdown";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";
import { markdownComponents, remarkPlugins } from "./markdown-components";
import { MemoizedMarkdown } from "./memoized-markdown";

const openFile = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile }),
}));

vi.mock("@/components/shared/mermaid-block", () => ({
  MermaidBlock: ({ code }: { code: string }) => <div data-kind="mermaid">{code}</div>,
}));

vi.mock("@/components/task/chat/messages/code-block", () => ({
  CodeBlock: ({ children }: { children: ReactNode }) => <pre>{children}</pre>,
}));

vi.mock("@/components/task/chat/messages/inline-code", () => ({
  InlineCode: ({ children }: { children: ReactNode }) => <code>{children}</code>,
}));

const SESSION_ID = toSessionId("session-1");
const OTHER_SESSION_ID = toSessionId("session-other");
const TASK_ID = toTaskId("task-1");
const session: TaskSession = {
  id: SESSION_ID,
  task_id: TASK_ID,
  state: "RUNNING",
  started_at: "2026-11-05T00:00:00Z",
  updated_at: "2026-11-05T00:00:00Z",
  workspace_path: "/task-root",
  worktree_path: "/task-root/master",
  agent_workspace_path: "/task-root/master",
  worktrees: [
    {
      id: "session-worktree-1",
      session_id: SESSION_ID,
      worktree_id: "worktree-master",
      repository_id: "repo-master" as never,
      position: 0,
      worktree_path: "/task-root/master",
    },
    {
      id: "session-worktree-2",
      session_id: SESSION_ID,
      worktree_id: "worktree-secondary",
      repository_id: "repo-secondary" as never,
      position: 1,
      worktree_path: "/task-root/secondary",
    },
  ],
};

const otherSession: TaskSession = {
  id: OTHER_SESSION_ID,
  task_id: TASK_ID,
  state: "RUNNING",
  started_at: "2026-11-05T00:00:00Z",
  updated_at: "2026-11-05T00:00:00Z",
  workspace_path: "/task-root-other",
  worktree_path: "/task-root-other/other",
  agent_workspace_path: "/task-root-other/other",
};

/** Seeds two live sessions through the real store, as hydration would. */
function SeedActiveSession({
  activeSessionId = SESSION_ID,
}: {
  activeSessionId?: typeof SESSION_ID;
}) {
  const store = useAppStoreApi();
  useEffect(() => {
    store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId },
      taskSessions: {
        ...state.taskSessions,
        items: {
          ...state.taskSessions.items,
          [SESSION_ID]: session,
          [OTHER_SESSION_ID]: otherSession,
        },
      },
    }));
  }, [activeSessionId, store]);
  return null;
}

function UnrelatedStoreUpdate() {
  const store = useAppStoreApi();
  return (
    <button
      onClick={() =>
        store.getState().setSessionAgentctlStatus(SESSION_ID, { status: "ready", updatedAt: "" })
      }
    >
      unrelated-update
    </button>
  );
}

function Providers({
  children,
  activeSessionId,
}: {
  children: ReactNode;
  activeSessionId?: typeof SESSION_ID;
}) {
  return (
    <StateProvider>
      <SeedActiveSession activeSessionId={activeSessionId} />
      <UnrelatedStoreUpdate />
      {children}
    </StateProvider>
  );
}

describe("Markdown fallback file-link snapshot", () => {
  afterEach(() => {
    cleanup();
    openFile.mockClear();
  });

  it("stays mounted through unrelated store updates with an active session", async () => {
    render(
      <Providers>
        <ReactMarkdown
          remarkPlugins={remarkPlugins}
          components={markdownComponents}
        >{`[work order](implementation_plans/task.md) [qualified](master/README.md) [secondary](secondary/README.md) [external](https://example.com/docs)`}</ReactMarkdown>
      </Providers>,
    );

    const link = await screen.findByRole("link", { name: "work order" });
    await waitFor(() => expect(screen.getByRole("link", { name: "external" })).toBeTruthy());

    fireEvent.click(link);
    expect(openFile).toHaveBeenCalledWith("master/implementation_plans/task.md");
    fireEvent.click(screen.getByRole("link", { name: "qualified" }));
    expect(openFile).toHaveBeenLastCalledWith("master/README.md");
    fireEvent.click(screen.getByRole("link", { name: "secondary" }));
    expect(openFile).toHaveBeenLastCalledWith("secondary/README.md");

    fireEvent.click(screen.getByRole("button", { name: "unrelated-update" }));
    await waitFor(() => expect(screen.getByRole("link", { name: "external" })).toBeTruthy());

    fireEvent.click(screen.getByRole("link", { name: "work order" }));
    expect(openFile).toHaveBeenCalledTimes(4);
    expect(openFile).toHaveBeenLastCalledWith("master/implementation_plans/task.md");
  });

  it("uses the rendering session's base when it differs from the active session", async () => {
    render(
      <Providers activeSessionId={OTHER_SESSION_ID}>
        <MemoizedMarkdown content="[run guide](docs/guide.md)" worktreePath="/task-root" />
      </Providers>,
    );

    const link = await screen.findByRole("link", { name: "run guide" });
    fireEvent.click(link);

    expect(openFile).toHaveBeenLastCalledWith("master/docs/guide.md");
  });
});
