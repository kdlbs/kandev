import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { pageRouteKey, usePageContext, type WorkspacePageRoute } from "./use-page-context";

const WS = "ws-1";

type Seed = {
  tasks?: Array<Record<string, unknown>>;
  workflows?: Array<{ id: string; workspaceId: string; name: string }>;
  activeWorkflowId?: string | null;
};

function context(route: WorkspacePageRoute, seed: Seed) {
  const initialState = {
    kanban: { tasks: seed.tasks ?? [] },
    workflows: { items: seed.workflows ?? [], activeId: seed.activeWorkflowId ?? null },
  } as never;
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider initialState={initialState}>{children}</StateProvider>
  );
  return renderHook(() => usePageContext(route, WS), { wrapper }).result.current;
}

const TASK = { id: "t-1", workspaceId: WS, identifier: "KAN-7", workflowId: "wf-1" };
const TASK_ROUTE: WorkspacePageRoute = { kind: "taskDetail", taskId: "t-1" };

describe("usePageContext task pages", () => {
  it("names the task by its identifier", () => {
    expect(context(TASK_ROUTE, { tasks: [TASK] })).toEqual({
      ref: { kind: "task", id: "t-1" },
      label: "KAN-7",
      workflowId: "wf-1",
      repositoryIds: [],
    });
  });

  it("is null until the task is in the store", () => {
    expect(context(TASK_ROUTE, {})).toBeNull();
  });

  it("is null for a task without an identifier, never falling back to the title", () => {
    expect(
      context(TASK_ROUTE, { tasks: [{ ...TASK, identifier: undefined, title: "Secret" }] }),
    ).toBeNull();
  });

  it("is null for a task of another workspace", () => {
    expect(context(TASK_ROUTE, { tasks: [{ ...TASK, workspaceId: "ws-2" }] })).toBeNull();
  });
});

describe("usePageContext board and inbox", () => {
  const workflows = [{ id: "wf-1", workspaceId: WS, name: "Sprint: one\nboard" }];

  it("names the board by the active workflow, normalized for the wire prefix", () => {
    expect(context({ kind: "kanban" }, { workflows, activeWorkflowId: "wf-1" })).toEqual({
      ref: { kind: "workflow", id: "wf-1" },
      label: "Sprint - one board",
      workflowId: "wf-1",
      repositoryIds: null,
    });
  });

  it("is null under All Workflows", () => {
    expect(context({ kind: "kanban" }, { workflows, activeWorkflowId: null })).toBeNull();
  });

  it("is null for a stale active workflow from another workspace", () => {
    const other = [{ id: "wf-9", workspaceId: "ws-2", name: "Other" }];
    expect(context({ kind: "kanban" }, { workflows: other, activeWorkflowId: "wf-9" })).toBeNull();
  });

  it("is null on the Inbox", () => {
    expect(context({ kind: "needsYouInbox" }, { workflows, activeWorkflowId: "wf-1" })).toBeNull();
  });
});

describe("pageRouteKey", () => {
  it("keys a task by its id and the board by the active workflow", () => {
    expect(pageRouteKey(TASK_ROUTE, "wf-1")).toBe("taskDetail:t-1");
    expect(pageRouteKey({ kind: "kanban" }, "wf-1")).toBe("kanban:wf-1");
    expect(pageRouteKey({ kind: "kanban" }, null)).toBe("kanban:");
    expect(pageRouteKey({ kind: "needsYouInbox" }, "wf-1")).toBe("needsYouInbox:");
  });
});

describe("usePageContext task repositories", () => {
  it("carries the task's repository ids, from the list or the main repository", () => {
    const withList = { ...TASK, repositories: [{ repository_id: "r1" }, { repository_id: "r2" }] };
    expect(context(TASK_ROUTE, { tasks: [withList] })?.repositoryIds).toEqual(["r1", "r2"]);
    expect(
      context(TASK_ROUTE, { tasks: [{ ...TASK, repositoryId: "r9" }] })?.repositoryIds,
    ).toEqual(["r9"]);
  });
});
