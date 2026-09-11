import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { StateProvider } from "@/components/state-provider";
import { TaskManagementMenu } from "./task-management-menu";

afterEach(cleanup);
describe("focused shared task actions", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-001.1, AC-TASKS-THREADS-ACTIONS-001.2, AC-TASKS-THREADS-ACTIONS-001.3
  it("preserves current-step eligibility and dispatches the chosen priority", async () => {
    const onPriority = vi.fn();
    render(
      <StateProvider>
        <ContextMenu>
          <ContextMenuTrigger>Open</ContextMenuTrigger>
          <ContextMenuContent>
            <TaskManagementMenu
              task={{
                id: "A",
                title: "A",
                priority: "high",
                workflowId: "one",
                workflowStepId: "s1",
              }}
              workflows={[
                { id: "one", name: "One" },
                { id: "hidden", name: "Hidden", hidden: true },
              ]}
              stepsByWorkflowId={{
                one: [
                  { id: "s1", title: "Current step" },
                  { id: "s2", title: "Review" },
                ],
              }}
              onMove={vi.fn()}
              onPriority={onPriority}
              onArchive={vi.fn()}
              onDelete={vi.fn()}
              closeMenu={vi.fn()}
              linkActions={{}}
            />
          </ContextMenuContent>
        </ContextMenu>
      </StateProvider>,
    );
    fireEvent.contextMenu(screen.getByText("Open"));
    expect(screen.queryByText("Send to workflow")).toBeNull();
    fireEvent.pointerMove(screen.getByTestId("task-context-move-to"), { pointerType: "mouse" });
    const current = await screen.findByTestId("task-context-step-s1");
    expect(current.getAttribute("aria-disabled")).toBe("true");
    fireEvent.pointerMove(screen.getByTestId("task-context-priority"), { pointerType: "mouse" });
    const high = await screen.findByTestId("task-context-priority-high");
    expect(within(high).getByText("Current")).toBeTruthy();
    fireEvent.click(screen.getByTestId("task-context-priority-low"));
    expect(onPriority).toHaveBeenCalledWith("low");
  });
});
