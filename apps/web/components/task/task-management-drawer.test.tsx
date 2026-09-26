import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TaskManagementDrawer } from "./task-management-drawer";

afterEach(cleanup);
describe("task drawer navigation", () => {
  // @covers AC-TASKS-THREADS-ACTIONS-004.3, AC-TASKS-THREADS-ACTIONS-004.6, AC-TASKS-THREADS-ACTIONS-001.3
  it("opens the change form directly and keeps same-workflow step navigation separate", () => {
    const onMove = vi.fn();
    const onChangeWorkflow = vi.fn();
    render(
      <StateProvider>
        <TaskManagementDrawer
          task={{ id: "A", title: "Task A", workflowId: "one", workflowStepId: "s1" }}
          workflows={[
            { id: "one", name: "One" },
            { id: "two", name: "Two" },
          ]}
          stepsByWorkflowId={{
            one: [
              { id: "s1", title: "Initial" },
              { id: "s2", title: "Review" },
            ],
            two: [{ id: "s3", title: "Build" }],
          }}
          onMove={onMove}
          onChangeWorkflow={onChangeWorkflow}
          onPriority={vi.fn()}
          onArchive={vi.fn()}
          onDelete={vi.fn()}
          closeMenu={vi.fn()}
          linkActions={{}}
          onCloseAutoFocus={(event) => event.preventDefault()}
        />
      </StateProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Change workflow..." }));
    expect(onChangeWorkflow).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByTestId("task-management-page-steps"));
    fireEvent.click(screen.getByRole("button", { name: "Review" }));
    expect(onMove).toHaveBeenCalledWith("one", "s2");
  });

  it("keeps current lifecycle status outside the disabled step choice", () => {
    render(
      <StateProvider>
        <TaskManagementDrawer
          task={{
            id: "A",
            title: "Task A",
            workflowId: "one",
            workflowStepId: "s1",
            state: "SCHEDULING",
            sessionState: "STARTING",
          }}
          workflows={[{ id: "one", name: "One" }]}
          stepsByWorkflowId={{
            one: [
              { id: "s1", title: "Initial" },
              { id: "s2", title: "Review" },
            ],
          }}
          onMove={vi.fn()}
          onChangeWorkflow={vi.fn()}
          onPriority={vi.fn()}
          onArchive={vi.fn()}
          onDelete={vi.fn()}
          closeMenu={vi.fn()}
          linkActions={{}}
          onCloseAutoFocus={(event) => event.preventDefault()}
        />
      </StateProvider>,
    );

    fireEvent.click(screen.getByTestId("task-management-page-steps"));
    const currentChoice = screen.getByTestId("task-context-step-s1");
    const status = screen.getByTestId("task-context-step-progress-s1");

    expect(currentChoice.hasAttribute("disabled")).toBe(true);
    expect(status.getAttribute("role")).toBe("status");
    expect(currentChoice.contains(status)).toBe(false);
  });
});
