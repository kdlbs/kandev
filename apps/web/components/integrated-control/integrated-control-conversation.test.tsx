import { useLayoutEffect } from "react";
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { Task, TaskSession } from "@/lib/types/http";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import { IntegratedControlConversation } from "./integrated-control-conversation";

const boundary = vi.hoisted(() => ({
  task: vi.fn(),
  sessions: vi.fn(),
  rendererCommits: [] as string[],
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: boundary.task }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listTaskSessions: boundary.sessions,
}));
vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: vi.fn().mockResolvedValue({ task_prs: {} }),
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
vi.mock("@/components/quick-chat/quick-chat-session-view", () => ({
  QuickChatSessionView: ({ session }: { session: { workspaceId: string } }) => {
    useLayoutEffect(() => {
      boundary.rendererCommits.push(session.workspaceId);
    });
    return null;
  },
}));
const workspace = {
  id: "beta",
  name: "Beta",
  scopes: ["workspace.read", "workspace.manage"],
} as ControlWorkspace;
const coordinator = {
  id: "coordinator",
  workspace_id: "beta",
  conversation_task_id: "task",
} as Coordinator;
const task: Task = {
  id: taskId("task"),
  workspace_id: workspaceId("beta"),
  workflow_id: workflowId("workflow"),
  workflow_step_id: "step",
  position: 0,
  title: "Original coordinator conversation",
  description: "",
  state: "WAITING_FOR_INPUT",
  priority: "medium",
  created_at: "",
  updated_at: "",
  origin: "coordinator",
  metadata: { coordinator_id: "coordinator" },
};
const session = {
  id: "session",
  task_id: "task",
  state: "WAITING_FOR_INPUT",
  is_primary: true,
} as TaskSession;

beforeEach(() => {
  boundary.task.mockReset().mockResolvedValue(task);
  boundary.sessions.mockReset().mockResolvedValue({ sessions: [session] });
  boundary.rendererCommits.length = 0;
});
afterEach(cleanup);

it("never commits the original renderer under an unverified new workspace claiming the same task", async () => {
  const renderOwner = (owner: ControlWorkspace) => (
    <StateProvider>
      <IntegratedControlConversation workspace={owner} coordinator={coordinator} />
    </StateProvider>
  );
  const view = render(renderOwner(workspace));
  await waitFor(() => expect(boundary.rendererCommits).toContain("beta"));
  const pending = Promise.withResolvers<Task>();
  boundary.task.mockReturnValueOnce(pending.promise);
  boundary.rendererCommits.length = 0;
  view.rerender(renderOwner({ ...workspace, id: "gamma" }));
  expect(boundary.rendererCommits).toEqual([]);
  await act(async () => pending.resolve(task));
  expect(boundary.rendererCommits).toEqual([]);
  expect(view.getByRole("alert").textContent).toBe(
    "This conversation is unavailable or no longer accessible.",
  );
});

it("does not read a task for a caller lacking management access", () => {
  render(
    <StateProvider>
      <IntegratedControlConversation
        workspace={{ ...workspace, scopes: ["workspace.read"] }}
        coordinator={coordinator}
      />
    </StateProvider>,
  );
  expect(boundary.task).not.toHaveBeenCalled();
  expect(boundary.sessions).not.toHaveBeenCalled();
});
