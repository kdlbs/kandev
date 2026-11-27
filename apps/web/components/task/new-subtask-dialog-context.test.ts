import { describe, expect, it } from "vitest";
import { taskId } from "@/lib/types/ids";
import { resolveSubtaskParentContext } from "./new-subtask-dialog";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { Message, TaskSession } from "@/lib/types/http";

const parentTask = {
  id: "parent-b",
  primarySessionId: "session-b",
  repositories: [
    { id: "task-repo-b", repository_id: "repo-b", base_branch: "release", position: 0 },
  ],
} as KanbanState["tasks"][number];

const parentSession = {
  id: "session-b",
  task_id: taskId("parent-b"),
  agent_profile_id: "profile-b",
  repository_id: "repo-b",
  base_branch: "main",
  worktree_branch: "parent-b-branch",
  is_primary: true,
} as TaskSession;

const parentMessage = {
  id: "message-b",
  session_id: "session-b",
  task_id: taskId("parent-b"),
  author_type: "user",
  content: "parent B prompt",
  type: "message",
} as Message;

const activeSession = {
  ...parentSession,
  id: "session-a",
  task_id: "parent-a",
  agent_profile_id: "profile-a",
} as TaskSession;

describe("resolveSubtaskParentContext", () => {
  it("uses the selected parent instead of the active session", () => {
    expect(
      resolveSubtaskParentContext({
        task: parentTask,
        sessionsById: {
          "session-a": activeSession,
          "session-b": parentSession,
        },
        sessionsByTaskId: { "parent-b": [parentSession] },
        worktreeIdsBySessionId: { "session-b": ["worktree-b"] },
        worktrees: { "worktree-b": { branch: "parent-b-worktree" } },
        messagesBySession: { "session-b": [parentMessage] },
      }),
    ).toMatchObject({
      session: parentSession,
      worktreeBranch: "parent-b-worktree",
      initialPrompt: "parent B prompt",
      parentRepositoryId: "repo-b",
      baseBranch: "release",
    });
  });

  it("handles a parent task without repositories", () => {
    expect(
      resolveSubtaskParentContext({
        task: { id: "repo-less-parent" } as KanbanState["tasks"][number],
        sessionsById: {},
        sessionsByTaskId: {},
        worktreeIdsBySessionId: {},
        worktrees: {},
        messagesBySession: {},
      }),
    ).toMatchObject({
      session: null,
      worktreeBranch: null,
      initialPrompt: null,
      parentRepositoryId: null,
      baseBranch: null,
    });
  });
});

// @covers AC-TASKS-SUBTASK-DIALOG-PARENT-DEFAULTS-001.2
describe("selected parent primary resolution", () => {
  it("does not substitute another session while the explicit primary is unavailable", () => {
    const context = resolveSubtaskParentContext({
      task: parentTask,
      sessionsById: {},
      sessionsByTaskId: {
        "parent-b": [{ ...activeSession, task_id: taskId("parent-b"), is_primary: true }],
      },
      worktreeIdsBySessionId: {},
      worktrees: {},
      messagesBySession: {},
    });
    expect(context.session).toBeNull();
    expect(context.parentRepositoryId).toBe("repo-b");
    expect(context.baseBranch).toBe("release");
  });
  it("uses the marked primary when the task has no explicit primary ID", () => {
    const context = resolveSubtaskParentContext({
      task: { ...parentTask, primarySessionId: null, repositories: [] },
      sessionsById: {},
      sessionsByTaskId: {
        "parent-b": [
          { ...activeSession, task_id: taskId("parent-b"), is_primary: false },
          parentSession,
        ],
      },
      worktreeIdsBySessionId: {},
      worktrees: {},
      messagesBySession: {},
    });
    expect(context.session?.agent_profile_id).toBe("profile-b");
    expect(context.parentRepositoryId).toBe("repo-b");
    expect(context.baseBranch).toBe("main");
  });
});
