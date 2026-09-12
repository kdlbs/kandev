import { createRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { ThreadTaskActionsProvider, ThreadTaskMenuButton } from "./thread-task-actions";

afterEach(cleanup);
// @covers AC-TASKS-THREADS-ACTIONS-002.1, AC-TASKS-THREADS-ACTIONS-004.1, AC-TASKS-THREADS-ACTIONS-004.2
it("opens task A's actions from its named overflow without navigating", async () => {
  render(
    <StateProvider
      initialState={{
        workspaces: { items: [], activeId: "workspace" },
        workflows: {
          activeId: null,
          items: [{ id: "workflow", workspaceId: "workspace", name: "Flow" }],
        },
        kanbanMulti: {
          isLoading: false,
          snapshots: {
            workflow: {
              workflowId: "workflow",
              workflowName: "Flow",
              steps: [],
              tasks: [
                {
                  id: "A",
                  title: "A",
                  workflowId: "workflow",
                  workflowStepId: "step",
                  position: 0,
                },
              ],
            },
          },
        },
      }}
    >
      <ToastProvider>
        <ThreadTaskActionsProvider boardRef={createRef()}>
          <ThreadTaskMenuButton taskId="A" />
        </ThreadTaskActionsProvider>
      </ToastProvider>
    </StateProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Task actions" }));
  expect(await screen.findByTestId("task-management-menu")).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: "Delete" })).toBeTruthy();
});
