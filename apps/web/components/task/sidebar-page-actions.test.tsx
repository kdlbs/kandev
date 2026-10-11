import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useSidebarTaskEdit } from "./task-session-sidebar-edit";
import { useSidebarLinkActions } from "./task-session-sidebar-link-actions";
import type { KanbanState } from "@/lib/state/slices";
it("uses bounded page records for editing and linking without a board snapshot", () => {
  const task = {
    id: "child",
    title: "Page child",
    description: "Description",
    workflowId: "workflow",
    workflowStepId: "step",
    repositoryId: "repository",
  } as KanbanState["tasks"][number];
  const { result } = renderHook(
    () => {
      const store = useAppStoreApi();
      return {
        store,
        edit: useSidebarTaskEdit([task]),
        links: useSidebarLinkActions(store, [task]),
      };
    },
    {
      wrapper: ({ children }: { children: ReactNode }) => <StateProvider>{children}</StateProvider>,
    },
  );
  act(() => {
    result.current.edit.handleEditTask({
      id: task.id,
      title: task.title,
      workflowId: task.workflowId,
      workflowStepId: task.workflowStepId,
    });
    result.current.links.handleLinkMergeRequestTask(task.id);
  });
  expect(result.current.edit.editingTask).toMatchObject({
    id: task.id,
    description: "Description",
    repositoryId: "repository",
  });
  expect(result.current.links.linkingMergeRequestTask).toMatchObject({
    id: task.id,
    repositoryId: "repository",
  });
  expect(result.current.store.getState().kanban.tasks).toEqual([]);
  expect(result.current.store.getState().kanbanMulti.snapshots).toEqual({});
});
